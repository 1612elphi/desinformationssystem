#!/usr/bin/env python3
"""Backfill geo tags (street links + primary Stadtteil) over enriched documents.

Idempotent and re-runnable: processes files with enrich_status='ok' and geo_status not 'ok',
newest first, so new documents get picked up on the next run. Needs TYPESAFE_API_KEY (geo.py
calls Jev). Whole-corpus cost is a few dollars of input tokens; output is free.

    TYPESAFE_API_KEY=... python geo_backfill.py [--limit N] [--batch 200] [--workers 8]
"""
from __future__ import annotations

import argparse
import sys
from concurrent.futures import ThreadPoolExecutor

import db
import geo
import typesafe


def _tag(doc: dict) -> dict:
    """Run both geo passes for one doc. Returns the row to persist (never raises to the pool)."""
    try:
        d = dict(doc)
        d["fulltext"] = db.file_fulltext(doc["id"])
        streets = geo.confirm_streets(d)
        district, conf = geo.classify_district(d)
        return {"id": doc["id"], "streets": streets, "district": district,
                "conf": conf, "status": "ok"}
    except Exception as e:  # one bad doc must not abort the batch; mark it for retry
        print(f"  error {doc['id']}: {e}", file=sys.stderr)
        return {"id": doc["id"], "streets": [], "district": None, "conf": None, "status": "error"}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=0, help="max docs this run (0 = all pending)")
    ap.add_argument("--batch", type=int, default=200)
    ap.add_argument("--workers", type=int, default=8)
    args = ap.parse_args()

    if not typesafe.available():
        print("TYPESAFE_API_KEY not set", file=sys.stderr)
        return 2
    db.init_db()

    done = 0
    while True:
        take = args.batch if not args.limit else min(args.batch, args.limit - done)
        if take <= 0:
            break
        docs = db.docs_needing_geo(take)
        if not docs:
            break
        with ThreadPoolExecutor(max_workers=args.workers) as ex:
            results = list(ex.map(_tag, docs))
        with db.write_conn() as conn:
            for r in results:
                db.set_geo(conn, r["id"], r["streets"], r["district"], r["conf"], r["status"])
        done += len(results)
        tagged = sum(1 for r in results if r["district"])
        streets = sum(len(r["streets"]) for r in results)
        print(f"processed {done}: +{tagged} with district, +{streets} street links "
              f"(last batch {len(results)})")
        if len(docs) < take:
            break
    print(f"done: {done} documents")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
