"""
Desinformationssystem — shared ingestion plumbing.

Everything both ingesters (scraper.py = HTML, oparl.py = OParl API) need:
  - the polite retrying Fetcher,
  - doc-type classification from a label,
  - Vorlagennummer extraction,
  - PDF text extraction (pdftotext, optional OCR),
  - process_file(): download → sha256-dedupe → pdftotext → version-archive →
    vorlage-extraction → DB upsert + FTS reindex.

This module was factored out of scraper.py (2026-07-24) so the OParl ingester
can reuse the exact download/dedupe/versioning path instead of duplicating it.
scraper.py re-exports the public names for backward compatibility.
"""
from __future__ import annotations

import hashlib
import json
import logging
import os
import re
import shutil
import subprocess
import time
from typing import Optional

import httpx

import db
import submitter

log = logging.getLogger("filestore")

USER_AGENT = os.environ.get(
    "USER_AGENT",
    "DesinfoBot/1.0 (+personal Karlsruhe Sitzungskalender archive; contact rmv@rmv.fyi)",
)
REQUEST_INTERVAL = float(os.environ.get("REQUEST_INTERVAL", "1.5"))   # seconds between requests
MAX_RETRIES = int(os.environ.get("MAX_RETRIES", "5"))
PDF_DIR = os.environ.get("PDF_DIR", os.path.join(os.path.dirname(__file__), "data", "pdfs"))
ENABLE_OCR = os.environ.get("ENABLE_OCR", "0") == "1"
FLARESOLVERR_URL = os.environ.get("FLARESOLVERR_URL", "")  # e.g. http://flaresolverr:8191
PDFTOTEXT_MAXCHARS = int(os.environ.get("PDFTOTEXT_MAXCHARS", "400000"))

# label keyword -> document type (checked in order — most specific first, since
# matching is substring-based: "wortprotokoll" contains "protokoll", and a
# combined "Protokoll mit Abstimmungsergebnis" must classify as the latter so
# parse_pending_votes picks it up)
DOC_TYPE_RULES = [
    ("niederschrift", "Niederschrift"),
    ("abstimmungsergebnis", "Abstimmungsergebnis"),
    ("wortprotokoll", "Wortprotokoll"),
    ("protokoll", "Protokoll"),
    ("beschlussvorlage", "Beschlussvorlage"),
    ("beschluss", "Beschluss"),
    ("einladung", "Einladung"),
    ("tagesordnung", "Tagesordnung"),
    ("antrag", "Antrag"),
    ("anfrage", "Anfrage"),
    ("vorlage", "Vorlage"),
    ("präsentation", "Präsentation"),
    ("praesentation", "Präsentation"),
    ("geschäftsordnung", "Geschäftsordnung"),
    ("anlage", "Anlage"),
    ("plan", "Plan"),
    ("karte", "Karte"),
]


def classify(label: str) -> str:
    low = (label or "").lower()
    for kw, t in DOC_TYPE_RULES:
        if kw in low:
            return t
    return "Sonstiges"


# --- Vorlagennummer extraction --------------------------------------------
# Headers read "Vorlage: 2026/0324"; agenda/minutes documents mention many
# numbers — those mentions are the lifecycle stations of a Vorlage.
_VORLAGE_RE = re.compile(
    r"(?:Vorlage|Drucksache)(?:n-?\s?Nr\.?|nummer)?\s*[:\s]\s*(20\d{2}\s*[/-]\s*\d{3,5})")
# Doc types whose header number is the document's OWN Vorlagennummer.
_OWN_VORLAGE_TYPES = {"Beschlussvorlage", "Vorlage", "Antrag", "Anfrage", "Beschluss"}


def _valid_vorlage(nr: str) -> Optional[str]:
    # normalise "2026-0324" / "2026 / 0324" to the canonical "2026/0324"
    nr = re.sub(r"\s*", "", nr).replace("-", "/")
    year, seq = nr.split("/")
    # "2026/2027" is a budget-year range, not a Vorlagennummer
    if len(seq) == 4 and seq.startswith("20") and abs(int(seq) - int(year)) <= 1:
        return None
    return nr


def extract_vorlagen(label: str, text: str, doc_type: str) -> tuple[Optional[str], set[str]]:
    """(own_number, all_referenced_numbers) from a document's label + text."""
    head = f"{label or ''}\n{(text or '')[:1500]}"
    own = None
    if doc_type in _OWN_VORLAGE_TYPES:
        for m in _VORLAGE_RE.finditer(head):
            own = _valid_vorlage(m.group(1))
            if own:
                break
    refs = set()
    for m in _VORLAGE_RE.finditer(f"{label or ''}\n{(text or '')[:200000]}"):
        nr = _valid_vorlage(m.group(1))
        if nr:
            refs.add(nr)
    return own, refs


# ---------------------------------------------------------------------------
# Fetcher (polite, retrying)
# ---------------------------------------------------------------------------

class Fetcher:
    def __init__(self) -> None:
        self.client = httpx.Client(
            headers={"User-Agent": USER_AGENT, "Accept-Language": "de-DE,de;q=0.9"},
            follow_redirects=True,
            timeout=httpx.Timeout(60.0),
        )
        self._last = 0.0

    def _throttle(self) -> None:
        wait = REQUEST_INTERVAL - (time.monotonic() - self._last)
        if wait > 0:
            time.sleep(wait)
        self._last = time.monotonic()

    def get(self, url: str, *, binary: bool = False) -> Optional[httpx.Response]:
        """GET with throttle + exponential backoff. Returns None on persistent failure."""
        for attempt in range(MAX_RETRIES):
            self._throttle()
            try:
                r = self.client.get(url)
            except httpx.HTTPError as e:
                log.warning("request error %s (%s) attempt %d", url, e, attempt + 1)
                if attempt < MAX_RETRIES - 1:
                    time.sleep(2 ** attempt)
                continue
            ct = r.headers.get("content-type", "")
            ok = r.status_code == 200 and (binary or "text/html" in ct or "json" in ct)
            # web1 returns a 404 *page* when rate-limited; treat 404/429/5xx as retryable
            if ok:
                return r
            if r.status_code in (403,) and FLARESOLVERR_URL and not binary:
                fs = self._flaresolverr(url)
                if fs is not None:
                    return fs
            log.warning("bad status %s for %s (attempt %d/%d)", r.status_code, url, attempt + 1, MAX_RETRIES)
            if attempt < MAX_RETRIES - 1:
                time.sleep(min(60, 3 * (2 ** attempt)))
        log.error("giving up on %s", url)
        return None

    def head(self, url: str) -> Optional[httpx.Response]:
        """Cheap HEAD for change detection (Last-Modified / Content-Length)."""
        for attempt in range(2):
            self._throttle()
            try:
                r = self.client.head(url)
                if r.status_code == 200:
                    return r
            except httpx.HTTPError as e:
                log.debug("head error %s: %s", url, e)
            time.sleep(2 ** attempt)
        return None

    def _flaresolverr(self, url: str) -> Optional[httpx.Response]:
        try:
            resp = self.client.post(
                f"{FLARESOLVERR_URL}/v1",
                json={"cmd": "request.get", "url": url, "maxTimeout": 60000},
                timeout=90.0,
            )
            sol = resp.json().get("solution", {})
            if sol.get("status") == 200:
                fake = httpx.Response(200, text=sol.get("response", ""), request=httpx.Request("GET", url))
                return fake
        except Exception as e:  # noqa: BLE001
            log.warning("flaresolverr failed for %s: %s", url, e)
        return None

    def close(self) -> None:
        self.client.close()


# ---------------------------------------------------------------------------
# Text extraction
# ---------------------------------------------------------------------------

def extract_text(pdf_path: str) -> tuple[str, str]:
    """Return (status, text). status in ok|empty|needs_ocr|error."""
    try:
        out = subprocess.run(
            ["pdftotext", "-q", "-enc", "UTF-8", pdf_path, "-"],
            capture_output=True, timeout=180,
        )
        text = out.stdout.decode("utf-8", "replace").strip()
    except Exception as e:  # noqa: BLE001
        log.warning("pdftotext failed for %s: %s", pdf_path, e)
        return "error", ""
    if len(text) >= 20:
        return "ok", text[:PDFTOTEXT_MAXCHARS]
    if ENABLE_OCR:
        text = _ocr(pdf_path)
        if len(text) >= 20:
            return "ok", text[:PDFTOTEXT_MAXCHARS]
    return ("needs_ocr" if not ENABLE_OCR else "empty"), text


def _ocr(pdf_path: str) -> str:
    try:
        tmp = pdf_path + ".ocr.pdf"
        subprocess.run(
            ["ocrmypdf", "-l", "deu", "--skip-text", "--optimize", "0", pdf_path, tmp],
            capture_output=True, timeout=900, check=True,
        )
        out = subprocess.run(["pdftotext", "-q", "-enc", "UTF-8", tmp, "-"],
                             capture_output=True, timeout=180)
        os.remove(tmp)
        return out.stdout.decode("utf-8", "replace").strip()
    except Exception as e:  # noqa: BLE001
        log.warning("OCR failed for %s: %s", pdf_path, e)
        return ""


def _sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


# ---------------------------------------------------------------------------
# The shared file pipeline
# ---------------------------------------------------------------------------

def process_file(f: Fetcher, fmeta: dict, meeting_id: str, counts: dict,
                 recheck: bool = False) -> bool:
    """Insert/refresh a file row; download+extract if new (or changed, when recheck).
    Returns True if newly downloaded or re-downloaded after a change.

    fmeta: {id, url, label, agenda_anchor, doc_type, filename}."""
    fid = fmeta["id"]
    conn = db.get_conn()
    existing = conn.execute(
        "SELECT id, sha256, local_path, size, remote_modified, downloaded_at FROM files WHERE id=?",
        (fid,)).fetchone()

    if existing and existing["local_path"] and os.path.exists(existing["local_path"]):
        changed = False
        if recheck:
            # cheap HEAD: re-download only if a KNOWN Last-Modified/size actually changed.
            h = f.head(fmeta["url"])
            if h is not None:
                lm = h.headers.get("last-modified")
                cl = h.headers.get("content-length")
                prior = existing["remote_modified"]
                if prior is None:
                    # no baseline recorded yet — backfill it, don't re-download
                    if lm:
                        with db.write_conn() as c:
                            c.execute("UPDATE files SET remote_modified=? WHERE id=?", (lm, fid))
                else:
                    try:
                        size_changed = bool(cl) and bool(existing["size"]) and int(cl) != existing["size"]
                    except ValueError:  # malformed Content-Length
                        size_changed = False
                    if (lm and lm != prior) or size_changed:
                        changed = True
                        log.info("file %s changed on server (recheck) — re-downloading", fid)
        if not changed:
            counts["files_seen"] += 1
            with db.write_conn() as c:  # keep association/labels fresh
                c.execute("""UPDATE files SET meeting_id=?, agenda_anchor=?, label=?,
                                              doc_type=? WHERE id=?""",
                          (meeting_id, fmeta["agenda_anchor"], fmeta["label"],
                           fmeta["doc_type"], fid))
                db.reindex_file(c, fid)
            return False

    # download (new file, or changed file on a recent meeting)
    r = f.get(fmeta["url"], binary=True)
    if not r:
        counts["errors"] += 1
        return False
    content = r.content
    if not content.lstrip()[:5].startswith(b"%PDF"):
        # A 200 that isn't a PDF (maintenance page, rate-limit interstitial) must
        # never clobber a known-good cached file.
        log.warning("file %s: response is not a PDF (%d bytes) — keeping existing", fid, len(content))
        counts["errors"] += 1
        return False
    sha = hashlib.sha256(content).hexdigest()
    is_update = bool(existing)
    if is_update and existing["sha256"] == sha:
        # content identical after all — just refresh metadata, don't re-enrich
        counts["files_seen"] += 1
        with db.write_conn() as c:
            c.execute("""UPDATE files SET remote_modified=?, meeting_id=?, agenda_anchor=?,
                                          label=?, doc_type=? WHERE id=?""",
                      (r.headers.get("last-modified"), meeting_id, fmeta["agenda_anchor"],
                       fmeta["label"], fmeta["doc_type"], fid))
            db.reindex_file(c, fid)  # label/meeting join columns feed the FTS index
        return False
    os.makedirs(PDF_DIR, exist_ok=True)
    local = os.path.join(PDF_DIR, f"{fid}.pdf")
    # Version history: before replacing an in-place-updated PDF, archive the
    # superseded bytes (copy, not move, so /api/file never sees a gap).
    archived = None
    if (is_update and existing["sha256"] and existing["local_path"]
            and os.path.exists(existing["local_path"])):
        vdir = os.path.join(PDF_DIR, "versions")
        os.makedirs(vdir, exist_ok=True)
        vpath = os.path.join(vdir, f"{fid}.{existing['sha256'][:12]}.pdf")
        try:
            if not os.path.exists(vpath):
                shutil.copy2(existing["local_path"], vpath)
            archived = vpath
        except OSError as e:
            log.warning("could not archive old version of %s: %s", fid, e)
    # Write to a temp file, extract from it, then atomically rename into place —
    # the daily scraper and the web refresh path can race on the same fid, and
    # readers (/api/file) must never see a half-written PDF.
    tmp = f"{local}.tmp.{os.getpid()}"
    try:
        with open(tmp, "wb") as fh:
            fh.write(content)
        status, text = extract_text(tmp)
        os.replace(tmp, local)
    finally:
        if os.path.exists(tmp):
            try:
                os.remove(tmp)
            except OSError:
                pass
    if status == "ok":
        counts["text_ok"] += 1
    counts["files_updated" if is_update else "files_new"] = \
        counts.get("files_updated" if is_update else "files_new", 0) + 1
    subs = json.dumps(submitter.derive(fmeta["label"], text, fmeta["doc_type"]),
                      ensure_ascii=False)
    with db.write_conn() as c:
        # New OR changed: (re)set enrich_status to 'pending' so it gets (re)enriched.
        c.execute(
            """INSERT INTO files(id,url,meeting_id,agenda_anchor,label,doc_type,filename,
                                 sha256,size,remote_modified,local_path,text_status,fulltext,
                                 submitters,enrich_status,downloaded_at)
               VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'pending',datetime('now'))
               ON CONFLICT(id) DO UPDATE SET url=excluded.url, meeting_id=excluded.meeting_id,
                 agenda_anchor=excluded.agenda_anchor, label=excluded.label,
                 doc_type=excluded.doc_type, sha256=excluded.sha256, size=excluded.size,
                 remote_modified=excluded.remote_modified, local_path=excluded.local_path,
                 text_status=excluded.text_status, fulltext=excluded.fulltext,
                 submitters=excluded.submitters,
                 enrich_status='pending', downloaded_at=datetime('now')""",
            (fid, fmeta["url"], meeting_id, fmeta["agenda_anchor"], fmeta["label"],
             fmeta["doc_type"], fmeta["filename"], sha, len(content),
             r.headers.get("last-modified"), local, status, text, subs))
        if archived:
            db.add_file_version(c, fid, existing["sha256"], existing["size"],
                                existing["remote_modified"], archived,
                                existing["downloaded_at"])
        own, refs = extract_vorlagen(fmeta["label"], text, fmeta["doc_type"])
        db.set_file_vorlagen(c, fid, own, refs)
        db.reindex_file(c, fid)
    return True
