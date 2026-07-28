"""
Desinformationssystem — OParl API ingester.

Alternative to the HTML scraper (scraper.py): ingests from the Karlsruhe OParl
1.1 endpoint (SOMACOS Session) at https://web1.karlsruhe.de/ris/oparl and writes
the SAME database rows, so the enrichment / vote / search / web layers are
untouched. See docs/oparl-migration.md for the verified API behaviour.

Key quirks handled here:
  - The API lives under /ris/oparl/… but every URL it emits (its own ids,
    links.next, file URLs) uses the broken /oparl/… prefix → every followed URL
    is rewritten via fix().
  - elementsPerPage is fixed at 10; pagination via links.next.
  - ?modified_since=<ISO8601> works and drives incremental sync (watermark in
    meta key 'oparl_since').

Identity crosswalk (verified live 2026-07-24, recorded in the migration doc):
  OParl meeting /meetings/N        == HTML slug  termin-N
  OParl organization /…/gr/N       == HTML slug  organisation-gr-N
  OParl person /people/N           == HTML slug  people-N
  OParl file fileName "0067…7.pdf" == our files.id "0067…7" (8-digit stem);
    the rewritten downloadUrl is byte-identical to the URL the HTML pages carry.
  agendaItem.number "3.1"          == our anchor "top3.1"

Deliberately still HTML-based (hybrid): committee list + member rosters
(scraper.sync_committees_members) because OParl exposes NO party affiliation
anywhere (persons carry no party, memberships list only committees), and the
party column feeds vote roll-call attribution.

Votes are untouched: ticker.py + parse_pending_votes keep working because the
meeting ids and agenda anchors written here are identical to the scraped ones.
"""
from __future__ import annotations

import argparse
import logging
import os
import re
import sys
from datetime import date, datetime, timedelta, timezone
from typing import Iterator, Optional
from urllib.parse import parse_qsl, quote, urlencode, urlsplit, urlunsplit
from zoneinfo import ZoneInfo

import db
import filestore
import scraper

log = logging.getLogger("oparl")

BASE = os.environ.get("OPARL_BASE", "https://web1.karlsruhe.de/ris/oparl")
BODY = os.environ.get("OPARL_BODY", "0001")
# Self-links in API responses use the broken prefix; rewrite before fetching.
_BROKEN = "https://web1.karlsruhe.de/oparl/"
_FIXED = "https://web1.karlsruhe.de/ris/oparl/"

# Page size. The server honours ?limit= (NOT ?elementsPerPage=, which it
# silently ignores — that mistake made every walk 100x longer than necessary
# and produced the wrong "elementsPerPage is fixed at 10" note in the migration
# doc). Verified live: limit=100 -> 100 items, limit=1000 -> 1000, and
# links.next carries the limit forward. Matches karlsruhe-oparl-syndication.
PAGE_SIZE = int(os.environ.get("OPARL_PAGE_SIZE", "1000"))
BACKFILL_MONTHS = int(os.environ.get("BACKFILL_MONTHS", "12"))
RECHECK_DAYS = int(os.environ.get("RECHECK_DAYS", "21"))
# Overlap the incremental window rather than resuming exactly at the last run's
# start: guards against clock skew between us and the vendor, and against an
# object whose `modified` lands during the run. Re-walking a day is cheap now
# that a page holds 1000 objects.
WATERMARK_OVERLAP_HOURS = int(os.environ.get("OPARL_OVERLAP_HOURS", "24"))
ENABLE_LLM = os.environ.get("ENABLE_LLM", "1") == "1"
_BERLIN = ZoneInfo("Europe/Berlin")

# paperType (the source's own classification) -> our doc_type vocabulary.
# Used as a fallback when classify(label) yields 'Sonstiges'; the raw value is
# always kept in files.paper_type. Enumerated live 2026-07-24 over the 4000
# most recently modified papers (modified 2025-03 … 2026-07):
#   Beschlussvorlage 1614 · Antrag 634 · Informationsvorlage 540 · Anfrage 472
#   · Änderungs-/Ergänzungsantrag 208 · "Haushalt THH nnnn"/"Haushalt Gesamt"
#   ~520 (budget motions, ref "DHH/YYYY/NNNN") · Offenlage 9 · (none) 3.
PAPER_TYPE_MAP = {
    "Beschlussvorlage": "Beschlussvorlage",
    "Informationsvorlage": "Vorlage",
    "Anfrage": "Anfrage",
    "Antrag": "Antrag",
    "Änderungs-/Ergänzungsantrag": "Antrag",
    "Offenlage": "Vorlage",   # sampled: ordinary Vorlagen under public display
}


def map_paper_type(ptype: Optional[str]) -> Optional[str]:
    if not ptype:
        return None
    if ptype in PAPER_TYPE_MAP:
        return PAPER_TYPE_MAP[ptype]
    if ptype.startswith("Haushalt"):   # "Haushalt THH 4100" etc. = budget motions
        return "Antrag"
    return None


class OparlError(RuntimeError):
    pass


def _iso_key(ts: str) -> datetime:
    """Parse an OParl ISO timestamp for ordering against the watermark. Offsets
    make lexicographic comparison wrong across a DST change, so compare as
    datetimes. Unparseable values sort oldest, which holds the watermark back —
    the safe direction (re-walk too much, never skip)."""
    try:
        d = datetime.fromisoformat((ts or "").replace("Z", "+00:00"))
    except ValueError:
        return datetime.min.replace(tzinfo=timezone.utc)
    return d if d.tzinfo else d.replace(tzinfo=_BERLIN)


def fix(url: Optional[str]) -> Optional[str]:
    return url.replace(_BROKEN, _FIXED) if url else url


def _requote_since(url: str, modified_since: Optional[str]) -> str:
    """Re-append modified_since, correctly percent-encoded.

    The API's own links.next emits the filter with a RAW '+' offset
    ("...modified_since=2026-07-25T16:46:24+02:00"). In a query string '+'
    decodes to a space, so the server can't parse the timestamp and silently
    drops the filter — verified live: page 40 followed that way is identical to
    page 40 with no filter at all. Following links.next verbatim therefore turns
    every incremental run into a full-archive walk from page 2 onward. Rebuild
    the param from our own value instead of trusting the returned query."""
    if not modified_since:
        return url
    parts = urlsplit(url)
    # parse_qsl has already turned the raw '+' into a space, so the server's
    # value is unrecoverable here — drop it and re-encode ours.
    kept = [(k, v) for k, v in parse_qsl(parts.query, keep_blank_values=True)
            if k != "modified_since"]
    query = urlencode(kept, quote_via=quote)
    since = f"modified_since={quote(modified_since, safe='')}"
    query = f"{query}&{since}" if query else since
    return urlunsplit((parts.scheme, parts.netloc, parts.path, query, parts.fragment))


def _tail(url: str) -> str:
    return url.rstrip("/").rsplit("/", 1)[1]


def _termin_num(tid: str) -> int:
    """Numeric sort key for a 'termin-N' id. String ordering is wrong once the
    archive crosses a digit-count boundary ("termin-9999" > "termin-10007")."""
    try:
        return int(tid.rsplit("-", 1)[1])
    except (IndexError, ValueError):
        return -1


_ORG_RE = re.compile(r"/organizations/([a-z]+)/(\d+)")


def _body_id_from_org(url: Optional[str]) -> Optional[str]:
    """OParl organizations/gr/N == HTML organisation-gr-N (verified). Non-gr
    organizations (at/… = Ämter) are not Gremien and yield None."""
    m = _ORG_RE.search(url or "")
    if m and m.group(1) == "gr":
        return f"organisation-gr-{m.group(2)}"
    return None


class Client:
    """Fetcher wrapper: prefix rewrite + JSON + pagination walking."""

    def __init__(self) -> None:
        self.f = filestore.Fetcher()

    def get_json(self, url: str) -> Optional[dict]:
        r = self.f.get(fix(url))
        if r is None:
            return None
        try:
            return r.json()
        except ValueError:
            log.warning("non-JSON response for %s", url)
            return None

    def iter_list(self, url: str, modified_since: Optional[str] = None,
                  max_pages: int = 100000) -> Iterator[dict]:
        """Yield objects across all pages, following links.next (rewritten).
        Raises OparlError if a page fails — a silent break could advance the
        watermark past unseen objects."""
        sep = "&" if "?" in url else "?"
        url = f"{url}{sep}limit={PAGE_SIZE}"
        if modified_since:
            url = f"{url}&modified_since={quote(modified_since, safe='')}"
        pages = 0
        # A self-referential or cyclic links.next would otherwise spin against
        # the API until max_pages; a repeat means the collection is broken, and
        # continuing would silently yield duplicates.
        seen: set[str] = set()
        while url and pages < max_pages:
            if url in seen:
                raise OparlError(f"pagination cycle at {url} after {pages} page(s)")
            seen.add(url)
            d = self.get_json(url)
            if d is None:
                raise OparlError(f"list page failed: {url}")
            if not isinstance(d.get("data"), list):
                # A 200 carrying a non-collection body (error page, maintenance
                # notice) is still an incomplete crawl — never checkpoint it.
                raise OparlError(f"no data array at {url}")
            pages += 1
            yield from d["data"]
            nxt = (d.get("links") or {}).get("next")
            url = _requote_since(fix(nxt), modified_since) if nxt else None

    def close(self) -> None:
        self.f.close()


def _fmt_hour(dt: datetime) -> str:
    return f"{dt.hour}.{dt.minute:02d}" if dt.minute else str(dt.hour)


def _german_time(start: Optional[str], end: Optional[str]) -> Optional[str]:
    """Rebuild the RIS-style '15.30 bis 20 Uhr' string the rest of the stack
    (db.parse_time_window, the live banner, the UI) expects. None when the API
    carries no real time-of-day (00:00:00 on both ends)."""
    try:
        s = datetime.fromisoformat(start) if start else None
        e = datetime.fromisoformat(end) if end else None
    except ValueError:
        return None
    if not s:
        return None
    if e and e > s and not (s.hour == s.minute == 0):
        return f"{_fmt_hour(s)} bis {_fmt_hour(e)} Uhr"
    return None


def _location_str(loc) -> Optional[str]:
    if not isinstance(loc, dict) or loc.get("deleted"):
        return None
    parts = [loc.get("room"), loc.get("description"), loc.get("streetAddress")]
    out = next((p.strip() for p in parts if p and p.strip()), None)
    return out


_PDF_NAME_RE = re.compile(r"^(\d+)\.pdf$", re.IGNORECASE)


def _file_meta(fobj, anchor: Optional[str]) -> Optional[dict]:
    """OParl File object -> the fmeta dict filestore.process_file expects.
    File identity: the zero-padded fileName stem ('00677201'), which is exactly
    the id the HTML scraper derives from download URLs — same rows, no dupes."""
    if not isinstance(fobj, dict) or fobj.get("deleted"):
        return None
    filename = fobj.get("fileName") or ""
    m = _PDF_NAME_RE.match(filename)
    url = fix(fobj.get("downloadUrl") or fobj.get("accessUrl"))
    if not m or not url:
        return None  # non-PDF or unnamed — the HTML path only ever ingested PDFs
    fid = m.group(1)
    label = re.sub(r"\s+", " ", (fobj.get("name") or "").strip()) or filename
    return {
        "id": fid,
        "url": url,
        "label": label,
        "agenda_anchor": anchor,
        "doc_type": filestore.classify(label),
        "filename": filename,
    }


def _ref_base(reference: Optional[str]) -> Optional[str]:
    """'2026/0365/3' -> '2026/0365' (the form file_vorlagen keys on)."""
    m = re.match(r"\s*(20\d{2}/\d{3,5})", reference or "")
    return m.group(1) if m else None


class Ingest:
    def __init__(self, cli: Client, recheck_cutoff: str, counts: dict,
                 failed: Optional[list] = None) -> None:
        self.cli = cli
        self.recheck_cutoff = recheck_cutoff
        self.counts = counts
        # `modified` stamps of objects whose ingest partly failed. sync() holds
        # the watermark back to the oldest of these so the next run re-walks
        # them; without it a swallowed error drops the object permanently.
        self.failed: list = failed if failed is not None else []
        # Indirection so the bulk seeder (seed.py) can substitute a local,
        # network-free file writer and still reuse every mapping rule below.
        self.process_file = filestore.process_file
        # OParl agendaitem numeric id -> (termin_id, anchor); filled while
        # ingesting meetings, extended on demand for consultations.
        self.aimap: dict[str, tuple[str, Optional[str]]] = {}
        self.meeting_dates: dict[str, Optional[str]] = {}
        conn = db.get_conn()
        self.known_meetings: set[str] = {
            r["id"] for r in conn.execute("SELECT id FROM meetings")}
        for r in conn.execute("SELECT id, date FROM meetings"):
            self.meeting_dates[r["id"]] = r["date"]

    # -- meetings -----------------------------------------------------------

    def ingest_meeting(self, m: dict, with_files: bool = True) -> Optional[str]:
        if m.get("deleted"):
            # The RIS deletes cancelled meetings. We keep any existing row (the
            # HTML scraper never saw deletions either) but don't create one.
            self.counts["deleted_meetings"] = self.counts.get("deleted_meetings", 0) + 1
            return None
        num = _tail(m["id"])
        tid = f"termin-{num}"
        title = (m.get("name") or "").strip()
        mdate = (m.get("start") or "")[:10] or None
        if not title and not mdate:
            # observed only on deleted objects, but never write junk rows
            self.counts["empty_meetings"] = self.counts.get("empty_meetings", 0) + 1
            return None
        body_name = re.sub(r"\s*\(.*?\)\s*$", "", title).strip()
        # Scan the whole array: every meeting seen so far carries exactly one
        # organization, but a non-gr entry (an Amt) listed first would otherwise
        # null out body_id and orphan the meeting from its committee.
        body_id = next(
            (b for b in (_body_id_from_org(o) for o in (m.get("organization") or []))
             if b), None)

        agenda = []
        for ai in m.get("agendaItem") or []:
            number = (ai.get("number") or "").strip()
            if not number:
                self.counts["agenda_no_number"] = self.counts.get("agenda_no_number", 0) + 1
                continue
            anchor = f"top{number}"
            agenda.append({
                "anchor": anchor,
                "number": number,
                "title": (ai.get("name") or "").strip(),
                "public": (1 if ai["public"] else 0) if ai.get("public") is not None else None,
            })
            self.aimap[_tail(ai["id"])] = (tid, anchor)

        meeting = {
            "id": tid,
            "title": title,
            "body_name": body_name,
            "date": mdate,
            "time": _german_time(m.get("start"), m.get("end")),
            "location": _location_str(m.get("location")),
            "public": scraper._meeting_public(title),
            "url": f"{scraper.RIS}/{tid}",
            "agenda": agenda,
        }
        if body_id:
            with db.write_conn() as c:
                # Placeholder name from the meeting title; the HTML /gremien
                # sync (which runs first) provides the canonical name.
                c.execute(
                    """INSERT INTO bodies(id,name) VALUES(?,?)
                       ON CONFLICT(id) DO UPDATE SET last_seen=datetime('now')""",
                    (body_id, body_name or body_id))
        scraper._upsert_meeting(meeting, body_id)
        self.known_meetings.add(tid)
        self.meeting_dates[tid] = mdate
        self.counts["meetings"] += 1

        if with_files:
            recheck = bool(mdate) and mdate >= self.recheck_cutoff
            for fmeta in self._meeting_files(m):
                try:
                    self.process_file(self.cli.f, fmeta, tid, self.counts,
                                           recheck=recheck)
                except Exception as e:  # noqa: BLE001
                    log.warning("file %s (%s) failed: %s", fmeta.get("id"), tid, e)
                    self.counts["errors"] += 1
        return tid

    def _meeting_files(self, m: dict) -> list[dict]:
        """invitation/protocols/auxiliaryFile at meeting level (anchor None) +
        each agendaItem's resolutionFile/auxiliaryFile under its TOP. Anchored
        entries win over meeting-level duplicates, like the HTML two-pass."""
        out: dict[str, dict] = {}

        def add(fobj, anchor):
            meta = _file_meta(fobj, anchor)
            if not meta:
                return
            if meta["id"] in out and anchor is None:
                return  # already placed under a TOP
            out[meta["id"]] = meta

        for ai in m.get("agendaItem") or []:
            number = (ai.get("number") or "").strip()
            anchor = f"top{number}" if number else None
            add(ai.get("resolutionFile"), anchor)
            for fo in ai.get("auxiliaryFile") or []:
                add(fo, anchor)
        add(m.get("invitation"), None)
        add(m.get("resultsProtocol"), None)
        add(m.get("verbatimProtocol"), None)
        for fo in m.get("auxiliaryFile") or []:
            add(fo, None)
        return list(out.values())

    def ensure_meeting(self, num: str) -> Optional[str]:
        """Meeting row (+agenda) for termin-<num>, fetching from the API when we
        haven't seen it — consultations may point at meetings outside the
        incremental window. Files are NOT fetched here (bounded work; they come
        via the normal meeting walk when the meeting changes)."""
        tid = f"termin-{num}"
        if tid in self.known_meetings:
            return tid
        m = self.cli.get_json(f"{BASE}/bodies/{BODY}/meetings/{num}")
        if not m or m.get("deleted"):
            return None
        return self.ingest_meeting(m, with_files=False)

    # -- papers -------------------------------------------------------------

    def _resolve_anchor(self, ai_num: str) -> tuple[Optional[str], Optional[str]]:
        """(termin_id, anchor) for an OParl agendaitem id, fetching on demand."""
        if ai_num in self.aimap:
            return self.aimap[ai_num]
        ai = self.cli.get_json(f"{BASE}/bodies/{BODY}/agendaitems/{ai_num}")
        if not ai or ai.get("deleted"):
            self.aimap[ai_num] = (None, None)
            return (None, None)
        number = (ai.get("number") or "").strip()
        murl = ai.get("meeting") or ""
        tid = f"termin-{_tail(murl)}" if murl else None
        anchor = f"top{number}" if number else None
        self.aimap[ai_num] = (tid, anchor)
        return (tid, anchor)

    def ingest_paper(self, p: dict) -> None:
        if p.get("deleted"):
            self.counts["deleted_papers"] = self.counts.get("deleted_papers", 0) + 1
            return
        ptype = p.get("paperType")
        ref_full = (p.get("reference") or "").strip() or None
        base = _ref_base(ref_full)
        self.counts["papers"] += 1

        # Resolve consultations -> (termin_id, anchor, meeting_date) stations.
        stations: list[tuple[str, Optional[str], Optional[str]]] = []
        for cons in p.get("consultation") or []:
            murl = cons.get("meeting")
            if not murl:
                continue
            tid = self.ensure_meeting(_tail(murl))
            if not tid:
                continue
            anchor = None
            if cons.get("agendaItem"):
                ai_tid, anchor = self._resolve_anchor(_tail(cons["agendaItem"]))
                if ai_tid and ai_tid != tid:
                    anchor = None  # inconsistent linkage — don't guess
            stations.append((tid, anchor, self.meeting_dates.get(tid)))

        files = []
        if p.get("mainFile"):
            files.append(p["mainFile"])
        files.extend(p.get("auxiliaryFile") or [])

        fids: list[str] = []
        if stations:
            # Attach the paper's PDFs to its latest station (the HTML scraper's
            # last-meeting-wins behaviour, made deterministic). Sort the termin id
            # numerically — lexicographically "termin-9999" > "termin-10007".
            tgt = max(stations, key=lambda s: (s[2] or "", _termin_num(s[0])))
            recheck = bool(tgt[2]) and tgt[2] >= self.recheck_cutoff
            conn = db.get_conn()
            for fobj in files:
                meta = _file_meta(fobj, tgt[1])
                if not meta:
                    continue
                mapped = map_paper_type(ptype)
                if meta["doc_type"] == "Sonstiges" and mapped:
                    meta["doc_type"] = mapped
                try:
                    self.process_file(self.cli.f, meta, tgt[0], self.counts,
                                           recheck=recheck)
                except Exception as e:  # noqa: BLE001
                    log.warning("paper file %s failed: %s", meta.get("id"), e)
                    self.counts["errors"] += 1
                    self.failed.append(p.get("modified") or "")
                    continue
                # process_file returns False both for "download failed" and for
                # "already present, unchanged", so its return value can't gate
                # this — check the row instead, or we stamp paper metadata and
                # file_vorlagen rows onto a file_id that doesn't exist.
                if conn.execute("SELECT 1 FROM files WHERE id=?",
                                (meta["id"],)).fetchone():
                    fids.append(meta["id"])
                else:
                    # process_file already counted the error; the PDF never
                    # landed, so this paper must come round again next run.
                    self.counts["files_missing"] = self.counts.get("files_missing", 0) + 1
                    self.failed.append(p.get("modified") or "")
        elif files:
            # No consultation on a known meeting -> nowhere to hang NEW files in
            # our meeting-rooted schema. But if a file already exists (e.g. the
            # meeting walk ingested it as an agenda attachment), still stamp the
            # paper metadata on it below.
            conn = db.get_conn()
            for fobj in files:
                meta = _file_meta(fobj, None)
                if meta and conn.execute("SELECT 1 FROM files WHERE id=?",
                                         (meta["id"],)).fetchone():
                    fids.append(meta["id"])
            if not fids:
                self.counts["papers_unattached"] = self.counts.get("papers_unattached", 0) + 1

        # Paper metadata + authoritative Vorlagen lifecycle (consultations beat
        # fulltext mention-mining, which process_file already ran).
        with db.write_conn() as c:
            for fid in fids:
                c.execute("UPDATE files SET paper_type=?, paper_reference=? WHERE id=?",
                          (ptype, ref_full, fid))
                if base:
                    # The OParl reference is authoritative, so demote any other
                    # own=1 row that fulltext mining (process_file ->
                    # db.set_file_vorlagen) set under a *different* number. The
                    # (file_id,vorlage) PK means INSERT OR REPLACE can't clear
                    # it, and two own=1 rows put this file at the head of two
                    # unrelated Vorlage chains. Demote rather than delete: the
                    # mined number really is mentioned in the document.
                    c.execute(
                        "UPDATE file_vorlagen SET own=0 WHERE file_id=? AND vorlage<>?",
                        (fid, base))
                    c.execute(
                        "INSERT OR REPLACE INTO file_vorlagen(file_id,vorlage,own) VALUES(?,?,1)",
                        (fid, base))
                    c.execute("UPDATE files SET vorlage=? WHERE id=?", (base, fid))
            if base:
                for (tid, _anchor, _dt) in stations:
                    carrier = c.execute(
                        """SELECT id FROM files WHERE meeting_id=?
                           AND doc_type IN ('Tagesordnung','Einladung')
                           ORDER BY id LIMIT 1""", (tid,)).fetchone()
                    if not carrier:
                        carrier = c.execute(
                            """SELECT id FROM files WHERE meeting_id=?
                               AND agenda_anchor IS NULL ORDER BY id LIMIT 1""",
                            (tid,)).fetchone()
                    if carrier and carrier["id"] not in fids:
                        # IGNORE: never downgrade an own=1 row to a mention.
                        c.execute(
                            "INSERT OR IGNORE INTO file_vorlagen(file_id,vorlage,own) VALUES(?,?,0)",
                            (carrier["id"], base))


# ---------------------------------------------------------------------------
# Sync orchestration
# ---------------------------------------------------------------------------

def _default_since() -> str:
    d = datetime.now(_BERLIN) - timedelta(days=BACKFILL_MONTHS * 31)
    return d.isoformat(timespec="seconds")


def sync(full: bool = False, since: Optional[str] = None,
         enable_llm: bool = ENABLE_LLM, skip_members: bool = False) -> dict:
    os.makedirs(filestore.PDF_DIR, exist_ok=True)
    db.init_db()
    cli = Client()
    counts = {"meetings": 0, "papers": 0, "files_new": 0, "files_seen": 0,
              "text_ok": 0, "enriched": 0, "errors": 0}
    # Watermark: capture run start BEFORE walking so objects modified mid-run
    # fall into the next window.
    run_start = datetime.now(_BERLIN).isoformat(timespec="seconds")
    if full:
        watermark = since  # may be None -> walk everything
    else:
        watermark = since or db.get_meta("oparl_since") or _default_since()
    recheck_cutoff = (date.today() - timedelta(days=RECHECK_DAYS)).isoformat()
    log.info("oparl sync start (since=%s, recheck>=%s)", watermark, recheck_cutoff)

    try:
        # 1. committees + member rosters — HTML (party data is OParl-absent)
        if not skip_members:
            committees, mcount = scraper.sync_committees_members(cli.f)
            counts["members"] = mcount

        failed: list = []
        ing = Ingest(cli, recheck_cutoff, counts, failed)

        # 2. meetings (incremental via modified_since)
        visited: set[str] = set()
        for m in cli.iter_list(f"{BASE}/bodies/{BODY}/meetings", modified_since=watermark):
            try:
                tid = ing.ingest_meeting(m)
                if tid:
                    visited.add(tid)
            except Exception as e:  # noqa: BLE001
                log.warning("meeting %s failed: %s", m.get("id"), e)
                counts["errors"] += 1
                failed.append(m.get("modified") or "")

        # 3. recently-held meetings not in the modified window: re-fetch so
        # late-added Abstimmungsergebnis/Niederschrift files are caught even if
        # the vendor doesn't bump meeting.modified for new attachments.
        for tid in db.recent_meeting_ids(recheck_cutoff):
            if tid in visited:
                continue
            num = tid.split("-", 1)[1]
            m = cli.get_json(f"{BASE}/bodies/{BODY}/meetings/{num}")
            if not m:
                counts["errors"] += 1
                continue
            try:
                ing.ingest_meeting(m)
            except Exception as e:  # noqa: BLE001
                log.warning("recheck meeting %s failed: %s", tid, e)
                counts["errors"] += 1
                # Deliberately NOT recorded as a watermark failure: these
                # meetings are outside the modified window by construction, so
                # holding the watermark back can't reach them. Step 3 itself is
                # the recovery — it re-runs by date every run inside RECHECK_DAYS.

        # 4. papers (Vorlagen/Anträge + their lifecycle via consultations)
        for p in cli.iter_list(f"{BASE}/bodies/{BODY}/papers", modified_since=watermark):
            try:
                ing.ingest_paper(p)
            except Exception as e:  # noqa: BLE001
                log.warning("paper %s failed: %s", p.get("id"), e)
                counts["errors"] += 1
                failed.append(p.get("modified") or "")

        # 5. enrichment + vote-PDF parsing (identical to the HTML pipeline)
        if enable_llm:
            counts["enriched"] += scraper._enrich_pending(counts)
        counts["votes"] = scraper.parse_pending_votes()

        # Hold the watermark back to just before the oldest object we failed to
        # ingest, so the next run re-walks it. Advancing past a swallowed error
        # drops that object for good: the vendor won't bump its `modified`
        # again, and only meetings get a date-based safety net (step 3) — papers
        # have none. This is the same guarantee iter_list protects by raising
        # rather than breaking mid-walk.
        # Back the watermark off by the overlap margin (see WATERMARK_OVERLAP_HOURS).
        next_since = (datetime.fromisoformat(run_start)
                      - timedelta(hours=WATERMARK_OVERLAP_HOURS)).isoformat(timespec="seconds")
        if failed:
            # An object carrying no `modified` stamp can't be placed in time;
            # fall back to re-walking the window we just walked. Never let a
            # blank stamp reach set_meta — it reads back falsy and silently
            # resets the watermark to the default backfill window.
            stamps = [t for t in failed if t]
            oldest = (min(stamps, key=_iso_key) if stamps
                      else (watermark or run_start))
            # Take the EARLIER of the overlap margin and the oldest failure —
            # comparing against run_start instead would let a recent failure
            # push the watermark forward and cancel the overlap.
            if _iso_key(oldest) < _iso_key(next_since):
                counts["held_watermark"] = oldest
                log.warning("%d object(s) failed — holding watermark at %s (not %s)",
                            len(failed), oldest, next_since)
                next_since = oldest

        with db.write_conn() as c:
            db.set_meta(c, "oparl_since", next_since)
            db.set_meta(c, "last_scrape", datetime.now().isoformat(timespec="seconds"))
            db.set_meta(c, "last_scrape_status", f"ok oparl {counts}")
        log.info("oparl sync done: %s", counts)
        return counts
    except Exception as e:
        try:
            with db.write_conn() as c:
                db.set_meta(c, "last_scrape_status", f"error oparl {e}")
        except Exception:  # noqa: BLE001
            pass
        raise
    finally:
        cli.close()


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout,
    )
    ap = argparse.ArgumentParser(description="Karlsruhe OParl ingester")
    ap.add_argument("--full", action="store_true",
                    help="ignore the watermark: walk the complete meeting/paper lists")
    ap.add_argument("--since", help="explicit modified_since ISO timestamp")
    ap.add_argument("--no-llm", action="store_true", help="skip LLM enrichment this run")
    ap.add_argument("--skip-members", action="store_true",
                    help="skip the HTML committee/roster crawl (validation runs)")
    args = ap.parse_args()
    counts = sync(full=args.full, since=args.since,
                  enable_llm=ENABLE_LLM and not args.no_llm,
                  skip_members=args.skip_members)
    print(counts)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
