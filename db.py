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

-- Superseded document versions: when the RIS updates a PDF in place, the old
-- bytes are archived (data/pdfs/versions/) and recorded here.
CREATE TABLE IF NOT EXISTS file_versions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id       TEXT NOT NULL,
    sha256        TEXT,
    size          INTEGER,
    remote_modified TEXT,
    local_path    TEXT,
    downloaded_at TEXT,                    -- when this version was originally fetched
    superseded_at TEXT DEFAULT (datetime('now')),
    UNIQUE(file_id, sha256)
);
CREATE INDEX IF NOT EXISTS idx_fileversions_file ON file_versions(file_id);

-- Vorlagennummer index ("Vorlage: 2026/0324" in document headers/text).
-- own=1: the document's own number (header of a Vorlage/Antrag/…);
-- own=0: a mention (Tagesordnung, Niederschrift, …) — those mentions ARE the
-- lifecycle: they place the Vorlage in committee/council sessions over time.
CREATE TABLE IF NOT EXISTS file_vorlagen (
    file_id  TEXT NOT NULL,
    vorlage  TEXT NOT NULL,               -- "2026/0324"
    own      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (file_id, vorlage)
);
CREATE INDEX IF NOT EXISTS idx_vorlagen_nr ON file_vorlagen(vorlage);

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
                       ("fts_id", "INTEGER"), ("vorlage", "TEXT"),
                       # OParl ingester (oparl.py): the source's own document type
                       # + the full Vorlagen reference incl. amendment suffix
                       # ("2026/0365/3"; files.vorlage keeps the base form).
                       ("paper_type", "TEXT"), ("paper_reference", "TEXT")):
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
        # Match context via FTS5 snippet() (column -1 = best-matching column).
        # Delimiters are bound control chars, NOT markup — the UI splits on them,
        # so document text can never inject HTML.
        sql = f"""SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
                         m.title AS meeting_title, m.public AS meeting_public,
                        {agenda_cols}
                        snippet(files_fts, -1, ?, ?, ' … ', 20) AS snippet,
                        bm25(files_fts) AS rank
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
    sel_params = (["\x01", "\x02", *params] if query else params)
    rows = conn.execute(sql, [*sel_params, limit, offset]).fetchall()
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
    if not row:
        return None
    d = _row_to_doc(row)
    d["versions"] = [dict(r) for r in conn.execute(
        """SELECT id, sha256, size, remote_modified, downloaded_at, superseded_at
           FROM file_versions WHERE file_id = ? ORDER BY superseded_at DESC""", (file_id,))]
    d["vorlagen"] = [dict(r) for r in conn.execute(
        "SELECT vorlage, own FROM file_vorlagen WHERE file_id = ? ORDER BY own DESC, vorlage",
        (file_id,))]
    return d


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


# ---------------------------------------------------------------------------
# People (roster + roll-call matching)
#
# Roll-call panels carry surnames only ("Dr. Mentrup", "Schütz, K."), while the
# members table has full names with titles. Matching is surname-based, with the
# initial ("K.") and the meeting's own committee roster as disambiguators; an
# ambiguous surname without an initial is skipped rather than guessed.
# ---------------------------------------------------------------------------

_TITLE_RE = re.compile(r"^(dr|prof|dipl|ing|med|rer|nat|phil|jur|habil|h|c)[.\-]*$", re.I)


def _surname(full_name: str) -> str:
    toks = [t for t in re.split(r"\s+", (full_name or "").strip()) if t]
    core = [t for t in toks if not t.endswith(".") and not _TITLE_RE.match(t)]
    return (core[-1] if core else (toks[-1] if toks else "")).casefold()


def _first_initial(full_name: str) -> Optional[str]:
    toks = [t for t in re.split(r"\s+", (full_name or "").strip()) if t]
    core = [t for t in toks if not t.endswith(".") and not _TITLE_RE.match(t)]
    return core[0][0].casefold() if len(core) >= 2 else None


def _parse_roll_name(roll_name: str) -> tuple[str, Optional[str]]:
    """'Schütz, K.' -> ('schütz', 'k'); 'Dr.Huber' -> ('huber', None)."""
    rn = (roll_name or "").strip()
    initial = None
    if "," in rn:
        rn, ini = rn.split(",", 1)
        ini = ini.strip().rstrip(".")
        initial = ini[:1].casefold() if ini else None
    rn = re.sub(r"\b(?:dr|prof|dipl|ing|med|rer|nat|phil|jur|habil)\.\s*", "", rn, flags=re.I)
    toks = [t for t in re.split(r"\s+", rn.strip()) if t]
    return (toks[-1].casefold() if toks else ""), initial


def list_people() -> list[dict[str, Any]]:
    conn = get_conn()
    people: dict[str, dict] = {}
    for r in conn.execute(
            """SELECT mb.person_id, mb.name, mb.party, mb.party_code, mb.role,
                      b.id AS body_id, b.name AS body_name
               FROM members mb JOIN bodies b ON b.id = mb.body_id ORDER BY mb.name"""):
        p = people.setdefault(r["person_id"], {
            "person_id": r["person_id"], "name": r["name"],
            "party": r["party"], "party_code": r["party_code"], "memberships": []})
        if len(r["name"] or "") > len(p["name"] or ""):
            p["name"] = r["name"]
        p["memberships"].append({"body_id": r["body_id"], "body_name": r["body_name"],
                                 "role": r["role"]})
    return sorted(people.values(), key=lambda p: _surname(p["name"]))


def _surname_counts() -> dict[Optional[str], dict[str, int]]:
    """body_id -> surname -> number of members carrying it (for ambiguity checks)."""
    conn = get_conn()
    out: dict[Optional[str], dict[str, int]] = {}
    for r in conn.execute("SELECT body_id, name FROM members"):
        sn = _surname(r["name"])
        out.setdefault(r["body_id"], {})
        out[r["body_id"]][sn] = out[r["body_id"]].get(sn, 0) + 1
    return out


def person_votes(full_name: str) -> list[dict[str, Any]]:
    """Roll-call record for one person across all parsed votes, newest first."""
    conn = get_conn()
    target_sn = _surname(full_name)
    target_ini = _first_initial(full_name)
    if not target_sn:
        return []
    counts = _surname_counts()
    out = []
    for r in conn.execute(
            """SELECT v.meeting_id, v.agenda_anchor, v.top_label, v.result_text, v.members,
                      v.source, m.body_id, m.body_name, m.date AS meeting_date,
                      ai.title AS agenda_title, ai.number AS agenda_number
               FROM votes v JOIN meetings m ON m.id = v.meeting_id
               LEFT JOIN agenda_items ai
                 ON ai.meeting_id = v.meeting_id AND ai.anchor = v.agenda_anchor
               WHERE v.members IS NOT NULL
               ORDER BY m.date DESC"""):
        try:
            roll = json.loads(r["members"])
        except (ValueError, TypeError):
            continue
        body_counts = counts.get(r["body_id"], {})
        n_same = body_counts.get(target_sn) or sum(
            c.get(target_sn, 0) for c in counts.values())
        for e in roll:
            sn, ini = _parse_roll_name(e.get("name", ""))
            if sn != target_sn:
                continue
            if ini is not None and target_ini is not None and ini != target_ini:
                continue
            if ini is None and n_same > 1:
                continue  # ambiguous surname, no initial — don't attribute
            d = dict(r)
            d.pop("members")
            d["vote"] = e.get("vote")
            d["roll_name"] = e.get("name")
            out.append(d)
            break
    return out


def get_person(person_id: str) -> Optional[dict[str, Any]]:
    conn = get_conn()
    rows = conn.execute(
        """SELECT mb.*, b.name AS body_name FROM members mb
           JOIN bodies b ON b.id = mb.body_id
           WHERE mb.person_id = ? ORDER BY b.name""", (person_id,)).fetchall()
    if not rows:
        return None
    name = max((r["name"] or "" for r in rows), key=len)
    out = {
        "person_id": person_id,
        "name": name,
        "party": rows[0]["party"],
        "party_code": rows[0]["party_code"],
        "memberships": [{"body_id": r["body_id"], "body_name": r["body_name"],
                         "role": r["role"], "since": r["since"]} for r in rows],
    }
    votes = person_votes(name)
    out["votes"] = votes
    tally: dict[str, int] = {}
    for v in votes:
        tally[v["vote"]] = tally.get(v["vote"], 0) + 1
    out["vote_summary"] = tally
    return out


# ---------------------------------------------------------------------------
# Vorlagen lifecycle
# ---------------------------------------------------------------------------

def vorlage_chain(nr: str) -> Optional[dict[str, Any]]:
    """Timeline of a Vorlagennummer: every meeting where a document owns or
    mentions it, with the documents and any vote on the associated TOP."""
    conn = get_conn()
    rows = conn.execute(
        """SELECT fv.own, f.id, f.label, f.doc_type, f.meeting_id, f.agenda_anchor,
                  f.submitters, m.date AS meeting_date, m.body_name, m.body_id,
                  m.title AS meeting_title,
                  ai.number AS agenda_number, ai.title AS agenda_title
           FROM file_vorlagen fv
           JOIN files f ON f.id = fv.file_id
           LEFT JOIN meetings m ON m.id = f.meeting_id
           LEFT JOIN agenda_items ai
             ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor
           WHERE fv.vorlage = ?
           ORDER BY m.date, f.id""", (nr,)).fetchall()
    if not rows:
        return None
    stations: dict[str, dict] = {}
    own_docs = []
    for r in rows:
        d = _row_to_doc(r)
        if d.pop("own"):
            own_docs.append(d)
        mid = d.get("meeting_id") or "?"
        st = stations.setdefault(mid, {
            "meeting_id": d.get("meeting_id"),
            "date": d.get("meeting_date"),
            "body_name": d.get("body_name"),
            "body_id": d.get("body_id"),
            "meeting_title": d.get("meeting_title"),
            "documents": [], "votes": []})
        st["documents"].append(d)
    # attach votes on the TOPs those documents hang under
    for st in stations.values():
        anchors = {d["agenda_anchor"] for d in st["documents"] if d.get("agenda_anchor")}
        if not st["meeting_id"] or not anchors:
            continue
        ph = ",".join("?" * len(anchors))
        st["votes"] = [dict(v) for v in conn.execute(
            f"""SELECT agenda_anchor, top_label, result_text, ja, nein, enthaltung, source
                FROM votes WHERE meeting_id = ? AND agenda_anchor IN ({ph})""",
            (st["meeting_id"], *anchors))]
    timeline = sorted(stations.values(), key=lambda s: (s["date"] or "9999", s["meeting_id"] or ""))
    # Title: prefer the primary document (the Vorlage itself) over supplements.
    rank = {"Beschlussvorlage": 0, "Vorlage": 1, "Antrag": 2, "Anfrage": 3}
    own_docs.sort(key=lambda d: rank.get(d.get("doc_type"), 9))
    title = next((d["label"] for d in own_docs), None)
    return {"vorlage": nr, "title": title, "own_documents": own_docs, "stations": timeline}


# ---------------------------------------------------------------------------
# Voting analytics (party cohesion, agreement, dissent, attendance)
# ---------------------------------------------------------------------------

def _party_lookup() -> dict[Optional[str], dict[str, set]]:
    """body_id -> surname -> {(initial, party_code)} for roll-call attribution."""
    conn = get_conn()
    out: dict[Optional[str], dict[str, set]] = {}
    for r in conn.execute(
            "SELECT body_id, name, party_code FROM members WHERE COALESCE(party_code,'') != ''"):
        sn = _surname(r["name"])
        out.setdefault(r["body_id"], {}).setdefault(sn, set()).add(
            (_first_initial(r["name"]), r["party_code"]))
    return out


def _entry_party(lookup, body_id, roll_name) -> Optional[str]:
    sn, ini = _parse_roll_name(roll_name)
    cands = lookup.get(body_id, {}).get(sn)
    if not cands:  # fall back to any body (e.g. OB not on the committee roster)
        cands = set()
        for body in lookup.values():
            cands |= body.get(sn, set())
    if ini is not None:
        cands = {c for c in cands if c[0] is None or c[0] == ini} or cands
    parties = {p for _, p in cands}
    return parties.pop() if len(parties) == 1 else None


def vote_analytics() -> dict[str, Any]:
    """Aggregate statistics over all parsed votes. Party attribution comes from
    roll-call surname matching; unattributable entries are counted as 'unknown'
    and excluded from per-party numbers rather than guessed."""
    conn = get_conn()
    lookup = _party_lookup()
    votes = conn.execute(
        """SELECT v.id, v.ja, v.nein, v.enthaltung, v.members, v.source,
                  m.body_id, m.body_name, m.date
           FROM votes v JOIN meetings m ON m.id = v.meeting_id""").fetchall()

    totals = {"votes": len(votes), "with_rollcall": 0, "unanimous": 0, "contested": 0}
    party: dict[str, dict] = {}   # code -> aggregates
    pair_agree: dict[tuple, list] = {}
    dissent: dict[str, dict] = {}  # roll name -> {party, count, total}
    unknown_entries = 0

    for v in votes:
        ja, nein, enth = v["ja"] or 0, v["nein"] or 0, v["enthaltung"] or 0
        if ja + nein + enth > 0:
            if nein == 0 and enth == 0:
                totals["unanimous"] += 1
            if nein > 0:
                totals["contested"] += 1
        if not v["members"]:
            continue
        try:
            roll = json.loads(v["members"])
        except (ValueError, TypeError):
            continue
        totals["with_rollcall"] += 1
        # per-party counts for THIS vote
        pv: dict[str, dict[str, int]] = {}
        attributed = []
        for e in roll:
            code = _entry_party(lookup, v["body_id"], e.get("name", ""))
            if not code:
                unknown_entries += 1
                continue
            w = e.get("vote") or "abwesend"
            pv.setdefault(code, {"ja": 0, "nein": 0, "enthaltung": 0, "abwesend": 0})
            pv[code][w] = pv[code].get(w, 0) + 1
            attributed.append((e.get("name", ""), code, w))
        # party line = majority among cast votes (ja/nein/enthaltung)
        lines: dict[str, str] = {}
        for code, c in pv.items():
            cast = {k: c[k] for k in ("ja", "nein", "enthaltung")}
            n_cast = sum(cast.values())
            if n_cast == 0:
                continue
            line = max(cast, key=lambda k: cast[k])
            lines[code] = line
            agg = party.setdefault(code, {"code": code, "votes": 0, "members_cast": 0,
                                          "with_line": 0, "ja": 0, "nein": 0,
                                          "enthaltung": 0, "abwesend": 0})
            agg["votes"] += 1
            agg["members_cast"] += n_cast
            agg["with_line"] += cast[line]
            for k in ("ja", "nein", "enthaltung", "abwesend"):
                agg[k] += c[k]
        for a, b in [(a, b) for a in lines for b in lines if a < b]:
            pair_agree.setdefault((a, b), []).append(lines[a] == lines[b])
        for name, code, w in attributed:
            if w in ("ja", "nein", "enthaltung") and code in lines:
                d = dissent.setdefault(name, {"name": name, "party": code,
                                              "dissents": 0, "votes": 0})
                d["votes"] += 1
                if w != lines[code]:
                    d["dissents"] += 1

    parties = []
    for agg in sorted(party.values(), key=lambda a: -a["votes"]):
        cast = agg["members_cast"]
        parties.append({**agg, "cohesion": round(agg["with_line"] / cast, 3) if cast else None,
                        "attendance": round(1 - agg["abwesend"] /
                                            (cast + agg["abwesend"]), 3)
                                      if cast + agg["abwesend"] else None})
    agreement = [{"a": a, "b": b, "agree": round(sum(x) / len(x), 3), "n": len(x)}
                 for (a, b), x in sorted(pair_agree.items())]
    dissenters = sorted((d for d in dissent.values() if d["dissents"] > 0),
                        key=lambda d: (-d["dissents"], -d["votes"]))[:20]
    return {"totals": totals, "parties": parties, "agreement": agreement,
            "dissenters": dissenters, "unattributed_entries": unknown_entries}


def add_file_version(conn: sqlite3.Connection, file_id: str, sha256: Optional[str],
                     size: Optional[int], remote_modified: Optional[str],
                     local_path: str, downloaded_at: Optional[str]) -> None:
    conn.execute(
        """INSERT OR IGNORE INTO file_versions
           (file_id, sha256, size, remote_modified, local_path, downloaded_at)
           VALUES(?,?,?,?,?,?)""",
        (file_id, sha256, size, remote_modified, local_path, downloaded_at))


def get_file_version(version_id: int) -> Optional[dict[str, Any]]:
    row = get_conn().execute("SELECT * FROM file_versions WHERE id = ?",
                             (version_id,)).fetchone()
    return dict(row) if row else None


def set_file_vorlagen(conn: sqlite3.Connection, file_id: str,
                      own: Optional[str], refs: Iterable[str]) -> None:
    conn.execute("DELETE FROM file_vorlagen WHERE file_id = ?", (file_id,))
    for nr in set(refs) | ({own} if own else set()):
        conn.execute(
            "INSERT OR REPLACE INTO file_vorlagen(file_id, vorlage, own) VALUES(?,?,?)",
            (file_id, nr, 1 if nr == own else 0))
    conn.execute("UPDATE files SET vorlage = ? WHERE id = ?", (own, file_id))


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


def recent_documents(
    since: Optional[str] = None,
    limit: int = 50,
    committee: Optional[str] = None,
    doc_type: Optional[str] = None,
    submitter: Optional[str] = None,
    topic: Optional[str] = None,
) -> list[dict[str, Any]]:
    conn = get_conn()
    where, params = [], []
    if since:
        where.append("f.downloaded_at >= ?"); params.append(since)
    if committee:
        where.append("(m.body_name = ? OR m.body_id = ?)"); params.extend([committee, committee])
    if doc_type:
        where.append("f.doc_type = ?"); params.append(doc_type)
    if submitter:
        where.append("f.submitters LIKE ?"); params.append(f'%"{submitter}"%')
    if topic:
        where.append("f.topics LIKE ?"); params.append(f'%"{topic}"%')
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
