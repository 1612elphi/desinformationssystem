"""
Desinformationssystem — SQLite schema + FTS5 + query helpers.

Shared by the scraper, the web API and the MCP server. Single SQLite file in
WAL mode so the three processes can read concurrently while the scraper writes.

Hierarchy mirrors the Karlsruhe Ratsinformationssystem:
    bodies (Gremien) -> meetings (Termine) -> agenda_items (TOPs)
    files (Dateien / the PDFs) attach to a meeting and optionally an agenda item.
"""
from __future__ import annotations

import json
import os
import re
import sqlite3
import threading
from contextlib import contextmanager
from datetime import datetime, timedelta
from typing import Any, Iterable, Optional
from zoneinfo import ZoneInfo

DB_PATH = os.environ.get("DB_PATH", os.path.join(os.path.dirname(__file__), "data", "desinformationssystem.db"))

_local = threading.local()

SCHEMA = """
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);

-- Gremien (committees / bodies)
CREATE TABLE IF NOT EXISTS bodies (
    id        TEXT PRIMARY KEY,            -- organisation-gr-NNNNN slug
    name      TEXT NOT NULL,
    url       TEXT,
    first_seen TEXT DEFAULT (datetime('now')),
    last_seen  TEXT DEFAULT (datetime('now'))
);

-- Termine (meetings)
CREATE TABLE IF NOT EXISTS meetings (
    id          TEXT PRIMARY KEY,          -- termin-NNNNN slug
    body_id     TEXT REFERENCES bodies(id),
    body_name   TEXT,                      -- denormalised committee name as shown on the meeting
    title       TEXT,
    date        TEXT,                      -- ISO date (YYYY-MM-DD)
    time        TEXT,
    location    TEXT,
    public      INTEGER,                   -- 1 public, 0 non-public, NULL mixed/unknown
    url         TEXT,
    scraped_at  TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_meetings_date ON meetings(date);
CREATE INDEX IF NOT EXISTS idx_meetings_body ON meetings(body_id);

-- Committee membership (parsed from the committee org pages)
CREATE TABLE IF NOT EXISTS members (
    body_id    TEXT REFERENCES bodies(id),
    person_id  TEXT,
    name       TEXT,
    party      TEXT,                       -- raw party text, e.g. "GRÜNE", "FDP/FW"
    party_code TEXT,                        -- derived submitter code (B90, FWV, …)
    role       TEXT,                        -- function: Vorsitz / Ordentliches Mitglied / …
    since      TEXT,
    PRIMARY KEY (body_id, person_id)
);
CREATE INDEX IF NOT EXISTS idx_members_body ON members(body_id);
CREATE INDEX IF NOT EXISTS idx_members_person ON members(person_id);

-- Tagesordnungspunkte (agenda items)
CREATE TABLE IF NOT EXISTS agenda_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_id  TEXT REFERENCES meetings(id),
    anchor      TEXT,                      -- e.g. top3 / top3.1
    number      TEXT,
    title       TEXT,
    public      INTEGER,
    UNIQUE(meeting_id, anchor)
);
CREATE INDEX IF NOT EXISTS idx_agenda_meeting ON agenda_items(meeting_id);

-- Dateien (the PDF documents)
CREATE TABLE IF NOT EXISTS files (
    id            TEXT PRIMARY KEY,        -- stable numeric file id from the download URL
    url           TEXT NOT NULL,
    meeting_id    TEXT REFERENCES meetings(id),
    agenda_anchor TEXT,                    -- which TOP it hung under, if any
    label         TEXT,                    -- anchor text from the RIS ("Beschlussvorlage", ...)
    doc_type      TEXT,                    -- rule-based classification from the label
    filename      TEXT,
    sha256        TEXT,
    size          INTEGER,
    remote_modified TEXT,                  -- Last-Modified header, for change detection
    local_path    TEXT,
    text_status   TEXT DEFAULT 'pending',  -- pending | ok | empty | needs_ocr | error
    fulltext      TEXT,
    -- LLM enrichment
    enrich_status TEXT DEFAULT 'pending',  -- pending | ok | skipped | error
    summary_de    TEXT,
    summary_en    TEXT,
    category      TEXT,
    topics        TEXT,                    -- JSON array
    entities      TEXT,                    -- JSON object {people,orgs,locations}
    submitters    TEXT,                    -- JSON array of submitter codes (SVK/CDU/B90/…)
    downloaded_at TEXT,
    enriched_at   TEXT,
    first_seen    TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_files_meeting   ON files(meeting_id);
CREATE INDEX IF NOT EXISTS idx_files_doctype   ON files(doc_type);
CREATE INDEX IF NOT EXISTS idx_files_textstat  ON files(text_status);
CREATE INDEX IF NOT EXISTS idx_files_enrich    ON files(enrich_status);

-- Abstimmungsergebnisse (vote tallies parsed from the live ticker / result PDFs).
CREATE TABLE IF NOT EXISTS votes (
    id            TEXT PRIMARY KEY,        -- meeting_id || ':' || top_label
    meeting_id    TEXT REFERENCES meetings(id),
    agenda_anchor TEXT,                     -- topN, for grouping under the agenda
    top_label     TEXT,                     -- "TOP 1"
    result_text   TEXT,                     -- e.g. "einstimmige Zustimmung" (from the ticker)
    ja            INTEGER,
    nein          INTEGER,
    enthaltung    INTEGER,
    members       TEXT,                     -- JSON [{name, vote}]
    source        TEXT,                     -- 'live' | 'pdf'
    image_url     TEXT,
    image_sha     TEXT,
    file_id       TEXT,                     -- source PDF file id (pdf source)
    parsed_at     TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_votes_meeting ON votes(meeting_id);

-- Vote-PDF parse attempts, so permanently unreadable scans stop re-billing the
-- vision model on every run (see scraper.parse_pending_votes).
CREATE TABLE IF NOT EXISTS vote_parse_attempts (
    file_id      TEXT PRIMARY KEY,
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_attempt TEXT
);

-- Full-text search over label + extracted text + summaries + topics.
-- Regular (not contentless) FTS5 so we can DELETE+reINSERT on re-enrichment;
-- rows are keyed on files.fts_id, a stable integer we assign ourselves
-- (the implicit rowid of a TEXT-PK table may be renumbered by VACUUM).
CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
    label, fulltext, summary_de, summary_en, topics, category,
    body_name, meeting_title
);
"""


def _connect() -> sqlite3.Connection:
    os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
    conn = sqlite3.connect(DB_PATH, timeout=30.0)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL;")
    conn.execute("PRAGMA busy_timeout=30000;")
    conn.execute("PRAGMA foreign_keys=ON;")
    return conn


def get_conn() -> sqlite3.Connection:
    """Thread-local connection (FastAPI/MCP run multi-threaded)."""
    conn = getattr(_local, "conn", None)
    if conn is None:
        conn = _local.conn = _connect()
    return conn


@contextmanager
def write_conn():
    """A dedicated connection for the scraper's writes."""
    conn = _connect()
    try:
        yield conn
        conn.commit()
    finally:
        conn.close()


def init_db() -> None:
    conn = _connect()
    try:
        conn.executescript(SCHEMA)
        _migrate(conn)
        conn.commit()
    finally:
        conn.close()


def _migrate(conn: sqlite3.Connection) -> None:
    """Idempotent column adds for DBs created before a column existed."""
    cols = {r["name"] for r in conn.execute("PRAGMA table_info(files)")}
    for name, decl in (("remote_modified", "TEXT"), ("submitters", "TEXT"),
                       ("fts_id", "INTEGER")):
        if name not in cols:
            conn.execute(f"ALTER TABLE files ADD COLUMN {name} {decl}")
    # Existing FTS rows were keyed on the implicit rowid; snapshot it into fts_id
    # once so the index stays valid and future VACUUMs can't shear the mapping.
    conn.execute("UPDATE files SET fts_id = rowid WHERE fts_id IS NULL")
    conn.execute("CREATE UNIQUE INDEX IF NOT EXISTS idx_files_ftsid ON files(fts_id)")


# ---------------------------------------------------------------------------
# FTS helpers
# ---------------------------------------------------------------------------

def reindex_file(conn: sqlite3.Connection, file_id: str) -> None:
    """Refresh the FTS row for one file, keyed on files.fts_id (stable integer)."""
    row = conn.execute(
        """SELECT f.fts_id AS rid, f.label, f.fulltext, f.summary_de, f.summary_en,
                  f.topics, f.category, m.body_name, m.title AS meeting_title
           FROM files f LEFT JOIN meetings m ON m.id = f.meeting_id
           WHERE f.id = ?""",
        (file_id,),
    ).fetchone()
    if not row:
        return
    if row["rid"] is None:
        # SQLite serialises writers, so MAX+1 inside this write txn can't race.
        conn.execute(
            "UPDATE files SET fts_id = (SELECT COALESCE(MAX(fts_id),0)+1 FROM files) WHERE id = ?",
            (file_id,))
        rid = conn.execute("SELECT fts_id FROM files WHERE id = ?", (file_id,)).fetchone()[0]
        row = dict(row) | {"rid": rid}
    conn.execute("DELETE FROM files_fts WHERE rowid = ?", (row["rid"],))
    conn.execute(
        """INSERT INTO files_fts(rowid, label, fulltext, summary_de, summary_en,
                                 topics, category, body_name, meeting_title)
           VALUES (?,?,?,?,?,?,?,?,?)""",
        (row["rid"], row["label"] or "", row["fulltext"] or "", row["summary_de"] or "",
         row["summary_en"] or "", row["topics"] or "", row["category"] or "",
         row["body_name"] or "", row["meeting_title"] or ""),
    )


# ---------------------------------------------------------------------------
# Query helpers (read side: web + MCP)
# ---------------------------------------------------------------------------

def _row_to_doc(row: sqlite3.Row) -> dict[str, Any]:
    d = dict(row)
    for k in ("topics", "entities", "submitters"):
        if d.get(k):
            try:
                d[k] = json.loads(d[k])
            except (ValueError, TypeError):
                pass
    return d


def search_documents(
    query: Optional[str] = None,
    committee: Optional[str] = None,
    doc_type: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    public: Optional[bool] = None,
    topic: Optional[str] = None,
    submitter: Optional[str] = None,
    limit: int = 50,
    offset: int = 0,
) -> dict[str, Any]:
    """Faceted full-text search. Returns {total, results:[...]}."""
    conn = get_conn()
    where: list[str] = []
    params: list[Any] = []

    if query:
        where.append("files_fts MATCH ?")
        params.append(_fts_query(query))
    if committee:
        where.append("(m.body_name = ? OR m.body_id = ?)")
        params.extend([committee, committee])
    if doc_type:
        where.append("f.doc_type = ?")
        params.append(doc_type)
    if date_from:
        where.append("m.date >= ?")
        params.append(date_from)
    if date_to:
        where.append("m.date <= ?")
        params.append(date_to)
    if public is not None:
        where.append("m.public = ?")
        params.append(1 if public else 0)
    if topic:
        where.append("f.topics LIKE ?")
        params.append(f'%"{topic}"%')
    if submitter:
        where.append("f.submitters LIKE ?")
        params.append(f'%"{submitter}"%')

    where_sql = (" WHERE " + " AND ".join(where)) if where else ""

    agenda_join = (
        " LEFT JOIN agenda_items ai "
        "ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor "
    )
    agenda_cols = " ai.number AS agenda_number, ai.title AS agenda_title,"
    if query:
        base = f"""
            FROM files f
            JOIN files_fts ON files_fts.rowid = f.fts_id
            LEFT JOIN meetings m ON m.id = f.meeting_id
            {agenda_join}
            {where_sql}
        """
        sql_count = f"SELECT COUNT(*) AS n {base}"
        sql = f"""SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
                         m.title AS meeting_title, m.public AS meeting_public,
                        {agenda_cols} bm25(files_fts) AS rank
                  {base} ORDER BY rank LIMIT ? OFFSET ?"""
    else:
        base = f"""
            FROM files f
            LEFT JOIN meetings m ON m.id = f.meeting_id
            {agenda_join}
        """ + where_sql
        sql_count = f"SELECT COUNT(*) AS n {base}"
        sql = f"""SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
                         m.title AS meeting_title, m.public AS meeting_public,
                        {agenda_cols.rstrip(',')}
                  {base} ORDER BY m.date DESC, f.id DESC LIMIT ? OFFSET ?"""

    total = conn.execute(sql_count, params).fetchone()["n"]
    rows = conn.execute(sql, [*params, limit, offset]).fetchall()
    results = [_row_to_doc(r) for r in rows]
    # Full extracted text is huge (up to 400k chars/doc) and this is a public,
    # unauthenticated endpoint — list responses carry metadata only; fetch the
    # text for one document via get_document.
    for d in results:
        d.pop("fulltext", None)
    return {"total": total, "results": results}


def _fts_query(q: str) -> str:
    """Make a safe FTS5 MATCH string: quote each token, AND them, allow prefix."""
    tokens = [t for t in _tokenize(q) if t]
    if not tokens:
        return '""'
    return " ".join(f'"{t}"*' for t in tokens)


def _tokenize(q: str) -> Iterable[str]:
    cur = []
    for ch in q:
        if ch.isalnum() or ch in "äöüßÄÖÜ-":
            cur.append(ch)
        else:
            if cur:
                yield "".join(cur)
                cur = []
    if cur:
        yield "".join(cur)


def get_document(file_id: str) -> Optional[dict[str, Any]]:
    conn = get_conn()
    row = conn.execute(
        """SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
                  m.title AS meeting_title, m.location, m.public AS meeting_public,
                  ai.number AS agenda_number, ai.title AS agenda_title
           FROM files f
           LEFT JOIN meetings m ON m.id = f.meeting_id
           LEFT JOIN agenda_items ai
             ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor
           WHERE f.id = ?""",
        (file_id,),
    ).fetchone()
    return _row_to_doc(row) if row else None


def list_committees() -> list[dict[str, Any]]:
    conn = get_conn()
    rows = conn.execute(
        """SELECT b.id, b.name,
                  (SELECT COUNT(*) FROM members mb WHERE mb.body_id = b.id) AS members,
                  (SELECT COUNT(*) FROM meetings m WHERE m.body_id = b.id) AS meetings,
                  (SELECT COUNT(*) FROM files f JOIN meetings m ON m.id=f.meeting_id
                     WHERE m.body_id = b.id) AS documents
           FROM bodies b ORDER BY b.name"""
    ).fetchall()
    return [dict(r) for r in rows]


def list_meetings(
    committee: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    order: str = "desc",
    limit: int = 100,
    offset: int = 0,
) -> list[dict[str, Any]]:
    conn = get_conn()
    where, params = [], []
    if committee:
        where.append("(body_name = ? OR body_id = ?)")
        params.extend([committee, committee])
    if date_from:
        where.append("date >= ?"); params.append(date_from)
    if date_to:
        where.append("date <= ?"); params.append(date_to)
    where_sql = (" WHERE " + " AND ".join(where)) if where else ""
    direction = "ASC" if str(order).lower() == "asc" else "DESC"
    rows = conn.execute(
        f"""SELECT m.*,
                   (SELECT COUNT(*) FROM files f WHERE f.meeting_id=m.id) AS documents,
                   (SELECT COUNT(*) FROM agenda_items a WHERE a.meeting_id=m.id) AS agenda_count
            FROM meetings m {where_sql}
            ORDER BY date {direction} LIMIT ? OFFSET ?""",
        [*params, limit, offset],
    ).fetchall()
    return [dict(r) for r in rows]


def count_meetings(
    committee: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
) -> int:
    conn = get_conn()
    where, params = [], []
    if committee:
        where.append("(body_name = ? OR body_id = ?)")
        params.extend([committee, committee])
    if date_from:
        where.append("date >= ?"); params.append(date_from)
    if date_to:
        where.append("date <= ?"); params.append(date_to)
    where_sql = (" WHERE " + " AND ".join(where)) if where else ""
    return conn.execute(f"SELECT COUNT(*) FROM meetings{where_sql}", params).fetchone()[0]


def get_committee(body_id: str) -> Optional[dict[str, Any]]:
    conn = get_conn()
    b = conn.execute("SELECT * FROM bodies WHERE id = ?", (body_id,)).fetchone()
    if not b:
        return None
    out = dict(b)
    out["members"] = [dict(r) for r in conn.execute(
        """SELECT person_id, name, party, party_code, role, since
           FROM members WHERE body_id = ? ORDER BY name""", (body_id,))]
    out["composition"] = [dict(r) for r in conn.execute(
        """SELECT COALESCE(NULLIF(party_code,''), party) AS code, COUNT(*) AS n
           FROM members WHERE body_id = ? GROUP BY code ORDER BY n DESC, code""", (body_id,))]
    out["document_count"] = conn.execute(
        """SELECT COUNT(*) FROM files f JOIN meetings m ON m.id=f.meeting_id
           WHERE m.body_id = ?""", (body_id,)).fetchone()[0]
    out["meetings"] = list_meetings(committee=body_id, limit=300)
    return out


def get_meeting(meeting_id: str) -> Optional[dict[str, Any]]:
    conn = get_conn()
    m = conn.execute("SELECT * FROM meetings WHERE id = ?", (meeting_id,)).fetchone()
    if not m:
        return None
    out = dict(m)
    out["agenda_items"] = [dict(r) for r in conn.execute(
        "SELECT * FROM agenda_items WHERE meeting_id = ? ORDER BY id", (meeting_id,))]
    out["files"] = [_row_to_doc(r) for r in conn.execute(
        """SELECT f.id, f.label, f.doc_type, f.filename, f.agenda_anchor, f.text_status,
                  f.enrich_status, f.category, f.summary_de, f.summary_en, f.topics,
                  f.submitters, f.url,
                  ai.number AS agenda_number, ai.title AS agenda_title
           FROM files f
           LEFT JOIN agenda_items ai
             ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor
           WHERE f.meeting_id = ? ORDER BY f.id""", (meeting_id,))]
    out["votes"] = meeting_votes(meeting_id)
    return out


# Numeric TOP ordering incl. dotted sub-items: "TOP 3.1" sorts as (3, 1), between
# TOP 3 and TOP 4 (CAST stops at the first non-digit, so CAST('3.1')=3).
_TOP_ORDER = """CAST(REPLACE(COALESCE({c},''),'TOP ','') AS INTEGER),
    CASE WHEN instr(REPLACE(COALESCE({c},''),'TOP ',''), '.') > 0
         THEN CAST(substr(REPLACE(COALESCE({c},''),'TOP ',''),
                          instr(REPLACE(COALESCE({c},''),'TOP ',''), '.') + 1) AS INTEGER)
         ELSE 0 END"""


def meeting_votes(meeting_id: str) -> list[dict[str, Any]]:
    conn = get_conn()
    rows = conn.execute(
        f"""SELECT id, agenda_anchor, top_label, result_text, ja, nein, enthaltung,
                  members, source, image_url FROM votes
           WHERE meeting_id = ?
           ORDER BY {_TOP_ORDER.format(c='top_label')},
                    top_label""", (meeting_id,)).fetchall()
    out = []
    for r in rows:
        d = dict(r)
        if d.get("members"):
            try:
                d["members"] = json.loads(d["members"])
            except (ValueError, TypeError):
                pass
        out.append(d)
    return out


def search_votes(
    committee: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    member: Optional[str] = None,
    query: Optional[str] = None,
    include_members: bool = False,
    limit: int = 50,
    offset: int = 0,
) -> dict[str, Any]:
    """Cross-meeting search over parsed vote tallies, newest meeting first.

    member matches roll-call names (json_each over the members JSON); matching
    entries are returned as member_votes. Full roll-calls only on include_members
    to keep list payloads small."""
    conn = get_conn()
    where, params = [], []
    if committee:
        where.append("(m.body_name = ? OR m.body_id = ?)")
        params.extend([committee, committee])
    if date_from:
        where.append("m.date >= ?"); params.append(date_from)
    if date_to:
        where.append("m.date <= ?"); params.append(date_to)
    if member:
        where.append(
            """EXISTS (SELECT 1 FROM json_each(COALESCE(v.members,'[]')) je
                       WHERE json_extract(je.value,'$.name') LIKE '%' || ? || '%')""")
        params.append(member)
    if query:
        where.append(
            "(v.top_label LIKE ? OR v.result_text LIKE ? OR COALESCE(ai.title,'') LIKE ?)")
        params.extend([f"%{query}%"] * 3)
    where_sql = (" WHERE " + " AND ".join(where)) if where else ""
    base = f"""FROM votes v
               JOIN meetings m ON m.id = v.meeting_id
               LEFT JOIN agenda_items ai
                 ON ai.meeting_id = v.meeting_id AND ai.anchor = v.agenda_anchor
               {where_sql}"""
    total = conn.execute(f"SELECT COUNT(*) {base}", params).fetchone()[0]
    rows = conn.execute(
        f"""SELECT v.meeting_id, v.agenda_anchor, v.top_label, v.result_text,
                   v.ja, v.nein, v.enthaltung, v.members, v.source, v.file_id,
                   m.body_name, m.body_id, m.date AS meeting_date,
                   ai.number AS agenda_number, ai.title AS agenda_title
            {base}
            ORDER BY m.date DESC,
                     {_TOP_ORDER.format(c='v.top_label')}
            LIMIT ? OFFSET ?""",
        [*params, limit, offset]).fetchall()
    needle = member.casefold() if member else None
    results = []
    for r in rows:
        d = dict(r)
        roll = None
        if d.get("members"):
            try:
                roll = json.loads(d["members"])
            except (ValueError, TypeError):
                roll = None
        if needle and roll:
            d["member_votes"] = [e for e in roll
                                 if needle in str(e.get("name", "")).casefold()]
        if include_members:
            d["members"] = roll
        else:
            d.pop("members")
        results.append(d)
    return {"total": total, "results": results}


def upsert_vote(conn: sqlite3.Connection, v: dict) -> None:
    """Insert/update a vote keyed by meeting_id:agenda_anchor.

    Precedence: the official Abstimmungsergebnis PDF (source='pdf') is authoritative — a
    'live' upsert never downgrades an existing 'pdf' row (enforced atomically via the
    conflict clause's WHERE, since ticker and scraper are separate processes). Empty
    tallies/members/result_text never clobber a stored non-empty value (COALESCE)."""
    anchor = v.get("agenda_anchor")
    if not anchor:
        return  # need an agenda anchor to place/key the vote
    vid = f"{v['meeting_id']}:{anchor}"
    members = v.get("members")
    members_json = json.dumps(members, ensure_ascii=False) if members else None
    conn.execute(
        """INSERT INTO votes(id,meeting_id,agenda_anchor,top_label,result_text,ja,nein,
                             enthaltung,members,source,image_url,image_sha,file_id,parsed_at)
           VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'))
           ON CONFLICT(id) DO UPDATE SET top_label=excluded.top_label,
             result_text=COALESCE(excluded.result_text, votes.result_text),
             ja=COALESCE(excluded.ja, votes.ja),
             nein=COALESCE(excluded.nein, votes.nein),
             enthaltung=COALESCE(excluded.enthaltung, votes.enthaltung),
             members=COALESCE(excluded.members, votes.members),
             source=excluded.source, image_url=excluded.image_url, image_sha=excluded.image_sha,
             file_id=COALESCE(excluded.file_id, votes.file_id), parsed_at=datetime('now')
           WHERE NOT (votes.source = 'pdf' AND excluded.source IS NOT 'pdf')""",
        (vid, v["meeting_id"], anchor, v["top_label"], v.get("result_text"),
         v.get("ja"), v.get("nein"), v.get("enthaltung"), members_json,
         v.get("source"), v.get("image_url"), v.get("image_sha"), v.get("file_id")))


def vote_parse_attempts(file_id: str) -> int:
    row = get_conn().execute(
        "SELECT attempts FROM vote_parse_attempts WHERE file_id = ?", (file_id,)).fetchone()
    return row["attempts"] if row else 0


def record_vote_parse_attempt(file_id: str, success: bool) -> None:
    """Track failed parses so unreadable scans stop re-billing the vision model;
    a success clears the counter (the file may later change in place)."""
    with write_conn() as c:
        if success:
            c.execute("DELETE FROM vote_parse_attempts WHERE file_id = ?", (file_id,))
        else:
            c.execute(
                """INSERT INTO vote_parse_attempts(file_id, attempts, last_attempt)
                   VALUES(?, 1, datetime('now'))
                   ON CONFLICT(file_id) DO UPDATE SET attempts = attempts + 1,
                     last_attempt = datetime('now')""", (file_id,))


def vote_image_sha(meeting_id: str, agenda_anchor: str) -> Optional[str]:
    conn = get_conn()
    row = conn.execute("SELECT image_sha FROM votes WHERE id = ?",
                       (f"{meeting_id}:{agenda_anchor}",)).fetchone()
    return row["image_sha"] if row else None


_BERLIN = ZoneInfo("Europe/Berlin")
# "15.30 bis 20 Uhr" / "16.30 bis 18.30 Uhr" — dot is the minute separator, end minutes optional.
_TIME_RE = re.compile(
    r"(\d{1,2})(?:[.:](\d{1,2}))?\s*(?:bis|-|–|—)\s*(\d{1,2})(?:[.:](\d{1,2}))?\s*Uhr",
    re.IGNORECASE,
)


def parse_time_window(time_str: Optional[str], day) -> Optional[tuple[datetime, datetime]]:
    """Parse a meeting time string into (start, end) naive datetimes on `day`.
    Returns None for NULL / unparseable / illogical (end<=start) strings."""
    if not time_str:
        return None
    m = _TIME_RE.search(time_str)
    if not m:
        return None
    h1, mi1 = int(m.group(1)), int(m.group(2) or 0)
    h2, mi2 = int(m.group(3)), int(m.group(4) or 0)
    if not (0 <= h1 <= 23 and 0 <= h2 <= 23 and 0 <= mi1 <= 59 and 0 <= mi2 <= 59):
        return None
    start = datetime(day.year, day.month, day.day, h1, mi1)
    end = datetime(day.year, day.month, day.day, h2, mi2)
    if end == start:
        return None  # degenerate / typo — no real window
    if end < start:
        end += timedelta(days=1)  # session runs past midnight (e.g. "22.00 bis 0.30 Uhr")
    return start, end


def live_meetings(now: Optional[datetime] = None) -> list[dict[str, Any]]:
    """Meetings whose parsed [start,end] window contains `now` (Europe/Berlin local).
    Returns full get_meeting() dicts, soonest-ending first. Empty if none."""
    if now is None:
        now = datetime.now(_BERLIN).replace(tzinfo=None)
    today = now.date()
    # Include yesterday so a session running past midnight ("22.00 bis 0.30 Uhr",
    # window extended by parse_time_window) stays live after 00:00.
    yesterday = today - timedelta(days=1)
    conn = get_conn()
    rows = conn.execute(
        "SELECT id, date, time FROM meetings WHERE date IN (?, ?) "
        "AND time IS NOT NULL AND time != ''",
        (today.isoformat(), yesterday.isoformat()),
    ).fetchall()
    live = []
    for r in rows:
        day = today if r["date"] == today.isoformat() else yesterday
        win = parse_time_window(r["time"], day)
        if win and win[0] <= now <= win[1]:
            live.append((win[1], r["id"]))
    live.sort(key=lambda t: t[0])
    return [get_meeting(mid) for _, mid in live]


def recent_documents(since: Optional[str] = None, limit: int = 50) -> list[dict[str, Any]]:
    conn = get_conn()
    where, params = [], []
    if since:
        where.append("f.downloaded_at >= ?"); params.append(since)
    where_sql = (" WHERE " + " AND ".join(where)) if where else ""
    rows = conn.execute(
        f"""SELECT f.id, f.label, f.doc_type, f.category, f.summary_de, f.summary_en,
                   f.topics, f.submitters, f.downloaded_at, f.agenda_anchor, m.body_name,
                   m.title AS meeting_title, m.date AS meeting_date,
                   ai.number AS agenda_number, ai.title AS agenda_title
            FROM files f
            LEFT JOIN meetings m ON m.id=f.meeting_id
            LEFT JOIN agenda_items ai
              ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor
            {where_sql} ORDER BY f.downloaded_at DESC, f.id DESC LIMIT ?""",
        [*params, limit],
    ).fetchall()
    return [_row_to_doc(r) for r in rows]


def facets() -> dict[str, Any]:
    """Distinct values for the UI filter controls."""
    conn = get_conn()
    committees = [dict(r) for r in conn.execute(
        "SELECT id, name FROM bodies ORDER BY name")]
    doc_types = [r["doc_type"] for r in conn.execute(
        "SELECT DISTINCT doc_type FROM files WHERE doc_type IS NOT NULL ORDER BY doc_type")]
    submitters = [r["value"] for r in conn.execute(
        """SELECT DISTINCT je.value AS value
           FROM files, json_each(files.submitters) je
           WHERE files.submitters IS NOT NULL AND files.submitters != '[]'
           ORDER BY je.value""")]
    return {"committees": committees, "doc_types": doc_types, "submitters": submitters}


def stats() -> dict[str, Any]:
    conn = get_conn()
    g = lambda q: conn.execute(q).fetchone()[0]
    return {
        "bodies": g("SELECT COUNT(*) FROM bodies"),
        "meetings": g("SELECT COUNT(*) FROM meetings"),
        "documents": g("SELECT COUNT(*) FROM files"),
        "with_text": g("SELECT COUNT(*) FROM files WHERE text_status='ok'"),
        "enriched": g("SELECT COUNT(*) FROM files WHERE enrich_status='ok'"),
        "votes": g("SELECT COUNT(*) FROM votes"),
        "votes_with_rollcall": g("SELECT COUNT(*) FROM votes WHERE members IS NOT NULL"),
        "last_scrape": get_meta("last_scrape"),
        "last_scrape_status": get_meta("last_scrape_status"),
    }


def recent_meeting_ids(since_date: str) -> list[str]:
    """Known meetings held on/after since_date (ISO) — for next-day re-checks."""
    conn = _connect()
    try:
        rows = conn.execute(
            "SELECT id FROM meetings WHERE date >= ? AND date <= date('now') ORDER BY date",
            (since_date,),
        ).fetchall()
        return [r["id"] for r in rows]
    finally:
        conn.close()


def get_meta(key: str) -> Optional[str]:
    conn = get_conn()
    row = conn.execute("SELECT value FROM meta WHERE key = ?", (key,)).fetchone()
    return row["value"] if row else None


def set_meta(conn: sqlite3.Connection, key: str, value: str) -> None:
    conn.execute(
        "INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
        (key, value),
    )


if __name__ == "__main__":
    init_db()
    print(f"Initialised {DB_PATH}")
