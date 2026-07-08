"""
Desinformationssystem — live-ticker watcher.

When a council session is in progress (db.live_meetings), find its live-ticker id on
the /live selection page, fetch the ticker, and for each agenda item pull the result
text + the vote-tally image. New/changed images are parsed (vote_parse) into structured
Ja/Nein/Enthaltung + per-member roll-call and stored in the `votes` table.

The ticker is plain auto-refresh HTML ("Bitte aktualisieren Sie die Seite regelmäßig").
Polls fast while live, slow when idle. Vote images live at
  …/db/ratsinformation/{liveId}/{topId}/file/highres{n}_TOP {N}.jpg  (note the space).
"""
from __future__ import annotations

import hashlib
import logging
import os
import re
import sys
import time
from typing import Optional

from bs4 import BeautifulSoup

import db
import scraper
import vote_parse

log = logging.getLogger("ticker")

RIS = scraper.RIS
POLL = int(os.environ.get("TICKER_POLL", "120"))
IDLE_POLL = int(os.environ.get("TICKER_IDLE_POLL", "900"))

_IMG_RE = re.compile(
    r"https://sitzungskalender\.karlsruhe\.de/db/ratsinformation/\d+/\d+/file/highres[^\"']+\.jpg",
    re.IGNORECASE,
)


def candidate_live_ids(html: str) -> list[str]:
    """live-ticker ids on the /live selection page that are currently running."""
    soup = BeautifulSoup(html, "lxml")
    flagged, allids = [], []
    for a in soup.find_all("a", href=re.compile(r"/\d+/live")):
        m = re.search(r"/(\d+)/live", a["href"])
        if not m:
            continue
        allids.append(m.group(1))
        ctx, anc = "", a
        for _ in range(3):
            anc = anc.parent
            if anc is None:
                break
            ctx = anc.get_text(" ", strip=True)
            if len(ctx) > 20:
                break
        if re.search(r"(jetzt\s*live|sitzung läuft|läuft gerade)", ctx, re.I):
            flagged.append(m.group(1))
    # prefer those explicitly marked live; else fall back to all (capped)
    return sorted(set(flagged)) or sorted(set(allids))[:6]


def parse_ticker(html: str) -> dict:
    """Header committee+date and per-TOP {top_label, image_url, result_text}."""
    soup = BeautifulSoup(html, "lxml")
    text = soup.get_text(" ", strip=True)
    date = scraper._parse_german_date(text)
    # committee name sits right before "(öffentlich…) am <date>"; exclude '.' so the
    # capture can't run backwards across the preceding nav/sentence text.
    cm = re.search(
        r"([A-ZÄÖÜ][^.<>()]{2,60}?)\s*\((?:öffentlich|nicht öffentlich|öffentlich/nicht öffentlich)\)\s*am\s+\d",
        text,
    )
    committee = cm.group(1).strip() if cm else None

    tops: dict[str, dict] = {}
    for url in _IMG_RE.findall(html):
        mt = re.search(r"_TOP\s*([\dA-Za-z.]+)\.jpg", url)
        if not mt:
            continue
        label = f"TOP {mt.group(1)}"
        tops.setdefault(label, {"top_label": label, "image_url": url, "result_text": None})

    # best-effort result text: the "Ergebnis ... <p>TEXT</p>" within a single TOP block
    # (negative lookahead stops the match before the next "TOP n" so blocks don't bleed)
    for m in re.finditer(
            r"TOP\s*([\dA-Za-z.]+)(?:(?!TOP\s*[\dA-Za-z.]+).){0,800}?Ergebnis\s*</strong>.{0,80}?>\s*([^<]{3,80})",
            html, re.S | re.I):
        label = f"TOP {m.group(1)}"
        res = re.sub(r"\s+", " ", m.group(2)).strip()
        if label in tops and not tops[label]["result_text"]:
            tops[label]["result_text"] = res
    return {"committee": committee, "date": date, "tops": list(tops.values())}


def _match_meeting(tk: dict, live: list[dict]) -> Optional[str]:
    # require a positive date match (a ticker without a parseable date is not trusted)
    if not tk.get("date"):
        return None
    same_date = [m for m in live if m.get("date") == tk["date"]]
    if len(same_date) == 1:
        return same_date[0]["id"]  # one live session that day — unambiguous
    # multiple live sessions same day: disambiguate by committee name
    for m in same_date:
        if tk.get("committee") and (m.get("body_name") or "").startswith(tk["committee"][:12]):
            return m["id"]
    return None


def _process_vote(f: "scraper.Fetcher", meeting_id: str, top: dict, counts: dict) -> None:
    label = top["top_label"]
    anchor = "top" + "".join(ch for ch in label if ch.isdigit())
    if anchor == "top":  # no TOP number -> can't place under an agenda item
        return
    url = top["image_url"].replace(" ", "%20")
    r = f.get(url, binary=True)
    if not r:
        counts["errors"] += 1
        return
    sha = hashlib.sha256(r.content).hexdigest()
    if db.vote_image_sha(meeting_id, anchor) == sha:
        counts["unchanged"] += 1
        return
    parsed = vote_parse.parse_vote_image(r.content) or {}
    with db.write_conn() as c:
        db.upsert_vote(c, {
            "meeting_id": meeting_id, "agenda_anchor": anchor, "top_label": label,
            "result_text": top.get("result_text"),
            "ja": parsed.get("ja"), "nein": parsed.get("nein"), "enthaltung": parsed.get("enthaltung"),
            "members": parsed.get("members"), "source": "live",
            "image_url": top["image_url"],
            # only persist the dedup sha on success; on failure leave it NULL so the
            # next poll retries this TOP (and the result_text is still captured)
            "image_sha": sha if parsed else None,
        })
    if parsed:
        counts["parsed"] += 1
        log.info("vote %s %s -> %s/%s/%s", meeting_id, label,
                 parsed.get("ja"), parsed.get("nein"), parsed.get("enthaltung"))
    else:
        counts["errors"] += 1


def run_once() -> dict:
    counts = {"parsed": 0, "unchanged": 0, "errors": 0, "tops": 0}
    live = db.live_meetings()
    if not live:
        return counts
    f = scraper.Fetcher()
    try:
        r = f.get(f"{RIS}/live")
        if not r:
            return counts
        want = {m["id"] for m in live}
        for lid in candidate_live_ids(r.text):
            if not want:
                break  # all live meetings matched — stop probing candidates
            rt = f.get(f"{RIS}/{lid}/live")
            if not rt:
                continue
            tk = parse_ticker(rt.text)
            mid = _match_meeting(tk, live)
            if not mid:
                continue
            want.discard(mid)
            for top in tk["tops"]:
                counts["tops"] += 1
                _process_vote(f, mid, top, counts)
        return counts
    finally:
        f.close()


def main() -> int:
    logging.basicConfig(level=logging.INFO,
                        format="%(asctime)s %(levelname)s %(name)s: %(message)s", stream=sys.stdout)
    db.init_db()
    log.info("ticker watcher start (poll=%ds idle=%ds, model=%s)", POLL, IDLE_POLL, vote_parse.VISION_MODEL)
    while True:
        try:
            c = run_once()
        except Exception as e:  # noqa: BLE001
            log.warning("ticker run error: %s", e)
            c = {"parsed": 0}
        live = db.live_meetings()
        nap = POLL if live else IDLE_POLL
        if c.get("parsed") or live:
            log.info("ticker: %s | live=%d sleep=%ds", c, len(live), nap)
        time.sleep(nap)


if __name__ == "__main__":
    raise SystemExit(main())
