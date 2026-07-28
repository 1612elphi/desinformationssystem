"""
Desinformationssystem — one-time bulk seed from the karlsruhe-oparl-syndication
dump (https://github.com/maxliesegang/karlsruhe-oparl-syndication, MIT).

Why: a native full backfill via oparl.py --full is ~14k papers with a ~4
request/paper fan-out at the 1.5s politeness throttle — call it 1.5 days of
sustained load on web1 plus ~70 GB of PDFs. The syndication repo publishes the
same OParl objects (byte-identical shape, same /oparl/ prefix quirk) plus
already-extracted PDF text, as a 632 MB git clone. Seeding from it once is both
far faster and considerably kinder to the city's server; ongoing freshness then
comes from our own modified_since deltas (oparl.py), which is why this is a
seed and not a dependency.

What it does NOT do:
  - download PDFs (files.local_path stays NULL — /api/file already 302-redirects
    to the source in that case; backfill_pdfs.py fills them in afterwards)
  - spend any LLM budget (rows land with enrich_status='skipped', which
    scraper._enrich_pending never selects)
  - touch committee rosters (HTML-only; OParl carries no party data)
  - degrade rows the live pipeline already owns (existing local_path, sha256,
    fulltext and enrich_status are preserved — see _seed_file)

Layout consumed (dump root = --dir):
  meetings/<id>.json      full OParl Meeting incl. embedded agendaItem[]
  papers/<id>.json        full OParl Paper incl. consultation[] + files
  file-contents/<id>.txt  extracted text, keyed by the OParl *file* id, which is
                          int() of our zero-padded files.id ("00010003" -> 10003)
  generation-manifest.json  completedAt -> the watermark deltas resume from
"""
from __future__ import annotations

import argparse
import glob
import json
import logging
import os
import sys
from typing import Optional

import db
import filestore
import oparl
import submitter

log = logging.getLogger("seed")


def _txt_path(dump: str, fid: str) -> str:
    """files.id '00010003' -> file-contents/10003.txt (OParl file id)."""
    try:
        return os.path.join(dump, "file-contents", f"{int(fid)}.txt")
    except ValueError:
        return os.path.join(dump, "file-contents", f"{fid}.txt")


class SeedIngest(oparl.Ingest):
    """oparl.Ingest with the network file-fetch swapped for a dump reader."""

    def __init__(self, cli, recheck_cutoff, counts, dump: str) -> None:
        super().__init__(cli, recheck_cutoff, counts)
        self.dump = dump
        self.process_file = self._seed_file

    def _seed_file(self, _fetcher, fmeta: dict, meeting_id: str, counts: dict,
                   recheck: bool = False) -> bool:
        """Write a files row from the dump. Signature matches
        filestore.process_file so Ingest's call sites are unchanged."""
        fid = fmeta["id"]
        conn = db.get_conn()
        existing = conn.execute(
            "SELECT id, local_path, text_status, fulltext FROM files WHERE id=?",
            (fid,)).fetchone()

        # A row the live pipeline already downloaded and extracted is
        # authoritative — only refresh its association/label fields, never
        # overwrite its text or blank its local_path.
        if existing and existing["text_status"] == "ok":
            with db.write_conn() as c:
                c.execute(
                    """UPDATE files SET meeting_id=?, agenda_anchor=?, label=?,
                                        doc_type=? WHERE id=?""",
                    (meeting_id, fmeta["agenda_anchor"], fmeta["label"],
                     fmeta["doc_type"], fid))
                db.reindex_file(c, fid)
            counts["files_kept"] = counts.get("files_kept", 0) + 1
            return False

        path = _txt_path(self.dump, fid)
        try:
            with open(path, encoding="utf-8", errors="replace") as fh:
                text = fh.read()
            status = "ok" if text.strip() else "empty"
        except FileNotFoundError:
            text, status = "", "missing"
        if status == "ok":
            counts["text_ok"] += 1
        else:
            # image-only PDFs (vote tallies, scans) — ~5% of the dump
            counts["text_missing"] = counts.get("text_missing", 0) + 1

        subs = json.dumps(submitter.derive(fmeta["label"], text, fmeta["doc_type"]),
                          ensure_ascii=False)
        with db.write_conn() as c:
            c.execute(
                """INSERT INTO files(id,url,meeting_id,agenda_anchor,label,doc_type,
                                     filename,text_status,fulltext,submitters,
                                     enrich_status,first_seen)
                   VALUES(?,?,?,?,?,?,?,?,?,?,'skipped',datetime('now'))
                   ON CONFLICT(id) DO UPDATE SET
                     url=excluded.url, meeting_id=excluded.meeting_id,
                     agenda_anchor=excluded.agenda_anchor, label=excluded.label,
                     doc_type=excluded.doc_type, filename=excluded.filename,
                     text_status=excluded.text_status, fulltext=excluded.fulltext,
                     submitters=excluded.submitters""",
                (fid, fmeta["url"], meeting_id, fmeta["agenda_anchor"],
                 fmeta["label"], fmeta["doc_type"], fmeta["filename"], status,
                 text, subs))
            own, refs = filestore.extract_vorlagen(fmeta["label"], text,
                                                   fmeta["doc_type"])
            db.set_file_vorlagen(c, fid, own, refs)
            db.reindex_file(c, fid)
        counts["files_new"] = counts.get("files_new", 0) + 1
        return True


class LocalClient:
    """oparl.Client stand-in: resolves object URLs to files in the dump.

    Only /meetings/<n> is served — agenda items are already embedded in the
    meeting objects, so seeding meetings before papers leaves Ingest.aimap warm
    and _resolve_anchor never needs a lookup. Anything else returns None, which
    every caller treats as 'unresolved' rather than an error.
    """

    def __init__(self, dump: str) -> None:
        self.dump = dump
        self.f = None          # never used: process_file is overridden
        self.misses = 0

    def get_json(self, url: str) -> Optional[dict]:
        num = oparl._tail(url)
        if "/meetings/" in url:
            p = os.path.join(self.dump, "meetings", f"{num}.json")
            if os.path.exists(p):
                with open(p, encoding="utf-8") as fh:
                    return json.load(fh)
        self.misses += 1
        return None

    def close(self) -> None:
        pass


def _sorted_ids(dump: str, sub: str) -> list[str]:
    """Dump entries by numeric id, so a resumed run is deterministic."""
    paths = glob.glob(os.path.join(dump, sub, "*.json"))
    def key(p):
        stem = os.path.splitext(os.path.basename(p))[0]
        return int(stem) if stem.isdigit() else -1
    return sorted(paths, key=key)


def _slice(paths: list[str], limit: int, newest: bool) -> list[str]:
    if not limit:
        return paths
    return paths[-limit:] if newest else paths[:limit]


def seed(dump: str, limit: int = 0, skip_papers: bool = False,
         skip_meetings: bool = False, newest: bool = False) -> dict:
    db.init_db()
    counts = {"meetings": 0, "papers": 0, "files_new": 0, "files_seen": 0,
              "text_ok": 0, "enriched": 0, "errors": 0}
    cli = LocalClient(dump)
    # recheck_cutoff far in the future is meaningless here (nothing is
    # re-downloaded); pass an empty-ish past date so no branch treats the
    # historic archive as 'recent'.
    ing = SeedIngest(cli, "1900-01-01", counts, dump)

    if not skip_meetings:
        paths = _slice(_sorted_ids(dump, "meetings"), limit, newest)
        log.info("seeding %d meetings", len(paths))
        for i, p in enumerate(paths, 1):
            try:
                with open(p, encoding="utf-8") as fh:
                    ing.ingest_meeting(json.load(fh))
            except Exception as e:  # noqa: BLE001
                log.warning("meeting %s failed: %s", os.path.basename(p), e)
                counts["errors"] += 1
            if i % 250 == 0:
                log.info("  meetings %d/%d  files_new=%d", i, len(paths),
                         counts.get("files_new", 0))

    if not skip_papers:
        paths = _slice(_sorted_ids(dump, "papers"), limit, newest)
        log.info("seeding %d papers", len(paths))
        for i, p in enumerate(paths, 1):
            try:
                with open(p, encoding="utf-8") as fh:
                    ing.ingest_paper(json.load(fh))
            except Exception as e:  # noqa: BLE001
                log.warning("paper %s failed: %s", os.path.basename(p), e)
                counts["errors"] += 1
            if i % 500 == 0:
                log.info("  papers %d/%d  files_new=%d", i, len(paths),
                         counts.get("files_new", 0))

    counts["client_misses"] = cli.misses
    return counts


def set_watermark(dump: str) -> Optional[str]:
    """Deltas must resume from when the dump was generated, not from now, or
    everything changed between generation and seeding is skipped forever."""
    p = os.path.join(dump, "generation-manifest.json")
    try:
        with open(p, encoding="utf-8") as fh:
            stamp = json.load(fh).get("completedAt")
    except (OSError, ValueError):
        return None
    if not stamp:
        return None
    with db.write_conn() as c:
        db.set_meta(c, "oparl_since", stamp)
    return stamp


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout,
    )
    ap = argparse.ArgumentParser(description="Bulk-seed from the syndication dump")
    ap.add_argument("--dir", required=True, help="dump root (the repo's docs/ dir)")
    ap.add_argument("--limit", type=int, default=0, help="only N of each kind")
    ap.add_argument("--newest", action="store_true",
                    help="with --limit, take the NEWEST N (validation runs; the "
                         "oldest entries are pre-digital stubs and unrepresentative)")
    ap.add_argument("--skip-meetings", action="store_true")
    ap.add_argument("--skip-papers", action="store_true")
    ap.add_argument("--no-watermark", action="store_true",
                    help="don't set meta oparl_since from the dump manifest")
    args = ap.parse_args()

    if not os.path.isdir(args.dir):
        print(f"no such dump dir: {args.dir}", file=sys.stderr)
        return 2
    counts = seed(args.dir, limit=args.limit, skip_papers=args.skip_papers,
                  skip_meetings=args.skip_meetings, newest=args.newest)
    if not args.no_watermark and not args.limit:
        stamp = set_watermark(args.dir)
        counts["watermark"] = stamp
    log.info("seed done: %s", counts)
    print(counts)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
