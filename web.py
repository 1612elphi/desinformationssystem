"""
Desinformationssystem — FastAPI JSON API + static Carbon SPA.

One origin (no CORS), print-relay style: API routes are declared first, then the
built Vite/React SPA in web/dist is mounted at "/" with html=True so client-side
routes fall back to index.html.
"""
from __future__ import annotations

import asyncio
import os
import re
import sqlite3
from datetime import date
from typing import Optional

from fastapi import FastAPI, HTTPException, Query, Response
from fastapi.concurrency import run_in_threadpool
from fastapi.responses import FileResponse, JSONResponse
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


# Static SPA last, so /api/* and /health win.
if os.path.isdir(DIST):
    app.mount("/", StaticFiles(directory=DIST, html=True), name="spa")
