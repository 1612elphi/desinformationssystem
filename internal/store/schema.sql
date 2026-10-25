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
    geo_status    TEXT DEFAULT 'pending',  -- pending | ok | error  (geo.py street/district tagging)
    district      TEXT,                    -- primary Karlsruhe Stadtteil (geo.py), NULL = city-wide
    district_conf REAL,                    -- confidence of the district classification
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
    members_ok    INTEGER,                  -- 1/0: roll-call ja/nein/enth tally equals the counters; NULL = no roll-call
    source        TEXT,                     -- 'live' | 'pdf'
    image_url     TEXT,
    image_sha     TEXT,
    file_id       TEXT,                     -- source PDF file id (pdf source)
    parsed_at     TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_votes_meeting ON votes(meeting_id);

-- Debate transcripts from council VODs (vod_cleanup.py): one row per speech, whisper text
-- plus the Sonnet readability pass. Speaker comes from the XPanel's green tile border.
CREATE TABLE IF NOT EXISTS speeches (
    id            TEXT PRIMARY KEY,        -- meeting_id:video_id:start second
    meeting_id    TEXT REFERENCES meetings(id),
    agenda_anchor TEXT,                     -- topN; NULL for opening/closing remarks
    top_label     TEXT,                     -- label as shown on the panel
    speaker       TEXT,                     -- name as on the seat map; NULL if unknown
    party         TEXT,
    video_id      TEXT,
    start_s       REAL,                     -- offset into the VOD
    end_s         REAL,
    text_raw      TEXT,                     -- whisper output
    text          TEXT,                     -- cleaned for reading
    created_at    TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_speeches_meeting ON speeches(meeting_id, start_s);

-- Jev's reading of each speech (speech_analysis.judge): {question: {choice: probability}}
-- for stance, tone, sentiment and to_<party> attitude. Keyed by speeches.id.
CREATE TABLE IF NOT EXISTS speech_judgements (
    speech_id  TEXT PRIMARY KEY,
    text_sha   TEXT,                        -- sha1 of the judged text; a re-cleaned speech is judged again
    answers    TEXT,
    judged_at  TEXT DEFAULT (datetime('now'))
);

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

-- Street/place mentions matched in a file's text and confirmed by the geo filter
-- (geo.py). name is the gazetteer street name; matching only, no geometry stored.
CREATE TABLE IF NOT EXISTS file_streets (
    file_id  TEXT NOT NULL,
    name     TEXT NOT NULL,
    PRIMARY KEY (file_id, name)
);
CREATE INDEX IF NOT EXISTS idx_file_streets_name ON file_streets(name);

-- Full-text search over label + extracted text + summaries + topics.
-- Regular (not contentless) FTS5 so we can DELETE+reINSERT on re-enrichment;
-- rows are keyed on files.fts_id, a stable integer we assign ourselves
-- (the implicit rowid of a TEXT-PK table may be renumbered by VACUUM).
CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
    label, fulltext, summary_de, summary_en, topics, category,
    body_name, meeting_title
);
