# Desinformationssystem

A daily archive of the **Karlsruhe Ratsinformationssystem** (the city-council meeting
calendar at `sitzungskalender.karlsruhe.de`). It scrapes every committee meeting and
its PDF documents, extracts the full text, **enriches** each document with an LLM
(German + English summaries, category, topics, entities), parses **vote tallies out of
the image-only result sheets** with a vision model, indexes everything in SQLite
(FTS5), and exposes it via a **Carbon Design System web UI** and an **MCP server**.

The name is a wink at *Ratsinformationssystem* (RIS → DIS).

Public instance: **https://dis.delphi.tools** · MCP endpoint: **https://dis.delphi.tools/mcp**

## Architecture

One Docker image (multi-stage: Node builds the Vite + React + `@carbon/react` SPA,
Python serves it), four roles sharing `data/` (SQLite DB + cached PDFs):

| Role | Container | Port | What |
|------|-----------|------|------|
| `scraper` | `desinfo-scraper` | — | daily crawl (run-at-start + 03:30), download, pdftotext, enrich |
| `web` | `desinfo-web` | 3650 | FastAPI JSON API + built Carbon SPA (one origin) |
| `mcp` | `desinfo-mcp` | 3651 | FastMCP Streamable HTTP at `/mcp` |
| `ticker` | `desinfo-ticker` | — | live-session watcher: polls the RIS live ticker, parses vote-tally images |

```
Gremien (committees) → Termine (meetings) → Tagesordnungspunkte (agenda items)
                                          → Dateien (the PDF documents)
                                          → Votes (per-TOP tallies + roll-call)
```

### Why HTML scraping (not OParl)

The site runs SOMACOS "Session", which advertises an OParl API at
`https://web1.karlsruhe.de/oparl/` — but only `system`/`bodies` respond and they
rate-limit aggressively, so the list/object endpoints are unusable. The HTML is rich
and stable (committee + öffentlich/nicht-öffentlich, agenda anchors `topN`, PDF `<a>`
links with descriptive labels), so we parse that. PDFs are pulled directly from the
`downloadfiles` URLs found in the HTML, sha256-deduped, throttled
(`REQUEST_INTERVAL`) and retried with backoff, because `web1` rate-limits bursts.

## Features

- **Full-text search** (SQLite FTS5 over label + extracted text + summaries + topics)
  with facet filters: Gremium, Dokumenttyp, Öffentlichkeit, date range, Einbringer.
- **LLM enrichment** per document via OpenRouter (`deepseek/deepseek-v4-pro` by
  default): summary_de/en, controlled-vocab category, topic tags, entities.
  Additive and retried — failed docs are re-attempted each run.
- **Submitter detection** (`submitter.py`, rule-based, no LLM): administrative doc
  types → SVK, Anträge/Anfragen → the fraction(s) parsed from the label/first lines
  (CDU, B90, SPD, … joint submissions supported). Shown as coloured chips, filterable.
- **Vote parsing**: the council publishes vote results only as *images* (live-ticker
  JPGs and image-only "Abstimmungsergebnis" PDFs). `vote_parse.py` sends them to a
  vision model (`VISION_MODEL`, default `google/gemini-3.1-flash-lite`) →
  `{ja, nein, enthaltung, members[]}` roll-call, stored per meeting + TOP. The
  official PDF is authoritative: a live re-parse never overwrites a PDF-sourced row.
- **Live sessions**: `GET /api/live` computes in-progress meetings (Europe/Berlin,
  handles past-midnight); the SPA polls it and shows a "Jetzt live" banner. The
  `ticker` role watches the RIS live ticker during sessions and ingests results
  near-real-time; `POST /api/meeting/{id}/refresh` pulls new files on demand.
- **Next-day results**: each run re-crawls meetings from the last `RECHECK_DAYS` to
  catch late-added minutes/results, and HEAD-compares existing files to re-download,
  re-extract and re-enrich in-place updates.
- **Committee rosters** parsed from the org pages (member, party, function, tenure),
  with party-composition breakdowns; upcoming meetings via `FORWARD_MONTHS`.
- **Web UI** (Carbon, dark): three tabs — Dokumente (dense results table with
  customisable, persisted columns), Sitzungen (incl. "Nur kommende"), Gremien —
  plus a shared meeting modal with the agenda as an accordion, per-TOP documents,
  vote tallies and deep-links back to the RIS.

## Files

- `db.py` — SQLite schema, FTS5, query helpers (shared by all roles)
- `scraper.py` — crawler + downloader + pdftotext/OCR extraction
  (`--months N`, `--backfill-all`, `--no-llm`, `--submitters`, `--votes`)
- `enrich.py` — OpenRouter enrichment (summaries, category, topics, entities)
- `submitter.py` — rule-based Einbringer detection
- `ticker.py` — live-ticker watcher (role `ticker`)
- `vote_parse.py` — vision-model vote-tally image parser
- `web.py` — FastAPI API + serves the built SPA from `web/dist`
- `web/` — Vite + React + TS SPA using `@carbon/react`
- `mcp_server.py` — MCP tools: `search_documents`, `get_document`,
  `list_committees`, `get_committee`, `list_meetings`, `get_meeting`,
  `search_votes`, `get_votes`, `recent_documents`, `stats`

## Deploy

```bash
cp .env.example .env      # set OPENROUTER_API_KEY (or ENABLE_LLM=0)
docker compose up -d --build
```

- Web UI: http://localhost:3650
- MCP: http://localhost:3651/mcp
- Full historical backfill: `docker exec desinfo-scraper python scraper.py --backfill-all`
- Backfill after logic changes: `... scraper.py --submitters` / `--votes`
- Logs: `docker logs -f desinfo-scraper`

All knobs are env vars — see `.env.example` (scrape window, politeness, schedule,
models, ticker cadence, OCR, FlareSolverr fallback).

## Dev (frontend)

```bash
cd web && npm install && npm run dev   # :5173, proxies /api to :3650
```

## Notes

- OCR for scanned PDFs is off by default (`ENABLE_OCR=0`); enabling it also needs
  `tesseract-ocr tesseract-ocr-deu ocrmypdf` added to the Dockerfile.
- Carbon's bundled IBM Plex webfonts aren't resolved by Vite (`~@ibm/plex` paths);
  the UI falls back to the system font stack. Cosmetic only.

## License

[0BSD](LICENSE)
