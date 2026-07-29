"""
Desinformationssystem — bulk enrichment via headless Claude Code (Sonnet).

Alternative backend to enrich.py's OpenRouter/DeepSeek call, for working through
the ~36k documents the syndication seed left unenriched. Uses the `claude -p`
CLI so it rides the existing subscription credential in ~/.claude — no API key.

WHY DOCUMENTS ARE BATCHED PER INVOCATION (the whole design turns on this):
every `claude -p` call loads the Claude Code harness — system prompt, CLAUDE.md,
tool schemas — measured at ~83k tokens, and the prompt cache hits only
intermittently between calls (measured: 3 identical back-to-back calls cost
$0.334 / $0.025 / $0.334). That is a fixed ~$0.23 of notional usage per
invocation no matter how small the task, so one-doc-per-call would be roughly
$8k of usage for the backlog. Putting BATCH_DOCS documents in each call
amortises it by ~an order of magnitude.

Resumability is in the data, not a state file: work is selected by
enrich_status, so killing and re-running this is always safe and never repeats
finished work.

Rate limits are the real ceiling, not money — the subscription rejects
over-plan requests rather than billing them. Sustained refusals are therefore
treated as "the window is exhausted": the run stops cleanly and the next timer
firing picks up where it left off.
"""
from __future__ import annotations

import argparse
import json
import logging
import os
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

import db
import enrich

log = logging.getLogger("enrich_claude")

CLAUDE_BIN = os.environ.get("CLAUDE_BIN", os.path.expanduser("~/.local/bin/claude"))
MODEL = os.environ.get("CLAUDE_MODEL", "sonnet")
# Documents per invocation. Higher amortises the ~83k-token harness overhead
# better but loses more work when one response is unparseable, and grows output
# per call (roughly 400 tokens/doc against Sonnet's 128k output ceiling).
BATCH_DOCS = int(os.environ.get("BATCH_DOCS", "25"))
CONCURRENCY = int(os.environ.get("ENRICH_CONCURRENCY", "6"))
CALL_TIMEOUT = int(os.environ.get("CLAUDE_TIMEOUT", "900"))
# Per-doc text cap. Lower than enrich.py's 24000 because batching multiplies it.
MAXCHARS = int(os.environ.get("ENRICH_MAXCHARS_BATCH", "6000"))
MAX_BATCHES = int(os.environ.get("MAX_BATCHES", "0"))        # 0 = until drained
MAX_SECONDS = int(os.environ.get("ENRICH_MAX_SECONDS", "0"))  # 0 = no limit
# Consecutive whole-batch failures before concluding the plan window is spent.
FAIL_STREAK_STOP = int(os.environ.get("FAIL_STREAK_STOP", "4"))


def pending(limit: int) -> list[dict]:
    """Documents with text but no enrichment. Newest first — recent material is
    the most useful to have enriched if the run is cut short."""
    conn = db.get_conn()
    rows = conn.execute(
        """SELECT f.id FROM files f
                LEFT JOIN meetings m ON m.id = f.meeting_id
            WHERE f.text_status = 'ok'
              AND f.enrich_status IN ('skipped', 'pending', 'error')
            ORDER BY m.date DESC, f.id DESC
            LIMIT ?""", (limit,)).fetchall()
    return [r["id"] for r in rows]


def _doc_block(i: int, doc: dict) -> str:
    text = (doc.get("fulltext") or "")[:MAXCHARS]
    return (
        f"--- DOKUMENT {i} ---\n"
        f"Gremium: {doc.get('body_name') or 'unbekannt'}\n"
        f"Sitzung: {doc.get('meeting_title') or ''} ({doc.get('meeting_date') or ''})\n"
        f"Dokumenttyp: {doc.get('doc_type') or ''}\n"
        f"Bezeichnung: {doc.get('label') or ''}\n"
        f"Text:\n\"\"\"\n{text}\n\"\"\"\n"
    )


def build_prompt(docs: list[dict]) -> str:
    """Same fields and category vocabulary as enrich._build_prompt, asked for
    N documents at once and keyed by index so results can be matched back."""
    cats = ", ".join(enrich.CATEGORIES)
    blocks = "\n".join(_doc_block(i, d) for i, d in enumerate(docs))
    return (
        f"Du bekommst {len(docs)} Dokumente aus dem Karlsruher Ratsinformationssystem.\n"
        f"Analysiere JEDES Dokument einzeln.\n\n"
        f"{blocks}\n"
        f"Antworte mit EINEM JSON-Objekt, ohne Markdown-Codeblock, ohne weiteren Text.\n"
        f"Schlüssel sind die Dokumentnummern als Strings (\"0\" bis \"{len(docs) - 1}\").\n"
        f"Jeder Wert ist ein Objekt mit genau diesen Feldern:\n"
        f"  summary_de: 2-4 Sätze deutsche Zusammenfassung des Inhalts.\n"
        f"  summary_en: dieselbe Zusammenfassung auf Englisch.\n"
        f"  category: GENAU einer dieser Werte: {cats}.\n"
        f"  topics: Liste von 3-7 kurzen deutschen Schlagworten.\n"
        f"  entities: Objekt mit den Listen people, orgs, locations, je 0-8 Einträge.\n"
        f"Wenn ein Text leer oder unbrauchbar ist: leere Zusammenfassungen und "
        f"category \"Sonstiges\".\n"
        f"Gib ein Objekt für jede Nummer zurück, auch bei unbrauchbarem Text."
    )


def _extract_json(text: str) -> dict:
    """The outermost JSON object in the CLI's answer.

    The model reliably wraps the object in a ```json fence despite being asked
    not to, so brace-slicing is the primary path rather than a fallback — it
    handles the fence, stray prose, and both at once. An empty result raises a
    clear error instead of a bare JSONDecodeError at char 0.
    """
    text = (text or "").strip()
    if not text:
        raise ValueError("claude returned an empty result")
    start, end = text.find("{"), text.rfind("}")
    if start < 0 or end <= start:
        raise ValueError(f"no JSON object in result: {text[:200]!r}")
    obj = json.loads(text[start:end + 1])
    if not isinstance(obj, dict):
        raise ValueError(f"result is {type(obj).__name__}, expected object")
    return obj


def call_claude(prompt: str) -> dict:
    """One `claude -p` invocation. Returns the parsed result map."""
    proc = subprocess.run(
        [CLAUDE_BIN, "-p", prompt, "--model", MODEL, "--output-format", "json"],
        capture_output=True, text=True, timeout=CALL_TIMEOUT,
        # Run from a directory with no CLAUDE.md of its own so the harness
        # prompt stays as small and as cacheable as possible.
        cwd="/tmp",
    )
    if proc.returncode != 0:
        raise RuntimeError(f"claude exited {proc.returncode}: {proc.stderr[:300]}")
    envelope = json.loads(proc.stdout)
    if envelope.get("is_error"):
        raise RuntimeError(f"claude reported error: {str(envelope)[:300]}")
    return _extract_json(envelope.get("result") or "")


def _write(file_id: str, data: dict) -> None:
    with db.write_conn() as c:
        c.execute(
            """UPDATE files SET summary_de=?, summary_en=?, category=?, topics=?,
                                entities=?, enrich_status='ok',
                                enriched_at=datetime('now')
               WHERE id=?""",
            (data["summary_de"], data["summary_en"], data["category"],
             json.dumps(data["topics"], ensure_ascii=False),
             json.dumps(data["entities"], ensure_ascii=False), file_id))
        db.reindex_file(c, file_id)


def run_batch(file_ids: list[str]) -> tuple[int, int]:
    """Enrich one batch. Returns (written, failed)."""
    docs, kept = [], []
    for fid in file_ids:
        d = db.get_document(fid)
        if d:
            docs.append(d)
            kept.append(fid)
    if not docs:
        return 0, 0

    result = call_claude(build_prompt(docs))   # raises -> caller marks the batch

    written = failed = 0
    for i, fid in enumerate(kept):
        entry = result.get(str(i))
        if not isinstance(entry, dict):
            failed += 1
            continue
        try:
            _write(fid, enrich._normalise(entry))
            written += 1
        except Exception as e:  # noqa: BLE001
            log.warning("write failed for %s: %s", fid, e)
            failed += 1
    # A document the model silently skipped stays selectable for the next run
    # rather than being marked done — do not touch its status here.
    return written, failed


def _mark_error(file_ids: list[str]) -> None:
    with db.write_conn() as c:
        c.executemany("UPDATE files SET enrich_status='error' WHERE id=?",
                      [(f,) for f in file_ids])


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout)
    ap = argparse.ArgumentParser(description="Bulk enrichment via headless Claude Code")
    ap.add_argument("--batches", type=int, default=MAX_BATCHES,
                    help="max invocations this run (0 = until drained)")
    ap.add_argument("--max-seconds", type=int, default=MAX_SECONDS)
    ap.add_argument("--batch-docs", type=int, default=BATCH_DOCS)
    ap.add_argument("--concurrency", type=int, default=CONCURRENCY)
    ap.add_argument("--count", action="store_true", help="report backlog and exit")
    args = ap.parse_args()

    db.init_db()
    if args.count:
        print(f"{len(pending(10_000_000))} documents awaiting enrichment")
        return 0

    if not os.path.exists(CLAUDE_BIN):
        log.error("claude CLI not found at %s", CLAUDE_BIN)
        return 2

    started = time.monotonic()
    todo = pending(args.batches * args.batch_docs if args.batches else 10_000_000)
    batches = [todo[i:i + args.batch_docs]
               for i in range(0, len(todo), args.batch_docs)]
    log.info("backlog %d docs -> %d batches of %d, concurrency %d",
             len(todo), len(batches), args.batch_docs, args.concurrency)

    totals = {"written": 0, "failed": 0, "batches_ok": 0, "batches_failed": 0}
    streak = 0
    with ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        futures = {}
        it = iter(batches)
        # Keep the pool topped up rather than submitting everything: a run that
        # stops early on rate limits shouldn't have thousands queued behind it.
        for _ in range(args.concurrency):
            b = next(it, None)
            if b:
                futures[pool.submit(run_batch, b)] = b
        while futures:
            done = next(as_completed(futures))
            batch = futures.pop(done)
            try:
                w, f = done.result()
                totals["written"] += w
                totals["failed"] += f
                totals["batches_ok"] += 1
                streak = 0
            except Exception as e:  # noqa: BLE001
                log.warning("batch of %d failed: %s", len(batch), e)
                _mark_error(batch)
                totals["batches_failed"] += 1
                streak += 1
            log.info("  written=%d failed=%d ok_batches=%d bad_batches=%d",
                     totals["written"], totals["failed"],
                     totals["batches_ok"], totals["batches_failed"])
            if streak >= FAIL_STREAK_STOP:
                log.error("%d batches failed in a row — assuming the plan window "
                          "is exhausted; stopping. Next run resumes.", streak)
                break
            if args.max_seconds and (time.monotonic() - started) > args.max_seconds:
                log.info("time budget reached")
                break
            b = next(it, None)
            if b:
                futures[pool.submit(run_batch, b)] = b

    totals["remaining"] = len(pending(10_000_000))
    log.info("done: %s", totals)
    print(totals)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
