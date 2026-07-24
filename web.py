"""
Desinformationssystem — FastAPI JSON API + static Carbon SPA.

One origin (no CORS), print-relay style: API routes are declared first, then the
built Vite/React SPA in web/dist is mounted at "/" with html=True so client-side
routes fall back to index.html.
"""
from __future__ import annotations

import asyncio
import email.utils
import os
import re
import sqlite3
import time
from datetime import date, datetime, timedelta, timezone
from typing import Optional
from xml.sax.saxutils import escape as xml_escape

from fastapi import FastAPI, HTTPException, Query, Request, Response
from fastapi.concurrency import run_in_threadpool
from fastapi.responses import FileResponse, JSONResponse, PlainTextResponse
from fastapi.staticfiles import StaticFiles

import db
import scraper

app = FastAPI(title="Desinformationssystem", docs_url="/api/docs", openapi_url="/api/openapi.json")

DIST = os.path.join(os.path.dirname(__file__), "web", "dist")


@app.on_event("startup")
def _startup() -> None:
    db.init_db()


@app.get("/health")
def health() -> dict:
    return {"ok": True}


@app.get("/api/stats")
def api_stats() -> dict:
    return db.stats()


@app.get("/api/facets")
def api_facets() -> dict:
    return db.facets()


@app.get("/api/search")
def api_search(
    q: Optional[str] = None,
    committee: Optional[str] = None,
    doc_type: Optional[str] = Query(None, alias="type"),
    date_from: Optional[str] = Query(None, alias="from"),
    date_to: Optional[str] = Query(None, alias="to"),
    public: Optional[str] = None,
    topic: Optional[str] = None,
    submitter: Optional[str] = None,
    limit: int = Query(50, ge=1, le=200),
    offset: int = Query(0, ge=0),
) -> dict:
    pub = None if public in (None, "", "all") else (public in ("1", "true", "public", "öffentlich"))
    try:
        return db.search_documents(
            query=q, committee=committee, doc_type=doc_type, date_from=date_from,
            date_to=date_to, public=pub, topic=topic, submitter=submitter,
            limit=limit, offset=offset)
    except sqlite3.OperationalError:
        # bad FTS syntax and the like -> client error; anything else propagates
        # as a real 500 instead of leaking server internals in a fake 400
        raise HTTPException(status_code=400, detail="invalid search query")


@app.get("/api/document/{file_id}")
def api_document(file_id: str) -> dict:
    doc = db.get_document(file_id)
    if not doc:
        raise HTTPException(status_code=404, detail="not found")
    return doc


@app.get("/api/committees")
def api_committees() -> list[dict]:
    return db.list_committees()


@app.get("/api/meetings")
def api_meetings(
    response: Response,
    committee: Optional[str] = None,
    date_from: Optional[str] = Query(None, alias="from"),
    date_to: Optional[str] = Query(None, alias="to"),
    upcoming: bool = False,
    order: Optional[str] = None,
    limit: int = Query(100, ge=1, le=500),
    offset: int = Query(0, ge=0),
) -> list[dict]:
    if upcoming:
        date_from = date_from or date.today().isoformat()
        order = order or "asc"
    response.headers["X-Total-Count"] = str(
        db.count_meetings(committee=committee, date_from=date_from, date_to=date_to))
    return db.list_meetings(committee=committee, date_from=date_from, date_to=date_to,
                            order=order or "desc", limit=limit, offset=offset)


@app.get("/api/meeting/{meeting_id}")
def api_meeting(meeting_id: str) -> dict:
    m = db.get_meeting(meeting_id)
    if not m:
        raise HTTPException(status_code=404, detail="not found")
    return m


@app.get("/api/committee/{body_id}")
def api_committee(body_id: str) -> dict:
    c = db.get_committee(body_id)
    if not c:
        raise HTTPException(status_code=404, detail="not found")
    return c


@app.get("/api/live")
def api_live() -> dict:
    """Council sessions in progress right now (Europe/Berlin). `meeting` is the
    primary (soonest-ending) one or null; `meetings` lists all concurrent ones."""
    live = db.live_meetings()
    return {"meeting": live[0] if live else None, "meetings": live}


# Per-meeting in-progress guard: a plain set is race-free here (the event loop is
# single-threaded and membership check + add happen with no await in between),
# unlike the previous lock-registry whose eviction had a window where two
# refreshes of the same meeting could run concurrently.
_refreshing: set[str] = set()
# Site-wide cap so a burst of refreshes (the endpoint is public + unauthenticated) can
# never starve the shared sync-route threadpool that serves search/reads.
_refresh_sema = asyncio.Semaphore(2)


@app.post("/api/meeting/{meeting_id}/refresh")
async def api_meeting_refresh(meeting_id: str) -> dict:
    """On-demand re-scrape of one meeting (pulls newly-added live minutes/results)."""
    if not re.fullmatch(r"termin-\d+", meeting_id):
        raise HTTPException(status_code=400, detail="invalid meeting id")
    if meeting_id in _refreshing:
        raise HTTPException(status_code=409, detail="refresh already running")
    _refreshing.add(meeting_id)
    try:
        async with _refresh_sema:
            try:
                result = await run_in_threadpool(scraper.scrape_one, meeting_id)
            except ValueError as e:
                raise HTTPException(status_code=400, detail=str(e))
            except HTTPException:
                raise
            except Exception as e:  # noqa: BLE001
                raise HTTPException(status_code=502, detail=f"refresh failed: {e}")
    finally:
        _refreshing.discard(meeting_id)
    m = db.get_meeting(meeting_id)
    if not m:
        raise HTTPException(status_code=404, detail="not found")
    m["_refresh"] = result
    return m


@app.get("/api/recent")
def api_recent(since: Optional[str] = None, limit: int = Query(50, ge=1, le=200)) -> list[dict]:
    return db.recent_documents(since=since, limit=limit)


@app.get("/api/people")
def api_people() -> list[dict]:
    return db.list_people()


@app.get("/api/person/{person_id}")
def api_person(person_id: str) -> dict:
    if not re.fullmatch(r"\d{1,10}", person_id):
        raise HTTPException(status_code=400, detail="invalid person id")
    p = db.get_person(person_id)
    if not p:
        raise HTTPException(status_code=404, detail="not found")
    return p


@app.get("/api/vorlage/{year}/{seq}")
def api_vorlage(year: str, seq: str) -> dict:
    if not (re.fullmatch(r"20\d{2}", year) and re.fullmatch(r"\d{3,5}", seq)):
        raise HTTPException(status_code=400, detail="invalid Vorlagennummer")
    chain = db.vorlage_chain(f"{year}/{seq}")
    if not chain:
        raise HTTPException(status_code=404, detail="not found")
    return chain


_analytics_cache: dict = {"ts": 0.0, "data": None}


@app.get("/api/analytics")
def api_analytics() -> dict:
    """Voting statistics (cached 10 min — the aggregation walks every roll-call)."""
    now = time.monotonic()
    if _analytics_cache["data"] is None or now - _analytics_cache["ts"] > 600:
        _analytics_cache["data"] = db.vote_analytics()
        _analytics_cache["ts"] = now
    return _analytics_cache["data"]


def _ics_escape(s: str) -> str:
    return (s or "").replace("\\", "\\\\").replace(";", "\\;").replace(",", "\\,").replace("\n", "\\n")


@app.get("/api/meetings.ics")
def api_meetings_ics(committee: Optional[str] = None) -> Response:
    """Subscribable calendar: recent + upcoming council sessions (optionally one Gremium)."""
    start = (date.today() - timedelta(days=30)).isoformat()
    meetings = db.list_meetings(committee=committee, date_from=start, order="asc", limit=500)
    lines = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        "PRODID:-//desinformationssystem//karlsruhe//DE",
        "CALSCALE:GREGORIAN",
        "METHOD:PUBLISH",
        f"X-WR-CALNAME:{_ics_escape('Karlsruhe ' + (committee or 'Sitzungskalender'))}",
        "X-WR-TIMEZONE:Europe/Berlin",
        "BEGIN:VTIMEZONE",
        "TZID:Europe/Berlin",
        "BEGIN:DAYLIGHT",
        "TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "TZNAME:CEST",
        "DTSTART:19700329T020000", "RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU",
        "END:DAYLIGHT",
        "BEGIN:STANDARD",
        "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100", "TZNAME:CET",
        "DTSTART:19701025T030000", "RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU",
        "END:STANDARD",
        "END:VTIMEZONE",
    ]
    for m in meetings:
        if not m.get("date"):
            continue
        day = date.fromisoformat(m["date"])
        win = db.parse_time_window(m.get("time"), day)
        summary = m.get("body_name") or m.get("title") or "Sitzung"
        lines += ["BEGIN:VEVENT",
                  f"UID:{m['id']}@dis.delphi.tools",
                  f"DTSTAMP:{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}"]
        if win:
            lines += [f"DTSTART;TZID=Europe/Berlin:{win[0].strftime('%Y%m%dT%H%M%S')}",
                      f"DTEND;TZID=Europe/Berlin:{win[1].strftime('%Y%m%dT%H%M%S')}"]
        else:
            lines += [f"DTSTART;VALUE=DATE:{day.strftime('%Y%m%d')}"]
        lines.append(f"SUMMARY:{_ics_escape(summary)}")
        if m.get("location"):
            lines.append(f"LOCATION:{_ics_escape(m['location'])}")
        if m.get("url"):
            lines.append(f"URL:{m['url']}")
        lines.append(f"DESCRIPTION:{_ics_escape((m.get('title') or '') + ' · ' + (m.get('url') or ''))}")
        lines.append("END:VEVENT")
    lines.append("END:VCALENDAR")
    # RFC5545 wants CRLF line endings
    body = "\r\n".join(lines) + "\r\n"
    return PlainTextResponse(body, media_type="text/calendar; charset=utf-8",
                             headers={"Content-Disposition": 'inline; filename="karlsruhe.ics"'})


def _public_base(request: Request) -> str:
    host = request.headers.get("host", "dis.delphi.tools")
    scheme = "https" if "delphi.tools" in host else "http"
    return f"{scheme}://{host}"


@app.get("/api/feed.xml")
def api_feed(
    request: Request,
    committee: Optional[str] = None,
    doc_type: Optional[str] = Query(None, alias="type"),
    submitter: Optional[str] = None,
    topic: Optional[str] = None,
    limit: int = Query(50, ge=1, le=100),
) -> Response:
    """RSS 2.0 feed of newly archived documents, filterable like the UI."""
    docs = db.recent_documents(limit=limit, committee=committee, doc_type=doc_type,
                               submitter=submitter, topic=topic)
    base = _public_base(request)
    bits = [f for f in (committee, doc_type, submitter, topic) if f]
    title = "Desinformationssystem Karlsruhe" + (f" — {' · '.join(bits)}" if bits else "")
    items = []
    for d in docs:
        # downloaded_at is SQLite datetime('now') = UTC
        try:
            dt = datetime.fromisoformat(d["downloaded_at"]).replace(tzinfo=timezone.utc)
            pub = email.utils.format_datetime(dt)
        except (ValueError, TypeError, KeyError):
            pub = ""
        desc_parts = [p for p in (d.get("doc_type"), d.get("body_name"), d.get("meeting_date")) if p]
        desc = " · ".join(desc_parts)
        if d.get("summary_de"):
            desc += "\n\n" + d["summary_de"]
        link = f"{base}/?doc={d['id']}"
        items.append(
            "<item>"
            f"<title>{xml_escape(d.get('label') or d['id'])}</title>"
            f"<link>{xml_escape(link)}</link>"
            f"<guid isPermaLink=\"false\">{xml_escape(d['id'])}</guid>"
            + (f"<pubDate>{pub}</pubDate>" if pub else "")
            + f"<description>{xml_escape(desc)}</description>"
            "</item>")
    xml = (
        '<?xml version="1.0" encoding="UTF-8"?>'
        '<rss version="2.0"><channel>'
        f"<title>{xml_escape(title)}</title>"
        f"<link>{xml_escape(base)}</link>"
        "<description>Neu archivierte Dokumente aus dem Karlsruher Ratsinformationssystem</description>"
        "<language>de-de</language>"
        + "".join(items) + "</channel></rss>")
    return Response(content=xml, media_type="application/rss+xml; charset=utf-8")


@app.get("/api/file/{file_id}")
def api_file(file_id: str, download: bool = False):
    """Stream the cached PDF (falls back to a redirect to the source if absent)."""
    doc = db.get_document(file_id)
    if not doc:
        raise HTTPException(status_code=404, detail="not found")
    path = doc.get("local_path")
    if path and os.path.exists(path):
        disposition = "attachment" if download else "inline"
        fname = doc.get("filename") or f"{file_id}.pdf"
        return FileResponse(path, media_type="application/pdf", filename=fname,
                            headers={"Content-Disposition": f'{disposition}; filename="{fname}"'})
    if doc.get("url") and doc["url"].startswith(("http://", "https://")):
        return JSONResponse({"redirect": doc["url"]}, status_code=302,
                            headers={"Location": doc["url"]})
    raise HTTPException(status_code=404, detail="file not cached")


@app.get("/api/file/{file_id}/version/{version_id}")
def api_file_version(file_id: str, version_id: int):
    """Stream an archived superseded version of a document."""
    v = db.get_file_version(version_id)
    if not v or v["file_id"] != file_id:
        raise HTTPException(status_code=404, detail="not found")
    path = v.get("local_path")
    # archived files always live under PDF_DIR/versions — refuse anything else
    vdir = os.path.realpath(os.path.join(scraper.PDF_DIR, "versions"))
    if not path or not os.path.realpath(path).startswith(vdir + os.sep) or not os.path.exists(path):
        raise HTTPException(status_code=404, detail="version file missing")
    stamp = (v.get("superseded_at") or "")[:10]
    fname = f"{file_id}.{stamp}.pdf" if stamp else f"{file_id}.old.pdf"
    return FileResponse(path, media_type="application/pdf", filename=fname,
                        headers={"Content-Disposition": f'inline; filename="{fname}"'})


# Static SPA last, so /api/* and /health win.
if os.path.isdir(DIST):
    app.mount("/", StaticFiles(directory=DIST, html=True), name="spa")
