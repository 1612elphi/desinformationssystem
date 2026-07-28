"""
Desinformationssystem — crawler + downloader + text extractor.

Source: https://sitzungskalender.karlsruhe.de  (SOMACOS "Session" RIS; the OParl
API is incomplete/unreliable, so we scrape the HTML, which is rich and stable).

Flow per run:
  1. /gremien                         -> committee id -> name map
  2. /termine?j=YYYY&m=MM&g=          -> discover termin-NNNNN ids in the window
  3. /db/ratsinformation/termin-N     -> meeting metadata, agenda items, PDF links
  4. download each new PDF (web1.karlsruhe.de), sha256-dedupe, pdftotext, [OCR]
  5. enrich.enrich_file()             -> LLM summary / topics / entities (optional)

web1.karlsruhe.de rate-limits bursts (returns 404 pages once tripped), so every
request is throttled and retried with backoff.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import logging
import os
import re
import subprocess
import sys
from datetime import date, datetime, timedelta
from typing import Optional
from urllib.parse import urljoin, urlsplit

from bs4 import BeautifulSoup

import db
import filestore
import submitter
# Shared plumbing lives in filestore.py (factored out 2026-07-24 so oparl.py can
# reuse it). Re-exported here so existing imports (ticker, web, tests) keep working.
from filestore import (  # noqa: F401
    DOC_TYPE_RULES, ENABLE_OCR, FLARESOLVERR_URL, Fetcher, MAX_RETRIES, PDF_DIR,
    PDFTOTEXT_MAXCHARS, REQUEST_INTERVAL, USER_AGENT, _VORLAGE_RE, _ocr, _sha256,
    _valid_vorlage, classify, extract_text, extract_vorlagen,
)

log = logging.getLogger("scraper")

SITE = os.environ.get("SITE_BASE", "https://sitzungskalender.karlsruhe.de")
RIS = f"{SITE}/db/ratsinformation"
BACKFILL_MONTHS = int(os.environ.get("BACKFILL_MONTHS", "12"))
# Always re-crawl meetings held in the last RECHECK_DAYS so late-added voting
# results (Abstimmungsergebnis) / minutes are picked up the next day, and re-check
# their existing files for in-place updates via a cheap HEAD compare.
RECHECK_DAYS = int(os.environ.get("RECHECK_DAYS", "21"))
# Also crawl this many months into the future, to capture upcoming sessions.
FORWARD_MONTHS = int(os.environ.get("FORWARD_MONTHS", "3"))
ENABLE_LLM = os.environ.get("ENABLE_LLM", "1") == "1"

GERMAN_MONTHS = {
    "januar": 1, "februar": 2, "märz": 3, "maerz": 3, "april": 4, "mai": 5,
    "juni": 6, "juli": 7, "august": 8, "september": 9, "oktober": 10,
    "november": 11, "dezember": 12,
}

# ---------------------------------------------------------------------------
# Pure parsers (testable against saved HTML)
# ---------------------------------------------------------------------------

def parse_committees(html: str) -> dict[str, str]:
    """organisation-gr-NNNNN -> committee name."""
    soup = BeautifulSoup(html, "lxml")
    out: dict[str, str] = {}
    for a in soup.select('a[href*="organisation-gr-"]'):
        m = re.search(r"organisation-gr-\d+", a["href"])
        if not m:
            continue
        name = re.sub(r"\s+", " ", a.get_text(" ", strip=True))
        if name:
            out[m.group(0)] = name
    return out


def parse_calendar_termine(html: str) -> list[str]:
    """Distinct termin-NNNNN ids present on a calendar month page."""
    return sorted(set(re.findall(r"termin-\d+", html)))


def parse_members(html: str) -> list[dict]:
    """Parse a committee org page's member roster (name, party, role, tenure)."""
    soup = BeautifulSoup(html, "lxml")
    out: list[dict] = []
    seen: set[str] = set()
    for tbl in soup.select("table.contenttable"):
        if not tbl.find("a", href=re.compile(r"people-\d+")):
            continue
        for tr in tbl.find_all("tr"):
            a = tr.find("a", href=re.compile(r"people-\d+"))
            if not a:
                continue
            pid = re.search(r"people-(\d+)", a["href"]).group(1)
            if pid in seen:
                continue
            seen.add(pid)
            name = re.sub(r"\s+", " ", a.get_text(" ", strip=True)).strip()
            cells = [re.sub(r"\s+", " ", c.get_text(" ", strip=True))
                     for c in tr.find_all(["td", "th"])]
            info = cells[1] if len(cells) > 1 else ""
            party, role = "", info
            if "," in info:
                party, role = (p.strip() for p in info.split(",", 1))
            since = ""
            for c in cells[2:]:
                if c.lower().startswith(("seit", "bis", "von")) or re.search(r"\d{4}", c):
                    since = c
                    break
            codes = submitter._scan(party) if party else []
            out.append({
                "person_id": pid, "name": name, "party": party,
                "party_code": codes[0] if codes else "", "role": role, "since": since,
            })
        break  # first roster table only
    return out


def _parse_german_date(text: str) -> Optional[str]:
    m = re.search(r"(\d{1,2})\.\s*([A-Za-zäöüÄÖÜ]+)\s*(\d{4})", text)
    if not m:
        return None
    day, mon, year = m.group(1), m.group(2).lower(), m.group(3)
    month = GERMAN_MONTHS.get(mon)
    if not month:
        return None
    try:
        return date(int(year), month, int(day)).isoformat()
    except ValueError:
        return None


def _public_flag(text: str) -> Optional[int]:
    low = text.lower()
    if "nicht öffentlich" in low or "nichtöffentlich" in low:
        return 0
    if "öffentlich" in low:
        return 1
    return None


def _meeting_public(title: str) -> Optional[int]:
    """Meeting titles often read '(öffentlich/nicht öffentlich)' = mixed -> NULL."""
    low = title.lower()
    has_non = "nicht öffentlich" in low or "nichtöffentlich" in low
    # strip the negated phrase before testing for a standalone "öffentlich" —
    # it's a substring of "nicht öffentlich", so testing the raw title would
    # make every purely non-public meeting look mixed.
    rest = low.replace("nicht öffentlich", "").replace("nichtöffentlich", "")
    has_pub = "öffentlich" in rest
    if has_pub and has_non:
        return None
    if has_non:
        return 0
    if has_pub:
        return 1
    return None


def parse_meeting(html: str, url: str, meeting_id: str) -> dict:
    soup = BeautifulSoup(html, "lxml")

    title = ""
    t = soup.find("title")
    if t:
        title = re.sub(r"^Karlsruhe:\s*Ratsinformation\s*-\s*", "", t.get_text(strip=True))
    h1 = soup.find(["h1", "h2"])
    body_name = re.sub(r"\s*\(.*?\)\s*$", "", title).strip() or (h1.get_text(strip=True) if h1 else "")

    # Metadata lives in <table class="contenttable"> as label/value rows
    # ("Datum: 19. Mai 2026, 15.30 bis 20 Uhr", "Ort: Bürgersaal", ...).
    meta: dict[str, str] = {}
    table = soup.select_one("table.contenttable")
    if table:
        for tr in table.find_all("tr"):
            cells = tr.find_all(["th", "td"])
            if len(cells) >= 2:
                label = cells[0].get_text(" ", strip=True).rstrip(":").lower()
                value = cells[1].get_text(" ", strip=True)
                if label and value:
                    meta[label] = value
    datum = meta.get("datum", "")
    mdate = _parse_german_date(datum) or _parse_german_date(soup.get_text(" ", strip=True))
    tm = re.search(r"(\d{1,2}[.:]\d{2})\s*(?:bis|-|–|—)\s*(\d{1,2}(?:[.:]\d{2})?)\s*Uhr", datum)
    mtime = re.sub(r"\s+", " ", tm.group(0)) if tm else None
    loc = meta.get("ort") or meta.get("raum") or None

    # Meeting-level public: "(öffentlich/nicht öffentlich)" means mixed -> NULL.
    public = _meeting_public(title)

    # Agenda items + their file associations
    agenda: list[dict] = []
    files: dict[str, dict] = {}

    def register_file(a, anchor: Optional[str]) -> None:
        href = a.get("href", "")
        if ".pdf" not in href.lower() or "downloadfiles" not in href:
            return
        # Resolve relative hrefs and refuse anything that isn't an http(s) URL on
        # a karlsruhe.de host — scraped hrefs end up both fetched by us and
        # rendered as links on the public site.
        abs_url = urljoin(url, href)
        parts = urlsplit(abs_url)
        host = (parts.hostname or "").lower()
        if parts.scheme not in ("http", "https") or not (
                host == "karlsruhe.de" or host.endswith(".karlsruhe.de")):
            log.warning("skipping off-site/odd file href %r on %s", href, url)
            return
        fid_m = re.search(r"/(\d+)\.pdf", abs_url, re.IGNORECASE)
        fid = fid_m.group(1) if fid_m else hashlib.sha1(abs_url.encode()).hexdigest()[:16]
        if fid in files and anchor is None:
            return  # agenda pass already placed it under a TOP — don't clobber
        label = re.sub(r"\s+", " ", a.get_text(" ", strip=True)) or os.path.basename(parts.path)
        files[fid] = {
            "id": fid,
            "url": abs_url,
            "label": label,
            "agenda_anchor": anchor,
            "doc_type": classify(label),
            "filename": os.path.basename(parts.path),
        }

    seen_anchors: set[str] = set()
    for det in soup.select("details.top"):
        did = det.get("id", "")
        anchor = did[1:] if did.startswith("d") else did  # dtop3 -> top3
        seen_anchors.add(anchor)
        strong = det.select_one(".summary-title")
        if strong:
            for junk in strong.select("a.anchor, .glyphicon"):
                junk.extract()
        raw = strong.get_text(" ", strip=True) if strong else ""
        raw = re.sub(r"\s*#\s*$", "", raw).strip()
        nm = re.match(r"\s*([\d.]+)\s*[-–]\s*(.*)", raw)
        number = nm.group(1) if nm else None
        atitle = nm.group(2).strip() if nm else raw
        contents = det.select_one(".detail-contents")
        ctext = contents.get_text(" ", strip=True) if contents else ""
        agenda.append({
            "anchor": anchor,
            "number": number,
            "title": atitle,
            "public": _public_flag(ctext),
        })
        scope = contents or det
        for a in scope.select('a[href*="downloadfiles"]'):
            register_file(a, anchor)

    # Files not inside any agenda detail block -> meeting-level (register_file
    # itself skips ids the agenda pass already placed, so anchors survive)
    for a in soup.select('a[href*="downloadfiles"]'):
        register_file(a, None)

    return {
        "id": meeting_id,
        "title": title,
        "body_name": body_name,
        "date": mdate,
        "time": mtime,
        "location": loc,
        "public": public,
        "url": url,
        "agenda": agenda,
        "files": list(files.values()),
    }


# ---------------------------------------------------------------------------
# Scrape orchestration
# ---------------------------------------------------------------------------

def sync_committees_members(f: Fetcher) -> tuple[dict[str, str], int]:
    """Crawl /gremien + each committee org page: upsert bodies, replace member
    rosters. Returns (committee id->name map, members parsed). Used by both the
    HTML scraper and the OParl ingester (OParl exposes no party affiliation, so
    the roster's party column can only come from the HTML pages)."""
    committees: dict[str, str] = {}
    r = f.get(f"{RIS}/gremien")
    if r:
        committees = parse_committees(r.text)
        with db.write_conn() as c:
            for cid, name in committees.items():
                c.execute(
                    """INSERT INTO bodies(id,name) VALUES(?,?)
                       ON CONFLICT(id) DO UPDATE SET name=excluded.name, last_seen=datetime('now')""",
                    (cid, name))
    log.info("committees: %d", len(committees))

    mcount = 0
    for cid in committees:
        r = f.get(f"{RIS}/{cid}")
        if not r:
            continue
        mems = parse_members(r.text)
        if not mems:
            # A 200 page without a parseable roster (markup change, error page)
            # must not wipe the stored membership.
            log.warning("no members parsed for %s — keeping existing roster", cid)
            continue
        with db.write_conn() as c:
            c.execute("DELETE FROM members WHERE body_id=?", (cid,))
            for mm in mems:
                c.execute(
                    """INSERT OR REPLACE INTO members
                       (body_id,person_id,name,party,party_code,role,since)
                       VALUES(?,?,?,?,?,?,?)""",
                    (cid, mm["person_id"], mm["name"], mm["party"],
                     mm["party_code"], mm["role"], mm["since"]))
        mcount += len(mems)
    log.info("members parsed: %d", mcount)
    return committees, mcount


def _window_months(months: int) -> list[tuple[int, int]]:
    today = date.today()
    out = []
    y, m = today.year, today.month
    for _ in range(months + 1):
        out.append((y, m))
        m -= 1
        if m == 0:
            y, m = y - 1, 12
    return out


def _forward_months(months: int) -> list[tuple[int, int]]:
    today = date.today()
    out = []
    y, m = today.year, today.month
    for _ in range(months + 1):
        out.append((y, m))
        m += 1
        if m == 13:
            y, m = y + 1, 1
    return out


def _all_months_since(start_year: int = 2014) -> list[tuple[int, int]]:
    today = date.today()
    out = []
    y, m = start_year, 1
    while (y, m) <= (today.year, today.month):
        out.append((y, m))
        m += 1
        if m == 13:
            y, m = y + 1, 1
    return out


def scrape(months: int, backfill_all: bool = False, enable_llm: bool = ENABLE_LLM) -> dict:
    os.makedirs(PDF_DIR, exist_ok=True)
    db.init_db()
    f = Fetcher()
    counts = {"meetings": 0, "files_new": 0, "files_seen": 0, "text_ok": 0, "enriched": 0, "errors": 0}
    try:
        # 1. committees + membership rosters (shared with the OParl ingester —
        # OParl carries no party affiliation, so rosters stay HTML either way)
        committees, mcount = sync_committees_members(f)
        name_to_id = {v: k for k, v in committees.items()}
        counts["members"] = mcount

        # 2. discover termin ids across the window + a few months ahead (upcoming)
        month_list = _all_months_since() if backfill_all else _window_months(months)
        month_list = sorted(set(month_list) | set(_forward_months(FORWARD_MONTHS)))
        termin_ids: set[str] = set()
        for (y, m) in month_list:
            r = f.get(f"{RIS}/termine?j={y}&m={m}&g=")
            if not r:
                continue
            ids = parse_calendar_termine(r.text)
            termin_ids.update(ids)
            log.info("%04d-%02d: %d meetings", y, m, len(ids))
        # Always revisit recently-held meetings (catch late-added voting results
        # / minutes) even if they fall outside the month window.
        recheck_cutoff = (date.today() - timedelta(days=RECHECK_DAYS)).isoformat()
        recent_known = db.recent_meeting_ids(recheck_cutoff)
        new_recent = set(recent_known) - termin_ids
        termin_ids.update(recent_known)
        log.info("discovered %d meetings (+%d recent re-checks since %s)",
                 len(termin_ids), len(new_recent), recheck_cutoff)

        # 3. each meeting (guarded — one bad meeting/file mustn't abort the run)
        for tid in sorted(termin_ids):
            try:
                url = f"{RIS}/{tid}"
                r = f.get(url)
                if not r:
                    counts["errors"] += 1
                    continue
                meeting = parse_meeting(r.text, url, tid)
                recheck = bool(meeting["date"]) and meeting["date"] >= recheck_cutoff
                body_id = name_to_id.get(meeting["body_name"])
                counts["meetings"] += 1
                _upsert_meeting(meeting, body_id)

                for fmeta in meeting["files"]:
                    try:
                        _process_file(f, fmeta, tid, counts, recheck=recheck)
                    except Exception as e:  # noqa: BLE001
                        log.warning("file %s (%s) failed: %s", fmeta.get("id"), tid, e)
                        counts["errors"] += 1
            except Exception as e:  # noqa: BLE001
                log.warning("meeting %s failed: %s", tid, e)
                counts["errors"] += 1
        # 4. enrichment of anything pending (this run + leftovers)
        if enable_llm:
            counts["enriched"] += _enrich_pending(counts)
        # 5. parse vote tallies from image-only Abstimmungsergebnis PDFs
        counts["votes"] = parse_pending_votes()

        with db.write_conn() as c:
            db.set_meta(c, "last_scrape", datetime.now().isoformat(timespec="seconds"))
            db.set_meta(c, "last_scrape_status", f"ok {counts}")
        log.info("scrape done: %s", counts)
        return counts
    except Exception as e:
        # Record the failure so the dashboard doesn't keep showing a stale "ok".
        try:
            with db.write_conn() as c:
                db.set_meta(c, "last_scrape_status", f"error {e}")
        except Exception:  # noqa: BLE001
            pass
        raise
    finally:
        f.close()


def _upsert_meeting(meeting: dict, body_id: Optional[str]) -> None:
    """Write the meeting row + its agenda items. Shared by scrape() and scrape_one()."""
    tid = meeting["id"]
    with db.write_conn() as c:
        c.execute(
            """INSERT INTO meetings(id,body_id,body_name,title,date,time,location,public,url,scraped_at)
               VALUES(?,?,?,?,?,?,?,?,?,datetime('now'))
               ON CONFLICT(id) DO UPDATE SET body_id=excluded.body_id,
                 body_name=excluded.body_name, title=excluded.title, date=excluded.date,
                 time=excluded.time, location=excluded.location, public=excluded.public,
                 scraped_at=datetime('now')""",
            (tid, body_id, meeting["body_name"], meeting["title"], meeting["date"],
             meeting["time"], meeting["location"], meeting["public"], meeting["url"]))
        for ai in meeting["agenda"]:
            c.execute(
                """INSERT INTO agenda_items(meeting_id,anchor,number,title,public)
                   VALUES(?,?,?,?,?)
                   ON CONFLICT(meeting_id,anchor) DO UPDATE SET number=excluded.number,
                     title=excluded.title, public=excluded.public""",
                (tid, ai["anchor"], ai["number"], ai["title"], ai["public"]))
        # Drop TOPs the RIS removed/renumbered — but only when we actually parsed
        # an agenda, so a markup change can't wipe a meeting's items.
        if meeting["agenda"]:
            anchors = [ai["anchor"] for ai in meeting["agenda"]]
            ph = ",".join("?" * len(anchors))
            c.execute(
                f"DELETE FROM agenda_items WHERE meeting_id=? AND anchor NOT IN ({ph})",
                (tid, *anchors))


def _resolve_body_id(body_name: str) -> Optional[str]:
    if not body_name:
        return None
    row = db.get_conn().execute("SELECT id FROM bodies WHERE name=?", (body_name,)).fetchone()
    return row["id"] if row else None


def scrape_one(termin_id: str, enable_llm: bool = False, recheck: bool = False) -> dict:
    """On-demand refresh of ONE meeting: re-parse its page, upsert meeting+agenda, and
    process its files. Defaults to recheck=False so it grabs only NEW files (e.g. live
    Protokoll/Abstimmungsergebnis) quickly without a per-file HEAD storm; LLM enrichment
    is deferred to the daily run. Returns {ok, counts, error?}."""
    if not re.fullmatch(r"termin-\d+", termin_id or ""):
        raise ValueError("invalid termin id")
    os.makedirs(PDF_DIR, exist_ok=True)
    db.init_db()
    counts = {"files_new": 0, "files_updated": 0, "files_seen": 0,
              "text_ok": 0, "enriched": 0, "errors": 0}
    f = Fetcher()
    try:
        url = f"{RIS}/{termin_id}"
        r = f.get(url)
        if not r:
            counts["errors"] += 1
            return {"ok": False, "error": "Sitzungsseite nicht erreichbar", "counts": counts}
        meeting = parse_meeting(r.text, url, termin_id)
        _upsert_meeting(meeting, _resolve_body_id(meeting["body_name"]))
        for fmeta in meeting["files"]:
            try:
                _process_file(f, fmeta, termin_id, counts, recheck=recheck)
            except Exception as e:  # noqa: BLE001 — one bad file mustn't abort the rest
                log.warning("refresh: file %s failed: %s", fmeta.get("id"), e)
                counts["errors"] += 1
        if enable_llm:
            counts["enriched"] += _enrich_pending(counts)
        return {"ok": True, "counts": counts}
    finally:
        f.close()


# The download → sha-dedupe → pdftotext → version-archive → vorlage pipeline is
# shared with oparl.py; it lives in filestore.process_file.
_process_file = filestore.process_file


def _vote_anchor(row) -> Optional[str]:
    """Agenda anchor for a vote PDF: prefer the file's own agenda_anchor, else derive
    'topN'/'topN.M' from a 'TOP n' in the label (dotted sub-items keep their dot so
    they key/reconcile the same way as the scraped anchors). None if neither is
    available (don't guess)."""
    if row["agenda_anchor"]:
        return row["agenda_anchor"]
    m = re.search(r"TOP\s*(\d+(?:\.\d+)*)", row["label"] or "")
    return f"top{m.group(1)}" if m else None


def parse_vote_pdf(file_id: str) -> bool:
    """Render an image-only 'Abstimmungsergebnis' PDF's page 1 and parse its vote tally."""
    import tempfile
    import vote_parse
    conn = db.get_conn()
    row = conn.execute(
        "SELECT id, meeting_id, agenda_anchor, label, url, local_path FROM files WHERE id=?",
        (file_id,)).fetchone()
    if not row or not row["local_path"] or not os.path.exists(row["local_path"]):
        return False
    anchor = _vote_anchor(row)
    if not anchor:
        log.info("vote pdf %s: no agenda anchor (label %r) — skipping", file_id, row["label"])
        return False
    m = re.search(r"TOP\s*(\d+(?:\.\d+)*)", row["label"] or "")
    top_label = f"TOP {m.group(1)}" if m else f"TOP {anchor[3:]}"
    with tempfile.TemporaryDirectory() as td:
        base = os.path.join(td, "v")
        try:
            subprocess.run(["pdftoppm", "-jpeg", "-r", "150", "-f", "1", "-l", "1",
                            row["local_path"], base], capture_output=True, timeout=120, check=True)
        except Exception as e:  # noqa: BLE001
            log.warning("pdftoppm failed for %s: %s", file_id, e)
            return False
        imgs = [os.path.join(td, x) for x in os.listdir(td) if x.endswith(".jpg")]
        if not imgs:
            return False
        data = open(imgs[0], "rb").read()
    parsed = vote_parse.parse_vote_image(data)
    if not parsed:
        return False
    with db.write_conn() as c:
        db.upsert_vote(c, {
            "meeting_id": row["meeting_id"], "agenda_anchor": anchor, "top_label": top_label,
            "result_text": None, "ja": parsed.get("ja"), "nein": parsed.get("nein"),
            "enthaltung": parsed.get("enthaltung"), "members": parsed.get("members"),
            "source": "pdf", "image_url": row["url"], "image_sha": hashlib.sha256(data).hexdigest(),
            "file_id": file_id,
        })
    return True


def parse_pending_votes(limit: int = 1000) -> int:
    """Parse Abstimmungsergebnis PDFs that don't yet have a stored vote tally."""
    import vote_parse
    if not vote_parse.enabled():
        return 0
    conn = db.get_conn()
    rows = conn.execute(
        """SELECT id, meeting_id, label, agenda_anchor FROM files
           WHERE doc_type='Abstimmungsergebnis' AND meeting_id IS NOT NULL
           ORDER BY downloaded_at DESC LIMIT ?""", (limit,)).fetchall()
    max_attempts = int(os.environ.get("VOTE_PARSE_MAX_ATTEMPTS", "3"))
    done = 0
    for r in rows:
        anchor = _vote_anchor(r)
        if not anchor:
            continue  # can't place it under an agenda item; skip (don't re-bill the model)
        ex = conn.execute("SELECT source FROM votes WHERE id=?",
                          (f"{r['meeting_id']}:{anchor}",)).fetchone()
        if ex and ex["source"] == "pdf":
            continue  # already have the authoritative PDF tally for this TOP
        if db.vote_parse_attempts(r["id"]) >= max_attempts:
            continue  # unreadable scan — stop re-billing the vision model every run
        try:
            ok = parse_vote_pdf(r["id"])  # parses + supersedes any earlier 'live' row
            db.record_vote_parse_attempt(r["id"], ok)
            if ok:
                done += 1
        except Exception as e:  # noqa: BLE001
            log.warning("vote pdf %s failed: %s", r["id"], e)
            db.record_vote_parse_attempt(r["id"], False)
    if done:
        log.info("parsed %d vote PDFs", done)
    return done


def backfill_vorlagen() -> int:
    """Recompute Vorlagennummer ownership/references for every file (no network)."""
    db.init_db()
    conn = db.get_conn()
    rows = conn.execute("SELECT id, label, doc_type, fulltext FROM files").fetchall()
    n = 0
    with db.write_conn() as c:
        for r in rows:
            own, refs = extract_vorlagen(r["label"], r["fulltext"], r["doc_type"])
            db.set_file_vorlagen(c, r["id"], own, refs)
            if own or refs:
                n += 1
    log.info("vorlagen backfilled: %d files carry numbers", n)
    return n


def backfill_submitters() -> int:
    """Recompute submitters for every existing file from label+text+doc_type (no network)."""
    db.init_db()
    conn = db.get_conn()
    rows = conn.execute("SELECT id, label, doc_type, fulltext FROM files").fetchall()
    n = 0
    with db.write_conn() as c:
        for r in rows:
            subs = submitter.derive(r["label"], r["fulltext"], r["doc_type"])
            c.execute("UPDATE files SET submitters=? WHERE id=?",
                      (json.dumps(subs, ensure_ascii=False), r["id"]))
            n += 1
    log.info("backfilled submitters for %d files", n)
    return n


def _enrich_pending(counts: dict) -> int:
    import enrich
    from concurrent.futures import ThreadPoolExecutor, as_completed
    conn = db.get_conn()
    rows = conn.execute(
        """SELECT id FROM files WHERE enrich_status='pending' AND text_status='ok'
           ORDER BY downloaded_at DESC""").fetchall()
    ids = [r["id"] for r in rows]
    if not ids:
        return 0
    conc = max(1, int(os.environ.get("ENRICH_CONCURRENCY", "4")))
    log.info("enriching %d documents (concurrency=%d)", len(ids), conc)

    def _one(fid: str) -> bool:
        try:
            return enrich.enrich_file(fid)
        except Exception as e:  # noqa: BLE001
            log.warning("enrich failed for %s: %s", fid, e)
            return False

    done = 0
    with ThreadPoolExecutor(max_workers=conc) as pool:
        futures = {pool.submit(_one, fid): fid for fid in ids}
        for i, fut in enumerate(as_completed(futures), 1):
            if fut.result():
                done += 1
            if i % 25 == 0:
                log.info("enrichment progress: %d/%d", i, len(ids))
    return done


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout,
    )
    ap = argparse.ArgumentParser(description="Karlsruhe Sitzungskalender scraper")
    ap.add_argument("--months", type=int, default=BACKFILL_MONTHS, help="how many months back")
    ap.add_argument("--backfill-all", action="store_true", help="crawl the entire archive")
    ap.add_argument("--no-llm", action="store_true", help="skip LLM enrichment this run")
    ap.add_argument("--submitters", action="store_true",
                    help="recompute submitter codes for all existing files and exit")
    ap.add_argument("--votes", action="store_true",
                    help="parse vote tallies from Abstimmungsergebnis PDFs and exit")
    ap.add_argument("--vorlagen", action="store_true",
                    help="recompute Vorlagennummer index for all existing files and exit")
    args = ap.parse_args()
    if args.submitters:
        print({"backfilled": backfill_submitters()})
        return 0
    if args.vorlagen:
        print({"vorlagen_files": backfill_vorlagen()})
        return 0
    if args.votes:
        db.init_db()
        print({"parsed_votes": parse_pending_votes()})
        return 0
    counts = scrape(args.months, backfill_all=args.backfill_all, enable_llm=ENABLE_LLM and not args.no_llm)
    print(counts)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
