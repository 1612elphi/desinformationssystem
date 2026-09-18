# Notes from riski

riski (https://codeberg.org/digital-codes/riski, OK Lab Karlsruhe / digital-codes) is a
second project over the same data: the Karlsruhe OParl API at
`https://web1.karlsruhe.de/ris/oparl`. It is a set of hand-run Python scripts on
Postgres + pgvector, plus a Vue demo with static sample data. No service, no scheduler.

Paths below are relative to a riski checkout (reference copy: `~/GitRepos/riski`, commit 2cacb1e).

DIS already covers, often further than riski: incremental OParl sync, OCR/text extraction,
LLM summaries and tags, vote parsing, committee rosters, Vorlage lifecycle (`db.vorlage_chain`).
The sections below are the parts DIS does not have.

## 1. Street and district references (geo)

DIS has no geo data. riski links agenda items to Karlsruhe streets and Stadtteile.

### Data

| File | Content |
|---|---|
| `references/geo/stadtteile.geojson` | 27 Stadtteile, properties `NUMMER`, `NAME` |
| `references/geo/karslruhe.geojson` | city boundary (filename typo is upstream) |
| `references/geo/strassen-ka-orig.md` | Liegenschaftsamt street list, 1768 names, with year of naming, former names and the person or place each is named after |
| `references/geo/all_found_streets.geojson` | OSM geometries for the streets that matched |
| `references/geo/streetMappingErrors.md` | 71 streets with no OSM match (typos in the source list, e.g. "Berhard-Metz-Straße", and squares/Anlagen) |

Geometries come from the Geofabrik extract `karlsruhe-regbez-latest.osm.pbf`, clipped to the
city boundary (`utils/streetGeometriesLocal.py`, needs `osmium` + `shapely`). The older
`utils/streetGeometries.py` uses Nominatim + Overpass at 1.5 s per street; the PBF version is
39.5 s for all 1768.

License check before import: OSM geometries are ODbL. The street list and Stadtteil GeoJSON
source licenses are not stated in the riski repo (unverified).

### Matching

`src/local_geo_matching.py`, 113 lines, stdlib only. Port it as-is.

- One regex for all names: alternation sorted longest-first, bounded by `(?<!\w)` / `(?!\w)`.
  One scan per text instead of one regex per street.
- Overlapping hits: the longest span wins. `Weststadt` does not match inside `Nordweststadt`.
- Case-insensitive, literal names only. `Kaiserstr.` and other abbreviations do not match.

riski runs it on AgendaItem names only (`src/pgAddStreetRefs.py`, `src/pgAddDistrictRefs.py`),
writing link tables `Streets_items_AgendaItems` / `Districts_items_AgendaItems`.

### For DIS

- Tables `streets(id, name, geojson)`, `districts(id, nummer, name, geojson)`,
  `file_streets(file_id, street_id)`, `file_districts(file_id, district_id)`.
- Match on `label` + agenda item title first. Full text produces more false positives: generic
  names ("Am Wald"-type) and street names in letterheads and address blocks (unverified which
  addresses repeat; count top matches over full text first). Keep a stoplist for those.
- Derive district from street by point-in-polygon when the text names only a street.
- Facet `district` in `search_documents` and the Dokumente tab; a map view is optional.

## 2. Semantic search with LLM rerank

`src/pgTimeline.py`, `src/pgVecSearch.py`, `src/pgNameVecs.py`.

1. Embed the query (OpenAI-compatible endpoint, 1024 dimensions, unit-normalised).
2. pgvector cosine distance (`<=>`), top 100.
3. Drop anything past distance threshold 0.40.
4. For each remaining file, ask a small LLM for a 0..1 relevance score (temperature 0.1);
   keep >= 0.8. riski uses `granite-4.0-350m` locally.
5. Collect all OParl objects linked to the kept files, sort by date, group into
   30-day windows. Output shape: `app/src/assets/sampleData.json`.

Step 5 is the part DIS lacks: `get_vorlage` follows one Vorlagennummer, riski builds one
timeline from a free-text question across several Vorlagen.

### For DIS

- `sqlite-vec` keeps it in SQLite. Embed `summary_de` + label, not full text: DIS has
  summaries for 17257 of 47516 docs, and full text makes the embeddings noisy.
- Combine with FTS5 (reciprocal rank fusion) instead of replacing it. FTS5 finds Vorlagennummern and
  names exactly; embeddings do not.
- riski reranks one HTTP call per file with the full content. For DIS, send summaries in
  one batched call, or drop rerank and keep the distance threshold.
- New MCP tool `topic_timeline(query)`, reusing the `vorlage_chain` station format.

## 3. Topic tag normalisation

DIS topics are free-form LLM output, so near-duplicates drift apart in the facet list
(for example "Radverkehr" / "Radwege" / "Fahrradverkehr"; unverified for DIS, check with
`SELECT topic, count(*) ... GROUP BY topic`).

riski: `src2/pass1_entities_tfidf.py` (TF-IDF over teasers, NLTK stopwords) then
`src2/pass2_cluster_words.py` (rapidfuzz, similarity threshold 85, merges clusters, keeps the
highest-TF-IDF word as representative). Output is a JSON cluster map.

For DIS: run the clustering offline over `topics`, review the map by hand, store it as
`topic_alias(alias, canonical)`, apply at query time. rapidfuzz catches spelling variants only;
synonyms like "Radwege"/"Fahrradverkehr" need embeddings or a manual list.

## 4. Vote parsing cross-check

riski `utils/voting/` does the same job as `vote_parse.py` with OpenCV + Tesseract.
Worse approach, two useful facts:

- Its colour map is yellow=Ja, red=Nein, **blue=Enthaltung**, grey=absent.
  The `vote_parse.py` prompt says `GRAU = Enthaltung/abwesend`. If blue is right, DIS merges
  abstentions into absences in roll-call rows. The riski sample (`utils/voting/abstimmung.png`,
  TOP 12, 28 Jul 2026) has 0 Enthaltungen, so it does not settle it. Verify with any stored
  tally image where `enthaltung > 0`.
- Consistency check: count of member tiles per vote must equal the donut counters
  (sample: 44 yellow, 4 red, 1 grey). DIS does not compare `members[]` against
  `ja/nein/enthaltung`. Store a mismatch flag and re-parse on mismatch.

## 5. Orphan files

`src/pgFileQuery.py:find_unreferenced_files` lists files linked to no agenda item, meeting
or paper. Same query against DIS as a data-quality check after each sync.

## Not worth porting

- Teasers (`src2/pgTeasers.py`): 3-5 sentence summaries, DIS `summary_de` covers it.
- Mistral OCR (`src/pgOcr.py`): only if `ENABLE_OCR` tesseract quality is too low.
- Mandate scraper (`utils/mandate/getMandates.py`): DIS has rosters.
- Knowledge graph (`utils/kg/`): notes and schema drafts, no working pipeline.
- Postgres per-entity tables (`src/pgGen.py`): DIS schema is simpler.
