"""
Desinformationssystem — slow PDF backfill for seeded rows.

seed.py writes files rows without downloading anything (local_path NULL), so
/api/file 302-redirects to web1 for those. This walks the backlog at the normal
politeness throttle and caches the real PDFs, so the archive becomes
self-hosted. Safe to stop and re-run: it only ever picks rows still missing a
local file, so progress is implicit in the data.

Priority order matters. The syndication dump only carries extracted text for
*paper* files (99% covered); meeting-level documents — Einladungen,
Niederschriften, Abstimmungsergebnisse — have none at all. Those rows gain both
a cached PDF *and* fulltext from our own pdftotext, so they go first; rows that
already have text only gain the cached bytes and can wait.

Cost guard: filestore.process_file sets enrich_status='pending' on every write,
which would silently queue tens of thousands of LLM calls the next time the
daily loop runs. Rows that were seeded 'skipped' are restored to 'skipped'
afterwards — enrichment stays an explicit, separate decision.
"""
from __future__ import annotations

import argparse
import logging
import os
import sys

import db
import filestore

log = logging.getLogger("backfill_pdfs")

# Bound a single run so an overnight job can't sprawl indefinitely.
MAX_FILES = int(os.environ.get("BACKFILL_MAX_FILES", "0"))       # 0 = no cap
MAX_SECONDS = int(os.environ.get("BACKFILL_MAX_SECONDS", "0"))   # 0 = no limit


def pending(limit: int = 0) -> list[dict]:
    """Rows with no cached PDF, text-less ones first (they gain the most)."""
    conn = db.get_conn()
    q = """SELECT id, url, label, doc_type, filename, agenda_anchor, meeting_id,
                  text_status, enrich_status
             FROM files
            WHERE (local_path IS NULL OR local_path = '')
              AND url LIKE 'http%'
            ORDER BY CASE WHEN text_status = 'ok' THEN 1 ELSE 0 END,
                     meeting_id DESC, id DESC"""
    if limit:
        q += f" LIMIT {int(limit)}"
    return [dict(r) for r in conn.execute(q)]


def run(limit: int = 0, max_seconds: int = 0) -> dict:
    import time
    started = time.monotonic()
    rows = pending(limit)
    counts = {"files_new": 0, "files_updated": 0, "files_seen": 0,
              "text_ok": 0, "errors": 0}
    log.info("backfill: %d files pending a cached PDF", len(rows))
    f = filestore.Fetcher()
    done = 0
    try:
        for i, r in enumerate(rows, 1):
            if max_seconds and (time.monotonic() - started) > max_seconds:
                log.info("time budget reached after %d files", i - 1)
                break
            fmeta = {
                "id": r["id"], "url": r["url"], "label": r["label"] or "",
                "agenda_anchor": r["agenda_anchor"],
                "doc_type": r["doc_type"] or "Sonstiges",
                "filename": r["filename"] or f"{r['id']}.pdf",
            }
            was = r["enrich_status"]
            try:
                filestore.process_file(f, fmeta, r["meeting_id"], counts)
                done += 1
            except Exception as e:  # noqa: BLE001
                log.warning("file %s failed: %s", r["id"], e)
                counts["errors"] += 1
                continue
            # Undo process_file's enrich_status='pending' for rows that were
            # deliberately seeded as skipped — enrichment is a separate call.
            if was == "skipped":
                with db.write_conn() as c:
                    c.execute("UPDATE files SET enrich_status='skipped' WHERE id=?",
                              (r["id"],))
            if i % 100 == 0:
                log.info("  %d/%d  new=%d text_ok=%d errors=%d", i, len(rows),
                         counts.get("files_new", 0), counts.get("text_ok", 0),
                         counts["errors"])
    finally:
        f.close()
    counts["processed"] = done
    counts["remaining"] = max(0, len(rows) - done)
    return counts


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout,
    )
    ap = argparse.ArgumentParser(description="Cache PDFs for seeded rows")
    ap.add_argument("--limit", type=int, default=MAX_FILES,
                    help="max files this run (0 = all pending)")
    ap.add_argument("--max-seconds", type=int, default=MAX_SECONDS,
                    help="stop after N seconds (0 = no limit)")
    ap.add_argument("--count", action="store_true",
                    help="just report how many are pending, then exit")
    args = ap.parse_args()

    db.init_db()
    if args.count:
        n = len(pending())
        print(f"{n} files pending a cached PDF")
        return 0
    counts = run(limit=args.limit, max_seconds=args.max_seconds)
    log.info("backfill done: %s", counts)
    print(counts)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
