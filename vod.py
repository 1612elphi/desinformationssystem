#!/usr/bin/env python3
"""Transcribe archived council VODs and link them to meetings + agenda items.

ops/vod-capture.sh downloads finished livestreams into data/vod/. This script, per video:
  <base>.transcript.json  whisper segments [{start, end, text}] (cached; the slow part)
  <base>.vod.json         {video_id, title, started_at, duration, media, meeting_id, tops}
tops maps agenda anchors (top5, top3.1 — same keys as votes.agenda_anchor) to the second
offset where the chair calls that item. vod.json is rebuilt every run, so a meeting
scraped after the download still gets linked.

    python vod.py [--link-only]      # needs faster-whisper unless --link-only
"""
from __future__ import annotations

import argparse
import functools
import glob
import json
import os
import re
import sys
from datetime import datetime
from typing import Optional
from zoneinfo import ZoneInfo

import db

VOD_DIR = os.environ.get("VOD_DIR", os.path.join(os.path.dirname(db.DB_PATH), "vod"))
WHISPER_MODEL = os.environ.get("WHISPER_MODEL", "large-v3-turbo")
# int8 on the box's Tesla P4: ~11x realtime on a full session, vs 1.4x on 16 CPU threads.
WHISPER_DEVICE = os.environ.get("WHISPER_DEVICE", "auto")
WHISPER_COMPUTE = os.environ.get("WHISPER_COMPUTE", "int8")
WHISPER_THREADS = int(os.environ.get("WHISPER_THREADS", "16"))
BERLIN = ZoneInfo("Europe/Berlin")

_TOP_RE = re.compile(r"\b(?:Tagesordnungspunkt|TOP|Punkt)\s+(\d+(?:\.\d+)*)", re.I)
_CALL_RE = re.compile(r"(?:rufe\w*\s.{0,20}?auf|aufrufen|kommen wir zu|wir kommen zu|ich eröffne)", re.I)


@functools.cache
def _model():
    from faster_whisper import WhisperModel

    try:
        return WhisperModel(WHISPER_MODEL, device=WHISPER_DEVICE,
                            compute_type=WHISPER_COMPUTE, cpu_threads=WHISPER_THREADS)
    except Exception as e:  # a driver/CUDA-library change must not stall the backlog
        print(f"  {WHISPER_DEVICE} unavailable ({e}), falling back to CPU", file=sys.stderr)
        return WhisperModel(WHISPER_MODEL, device="cpu",
                            compute_type="int8", cpu_threads=WHISPER_THREADS)


def transcribe(media: str) -> list[dict]:
    segments, _ = _model().transcribe(media, language="de", vad_filter=True)
    return [{"start": round(s.start, 1), "end": round(s.end, 1), "text": s.text.strip()}
            for s in segments]


def match_meeting(release_ts: Optional[int]) -> Optional[str]:
    """The Gemeinderat meeting held on the stream's (Berlin) start date, if exactly one."""
    if not release_ts:
        return None
    day = datetime.fromtimestamp(release_ts, BERLIN).date().isoformat()
    rows = db.get_conn().execute(
        "SELECT id FROM meetings WHERE date = ? AND body_name LIKE 'Gemeinderat%'", (day,)
    ).fetchall()
    return rows[0]["id"] if len(rows) == 1 else None


def top_offsets(segments: list[dict], anchors: set[str]) -> dict[str, float]:
    """Earliest offset per agenda item, preferring the chair calling it over a bare mention.

    One segment can hold both ("Das war der Top 1 und ich rufe jetzt auf den
    Tagesordnungspunkt 2"), so the call phrase is looked for in the text before each number.
    """
    best: dict[str, tuple[int, float]] = {}
    for s in segments:
        for m in _TOP_RE.finditer(s["text"]):
            a = f"top{m.group(1)}"
            if a not in anchors:
                continue
            score = 1 if _CALL_RE.search(s["text"][: m.start()]) else 0
            if a not in best or score > best[a][0]:
                best[a] = (score, s["start"])
    return {a: v[1] for a, v in sorted(best.items())}


def _archived_ids() -> set[str]:
    # yt-dlp appends "youtube <id>" only after the merged file is complete,
    # so this excludes downloads still in progress.
    try:
        with open(os.path.join(VOD_DIR, "archive.txt")) as f:
            return {line.split()[1] for line in f if line.strip()}
    except FileNotFoundError:
        return set()


def process(info_path: str, link_only: bool) -> None:
    base = info_path[: -len(".info.json")]
    info = json.load(open(info_path, encoding="utf-8"))
    media = next((p for p in glob.glob(glob.escape(base) + ".*") if not p.endswith(".json")), None)
    if not media:
        print(f"  no media for {base}", file=sys.stderr)
        return

    tpath = base + ".transcript.json"
    if not os.path.exists(tpath):
        if link_only:
            segments = []
        else:
            print(f"  transcribing {os.path.basename(media)} ({WHISPER_MODEL})")
            segments = transcribe(media)
            with open(tpath + ".tmp", "w", encoding="utf-8") as f:
                json.dump(segments, f, ensure_ascii=False)
            os.replace(tpath + ".tmp", tpath)
    else:
        segments = json.load(open(tpath, encoding="utf-8"))

    meeting_id = match_meeting(info.get("release_timestamp") or info.get("timestamp"))
    anchors = set()
    if meeting_id:
        anchors = {r["anchor"] for r in db.get_conn().execute(
            "SELECT anchor FROM agenda_items WHERE meeting_id = ?", (meeting_id,))}
    ts = info.get("release_timestamp")
    vod = {
        "video_id": info["id"],
        "title": info.get("title"),
        "started_at": datetime.fromtimestamp(ts, BERLIN).isoformat() if ts else None,
        "duration": info.get("duration"),
        "media": os.path.basename(media),
        "meeting_id": meeting_id,
        "tops": top_offsets(segments, anchors),
    }
    with open(base + ".vod.json", "w", encoding="utf-8") as f:
        json.dump(vod, f, ensure_ascii=False, indent=1)
    print(f"  {info['id']} -> {meeting_id or 'no meeting'}, {len(vod['tops'])}/{len(anchors)} TOPs located")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--link-only", action="store_true", help="skip transcription, relink only")
    args = ap.parse_args()
    done = _archived_ids()
    for info_path in sorted(glob.glob(os.path.join(VOD_DIR, "*.info.json"))):
        vid = json.load(open(info_path, encoding="utf-8"))["id"]
        if vid in done:
            process(info_path, args.link_only)
    return 0


def _selfcheck() -> None:
    segs = [{"start": 10.0, "text": "Wie schon bei Punkt 5 gesagt, dazu später."},
            {"start": 20.0, "text": "Ich rufe auf Tagesordnungspunkt 3."},
            {"start": 30.0, "text": "Dann kommen wir zu TOP 3.1, Sanierungsbeirat."},
            {"start": 40.0, "text": "Das war der Top 3 und ich rufe jetzt auf den Tagesordnungspunkt 5."},
            {"start": 50.0, "text": "Tagesordnungspunkt 31 ist abgesetzt."}]
    got = top_offsets(segs, {"top3", "top3.1", "top5", "top31"})
    # top5 moves from the bare mention at 10 to the chair's call at 40; top3 keeps its call at 20
    assert got == {"top3": 20.0, "top3.1": 30.0, "top31": 50.0, "top5": 40.0}, got
    assert top_offsets(segs, {"top7"}) == {}


if __name__ == "__main__":
    _selfcheck()
    raise SystemExit(main())
