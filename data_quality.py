#!/usr/bin/env python3
"""Post-sync data-quality report: vote roll-call consistency + orphan files.

- Roll-call consistency: the big ja/nein/enthaltung counters are read from the numeric display
  and are reliable; the per-member tile grid is parsed by a vision model and drifts, so many
  stored roll-calls disagree with their own counters. members_ok flags each one; this recomputes
  it over all votes and reports the rate.
- Orphan files: PDFs attached to no meeting and no Vorlage (riski's find_unreferenced_files).

    python data_quality.py
"""
import db


def main() -> int:
    db.init_db()
    with db.write_conn() as conn:
        n = db.refresh_vote_consistency(conn)   # backfill members_ok over every vote
    c = db.vote_consistency()
    rc = c["with_rollcall"] or 1
    print(f"vote roll-calls: {c['with_rollcall']}  consistent: {c['consistent']}  "
          f"mismatch: {c['mismatch']} ({c['mismatch'] / rc * 100:.0f}%)  [recomputed {n}]")

    orphans = db.orphan_files()
    print(f"orphan files (no meeting, no Vorlage): {len(orphans)}")
    for o in orphans[:10]:
        print(f"  {o['id']}  {(o['label'] or '')[:40]:<40}  {o['url']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
