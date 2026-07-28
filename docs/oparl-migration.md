# OParl ingestion — findings & migration plan

**Status:** the Karlsruhe OParl API is fully functional. The long-standing note that
it is "incomplete/unreliable, so we scrape the HTML" is **wrong** — it was caused by
probing the wrong base path. This document records what was verified live (2026-07-24)
and specifies a hybrid ingester that replaces HTML scraping with OParl while keeping the
enrichment/vote/search layers.

---

## 1. What was actually wrong

Two compounding issues made the API *look* dead:

1. **Wrong base path.** The API is served at **`/ris/oparl/…`**, not `/oparl/…`.
   `https://web1.karlsruhe.de/oparl/system` → 404.
   `https://web1.karlsruhe.de/ris/oparl/system` → 200 JSON.

2. **Self-links point at the broken prefix.** Every object reports its own `id` and all
   child links under `https://web1.karlsruhe.de/oparl/…` (no `/ris`). So even after you
   find the working `system` endpoint, *following the links it gives you* 404s. You must
   rewrite **`/oparl/` → `/ris/oparl/`** on every URL before fetching.

   ```python
   def fix(url: str) -> str:
       return url.replace("https://web1.karlsruhe.de/oparl/",
                          "https://web1.karlsruhe.de/ris/oparl/")
   ```

3. **Rate limiting did not reproduce.** 12 rapid back-to-back requests → twelve 200s, no
   throttle. A polite ~1s spacing is plenty; keep `REQUEST_INTERVAL` conservative anyway.

Browser-like User-Agent was used but does not appear to be required (the wrong path 404s
under any UA; the right path works under any UA).

---

## 2. Verified endpoints (base = `https://web1.karlsruhe.de/ris/oparl`)

| Endpoint | Path (append to base) | Notes |
|---|---|---|
| System | `/system` | vendor SOMACOS Session v1.6.1, OParl 1.1 |
| Bodies | `/bodies` | one body: `0001` |
| Papers | `/bodies/0001/papers` | Drucksachen/Vorlagen |
| Meetings | `/bodies/0001/meetings` | Sitzungen |
| Organizations | `/bodies/0001/organizations` | Gremien |
| People | `/bodies/0001/people` | Personen |
| Memberships | `/bodies/0001/memberships` | person↔organization |
| Files | `/bodies/0001/files` | PDF metadata + accessUrl |
| Consultations | `/bodies/0001/consultations` | **the lifecycle** (see below) |
| Agenda items | `/bodies/0001/agendaitems` | TOPs |
| Locations | `/bodies/0001/locations` | |
| Legislative terms | `/bodies/0001/legislativeterms` | |

**Pagination:** `?page=N`, `elementsPerPage=10`. `links.next` is present until the last
page (remember to prefix-rewrite it).

**Incremental sync:** **`?modified_since=<ISO8601>`** works and is the key to an efficient
daily job — fetch only changed objects instead of re-crawling. Example verified:
`?modified_since=2026-07-20T00:00:00+02:00` returned only papers modified on/after that
date. (URL-encode the `+` as `%2B`.) There is a matching `created_since` per the OParl 1.1
spec — confirm before relying on it.

Contact for the data source (from the `system` object):
**Wendelin Bastian, wendelin.bastian@it.karlsruhe.de** — the city's RIS admin. Good escalation
path if a field ever changes, and worth a courtesy heads-up that a city employee is building
on the open interface.

---

## 3. Object shapes that matter

### Paper (`type` .../Paper)
Keys observed: `id, type, body, name, reference, date, paperType, subordinatedPaper,
auxiliaryFile, underDirectionOf, consultation, created, modified`.

- **`reference`** = the Vorlagennummer directly, e.g. `"2026/0365/3"` — no regex needed.
  (Note the trailing `/3` suffix on amendments; our current `file_vorlagen` keys on
  `YYYY/NNNN` — decide whether to store the full reference or the base number, and keep
  both if useful for cross-linking amendments.)
- **`paperType`** = document type already classified by the source (e.g.
  `"Änderungs-/Ergänzungsantrag"`). Map these onto our `doc_type` vocabulary; keep the raw
  value too.
- **`mainFile`** / `auxiliaryFile` = File objects (or refs) carrying `accessUrl` /
  `downloadUrl` + `fileName`. These are the PDFs.
- **`consultation[]`** = **the Vorlagen lifecycle, pre-structured.** Each entry links
  `paper → agendaItem → meeting → organization[]`. This is exactly the committee→council
  timeline we currently reconstruct by mining fulltext for Vorlagennummer mentions. With
  OParl it is authoritative and free.
- **`subordinatedPaper`** = links an amendment to its parent Vorlage → the many-to-many
  document relationship we deferred as scope creep is native here.
- **`underDirectionOf`** = submitting body (federführend) → cross-check against our
  rule-based `submitters` (SVK/fraction) derivation.

### Consultation (`type` .../Consultation)
Links `paper`, `agendaItem`, `meeting`, `organization[]`. Iterating consultations (or the
`consultation[]` embedded in papers) gives every station of every Vorlage.

### Files
`accessUrl` is the PDF download URL (same web1 host as our current `downloadfiles/*.pdf`
grabs, which already work — PDF fetching is proven). sha/size still computed on our side.

---

## 4. Migration plan — hybrid ingester

**Goal:** replace HTML *discovery/structure* with OParl; keep everything downstream. The
derived layer (LLM enrichment, vote-image parsing, party attribution, FTS) is unchanged
and stays keyed on the same DB schema.

### Keep unchanged
- `db.py` schema, FTS, `file_versions`, `file_vorlagen`, `votes`, analytics, people.
- `enrich.py`, `vote_parse.py`, `ticker.py` (OParl carries no vote-tally images — the
  live ticker + result-PDF vision parsing stays exactly as is).
- `web.py`, `mcp_server.py`, the SPA — they read the DB, not the source.

### Build: `oparl.py` — a new ingester feeding the existing tables
1. **Client** with the prefix-rewrite baked in, throttle + retry mirroring
   `scraper.Fetcher` (reuse it if clean). Every fetched URL runs through `fix()`.
2. **Walkers**: `iter_papers(modified_since=…)`, `iter_meetings(…)`,
   `iter_organizations()`, `iter_people()`, following `links.next` (rewritten) to the end.
3. **Mappers** OParl object → our rows:
   - organization → `bodies` (+ `members` from memberships/people).
   - meeting → `meetings` (date/time/location/public); agendaItem → `agenda_items`
     (anchor scheme must match what the web UI deep-links expect — check `App.tsx`
     `RIS_BASE` links and the existing `topN`/`topN.M` anchors).
   - paper.mainFile/auxiliaryFile → `files` (download via `accessUrl`, sha-dedupe,
     `pdftotext`, **version-archive on change** exactly as `scraper._process_file` does —
     factor that helper out so both ingesters share it).
   - paper.reference → `files.vorlage` + `file_vorlagen(own=1)`; every consultation's paper
     reference that shows up on a meeting → `file_vorlagen(own=0)` station rows. **Prefer
     OParl consultations over fulltext mention-mining** for the lifecycle.
   - paperType → `doc_type` (mapping table; keep raw).
4. **Incremental**: store last-successful `modified_since` watermark in `meta`; daily run
   pulls only changes. Full backfill = no watermark (walk everything).
5. **CLI + entrypoint**: `python oparl.py [--full] [--since ISO]`. Add `INGEST=oparl|html`
   env switch in `entrypoint.sh` (default **keep html** until OParl is validated, then flip).

### Implementation status (2026-07-24)

**Built.** `oparl.py` implements the plan above:
- `Client` wraps the shared `filestore.Fetcher` (same throttle/retry/UA), rewrites
  the `/oparl/` → `/ris/oparl/` prefix on every followed URL, walks `links.next`.
- The download → sha-dedupe → pdftotext → version-archive → vorlage-mining pipeline
  was factored out of scraper.py into **`filestore.py`** (`process_file()` + `Fetcher`
  + `classify` + `extract_vorlagen` + `extract_text`); scraper.py re-exports the old
  names so ticker/web/tests are untouched. Both ingesters call the same code.
- Meetings walk (`modified_since` watermark in meta `oparl_since`; `--full` /
  `--since` override) → `meetings` + `agenda_items` (+ meeting-level and per-TOP
  files); recently-held meetings (RECHECK_DAYS) are re-fetched by id every run even
  when outside the modified window, in case the vendor doesn't bump
  `meeting.modified` for late-added protocol files.
- Papers walk → paper PDFs attached at the paper's latest consultation station
  (`ensure_meeting()` fetches + ingests meeting rows on demand for stations outside
  the window); `paperType`/`reference` stamped into `files.paper_type` /
  `files.paper_reference`; base reference → `files.vorlage` + `file_vorlagen(own=1)`;
  every consultation station adds `file_vorlagen(own=0)` on the meeting's
  Tagesordnung/Einladung carrier file (authoritative lifecycle on top of the shared
  pipeline's fulltext mention-mining).
- Committee list + member rosters stay HTML (`scraper.sync_committees_members`) —
  OParl has no party data (see above).
- Enrichment + vote-PDF parsing run at the end exactly like scrape() does.
- **Watermark hold-back** (added 2026-07-28): per-item failures are swallowed so
  one bad object can't abort the run, but each one records the object's
  `modified` stamp, and the run then advances `oparl_since` only as far as the
  *oldest* failure (`counts["held_watermark"]` reports it). Advancing past a
  swallowed error would drop that object permanently — the vendor never bumps
  `modified` again, and only meetings have the date-based recheck net.
- `entrypoint.sh` gained `INGEST=html|oparl` (**default html** — flip only after
  validation); Dockerfile copies the new modules. db._migrate adds
  `files.paper_type` / `files.paper_reference` idempotently.

### Validate before cutover (do NOT delete the scraper)
- Run OParl ingest into a **copy** of the DB; diff counts (meetings/papers/files) and
  spot-check Vorlagen chains vs the current HTML-derived ones.
- ~~Confirm a stable mapping between OParl meeting id and our `termin-` id is required~~
  — **superseded**: the id spaces are identical, so no crosswalk exists or is needed
  (`oparl.py` builds `termin-{N}` directly). See "Open questions — RESOLVED" below;
  re-verified against the live API 2026-07-28 (meeting 10802, agenda `14.1.1` →
  `top14.1.1` vs the HTML's `dtop14.1.1`, file stem `00677201`, all 27 `gr` org ids).
  There is no runtime assertion on this, so it stays worth a spot-check after any
  vendor upgrade.
- Keep `scraper.py` as a working fallback behind the `INGEST` switch indefinitely.

### Open questions — RESOLVED (verified live 2026-07-24, during the oparl.py build)

- **Meeting identity: SOLVED — the numeric ids are the SAME id space.**
  OParl `/bodies/0001/meetings/N` **is** the HTML `termin-N`. No crosswalk needed.
  Verified on 8 random meetings from the production DB: 7/8 matched exactly on
  date+title (e.g. termin-10802 = Ortschaftsrat Wolfartsweier 2026-04-14 on both
  sides); the 8th (termin-10749) is `deleted: true` in OParl **and** 404 on the HTML
  site — a cancelled meeting our HTML DB retains as a stale row (OParl actually
  surfaces deletions, which the HTML scraper never could). Same pattern holds for the
  other id spaces (structural, not coincidental — the HTML site is a frontend over the
  same Session backend):
  - organizations `/organizations/gr/N` == `organisation-gr-N` (3/3 name-exact),
  - people `/people/N` == `people-N` (spot-checked Mentrup=449, matches members table),
  - files: OParl `fileName` "00677479.pdf" stem == our `files.id` "00677479", and the
    prefix-rewritten `downloadUrl` is **byte-identical** to the URL already stored in
    `files.url` (the HTML pages link into `/ris/oparl/bodies/0001/downloadfiles/…`).
  So votes (`termin-N:topX`), web deep-links, and file dedupe all survive unchanged.
- **Agenda anchors: SOLVED.** OParl `agendaItem.number` ("3", "12.1", "14.1.1") maps to
  our anchor as `top{number}`; the production DB satisfies `anchor == 'top'||number` on
  100% of 332 agenda_items rows, and dotted sub-items appear identically on both sides.
  Meeting objects also carry real `start`/`end` times (e.g. 15:30–20:00) from which
  oparl.py rebuilds the RIS-style `"15.30 bis 20 Uhr"` string `db.parse_time_window`
  (the live banner) expects.
- **paperType → doc_type:** enumerated from the 4000 most-recently-modified papers
  (modified 2025-03…2026-07): Beschlussvorlage 1614, Antrag 634, Informationsvorlage
  540, Anfrage 472, Änderungs-/Ergänzungsantrag 208, `Haushalt THH nnnn`/`Haushalt
  Gesamt` ~520 (budget motions, reference format `DHH/YYYY/NNNN`), Offenlage 9,
  (none — deleted stubs) 3. Mapping lives in `oparl.py PAPER_TYPE_MAP` /
  `map_paper_type()` (Haushalt* → Antrag by prefix); used only as a fallback when
  `classify(label)` yields Sonstiges, and the raw value is kept in `files.paper_type`.
- **Reference format:** both. `files.vorlage` + `file_vorlagen` keep the base
  `2026/0365` (what the existing chains key on); the full `2026/0365/3` is stored in
  the new `files.paper_reference` column (with `files.paper_type` for the raw type).
- **NEW finding — no party data in OParl.** Person objects carry no party and
  memberships only reference committees (`gr/…`) — there are no Fraktion organizations
  reachable from people, and `underDirectionOf` is the responsible *Amt* (e.g. "Team
  Sauberes Karlsruhe"), not the submitting fraction. The members table's `party` /
  `party_code` columns feed vote roll-call attribution and the analytics layer, so the
  committee list + member rosters **stay HTML-scraped** even under `INGEST=oparl`
  (`scraper.sync_committees_members`, ~28 requests/run). This is the one deliberate
  hybrid piece.
- **NEW quirk — `elementsPerPage` is fixed at 10**; the server ignores requests for a
  larger page size. Full-archive walks are therefore slow (~1 page/s polite): plan
  tens of minutes for a full paper backfill.
- **NEW quirk — deleted objects**: cancelled meetings/papers stay in the lists as
  `deleted: true` stubs with empty name/start. oparl.py skips them (counted as
  `deleted_meetings`/`deleted_papers`) and never deletes existing rows.

---

## 5. Quick reference — reproduce the probe

```bash
BASE=https://web1.karlsruhe.de/ris/oparl
curl -s "$BASE/system" | python3 -m json.tool
curl -s "$BASE/bodies/0001/papers" | python3 -m json.tool | head
curl -s "$BASE/bodies/0001/papers?modified_since=2026-07-20T00:00:00%2B02:00"
# follow next page — remember the prefix rewrite:
#   links.next = https://web1.karlsruhe.de/oparl/...  ->  .../ris/oparl/...
```
