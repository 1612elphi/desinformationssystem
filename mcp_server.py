"""
Desinformationssystem — MCP server (FastMCP, Streamable HTTP).

Read-only tools over the indexed Karlsruhe council documents. Served at /mcp on
its own port (default 3651), mirroring the More Later MCP deployment shape.
"""
from __future__ import annotations

import os
from typing import Optional

from mcp.server.fastmcp import FastMCP

import db

mcp = FastMCP(
    name="desinformationssystem",
    instructions=(
        "Durchsuchbares Archiv des Karlsruher Ratsinformationssystems "
        "(Sitzungskalender). Gremien (committees) -> Sitzungen (meetings) -> "
        "Tagesordnungspunkte -> Dateien (PDF documents). Use search_documents for "
        "full-text + faceted search; document/meeting tools return full metadata. "
        "Dates are ISO (YYYY-MM-DD). Document fields include German+English summaries, "
        "a category, topic tags and extracted entities. Parsed vote tallies "
        "(Abstimmungsergebnisse, incl. per-member roll-call) are available via "
        "search_votes / get_votes and inside get_meeting."
    ),
    host="0.0.0.0",
    port=int(os.environ.get("MCP_PORT", "3651")),
    streamable_http_path="/mcp",
)


@mcp.tool()
def search_documents(
    query: Optional[str] = None,
    committee: Optional[str] = None,
    doc_type: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    public: Optional[bool] = None,
    topic: Optional[str] = None,
    submitter: Optional[str] = None,
    limit: int = 25,
    offset: int = 0,
) -> dict:
    """Full-text + faceted search over all indexed documents.

    Args:
        query: free-text search (German); matches label, full text, summaries, topics.
        committee: committee name or id (Gremium) to filter by.
        doc_type: e.g. Beschlussvorlage, Protokoll, Vorlage, Antrag, Niederschrift.
        date_from / date_to: ISO dates bounding the meeting date.
        public: True = only public items, False = only non-public, None = all.
        topic: filter to documents carrying this exact topic tag.
        submitter: filter by submitter code — SVK (Stadtverwaltung) or a fraction
            (CDU, B90, SPD, LIN, FDP, AFD, FWV, KAL, VOL, PAR, HEI).
        limit / offset: pagination (limit <= 200).
    Returns: {"total": int, "results": [document, ...]}. Each document carries a
    "submitters" list of such codes.
    """
    return db.search_documents(
        query=query, committee=committee, doc_type=doc_type, date_from=date_from,
        date_to=date_to, public=public, topic=topic, submitter=submitter,
        limit=min(limit, 200), offset=offset,
    )


@mcp.tool()
def get_document(file_id: str) -> dict:
    """Return one document with full metadata, summaries, topics, entities and
    (truncated) extracted text. file_id is the numeric RIS file id."""
    doc = db.get_document(file_id)
    if not doc:
        return {"error": "not found", "file_id": file_id}
    if doc.get("fulltext"):
        doc["fulltext"] = doc["fulltext"][:8000]
    return doc


@mcp.tool()
def list_committees() -> dict:
    """List all committees (Gremien) with member, meeting and document counts."""
    return {"committees": db.list_committees()}


@mcp.tool()
def list_meetings(
    committee: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    upcoming: bool = False,
    limit: int = 50,
) -> dict:
    """List meetings (Sitzungen), optionally filtered by committee and ISO date range.
    Set upcoming=True for only future sessions, sorted soonest-first."""
    import datetime
    order = "desc"
    if upcoming:
        date_from = date_from or datetime.date.today().isoformat()
        order = "asc"
    return {"meetings": db.list_meetings(committee=committee, date_from=date_from,
                                         date_to=date_to, order=order, limit=min(limit, 500))}


@mcp.tool()
def get_committee(committee_id: str) -> dict:
    """Return a committee (Gremium) with its member roster (name, party, role),
    party composition, meeting list and document count. committee_id is the RIS slug,
    e.g. 'organisation-gr-32130'."""
    c = db.get_committee(committee_id)
    return c or {"error": "not found", "committee_id": committee_id}


@mcp.tool()
def get_meeting(meeting_id: str) -> dict:
    """Return one meeting with its agenda items, attached documents and parsed
    vote tallies ('votes': per-TOP Ja/Nein/Enthaltung + member roll-call).
    meeting_id is the RIS slug, e.g. 'termin-10526'."""
    m = db.get_meeting(meeting_id)
    return m or {"error": "not found", "meeting_id": meeting_id}


@mcp.tool()
def search_votes(
    committee: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    member: Optional[str] = None,
    query: Optional[str] = None,
    include_members: bool = False,
    limit: int = 50,
    offset: int = 0,
) -> dict:
    """Search parsed vote tallies (Abstimmungsergebnisse) across all meetings,
    newest first. Tallies come from the live ticker and the official result PDFs
    ('source': 'live' | 'pdf'; pdf is authoritative).

    Args:
        committee: committee name or id (Gremium) to filter by.
        date_from / date_to: ISO dates bounding the meeting date.
        member: council-member name (substring, case-insensitive); each hit then
            carries 'member_votes' with how the matching member(s) voted
            (ja / nein / enthaltung / abwesend).
        query: substring match on TOP label, result text or agenda-item title.
        include_members: True = include the full per-member roll-call per vote
            (large; default off — use get_votes/get_meeting for one meeting).
        limit / offset: pagination (limit <= 200).
    Returns: {"total": int, "results": [vote, ...]} — each vote has meeting_id,
    body_name, meeting_date, top_label, agenda_number/title, ja/nein/enthaltung
    counts and result_text."""
    return db.search_votes(
        committee=committee, date_from=date_from, date_to=date_to, member=member,
        query=query, include_members=include_members,
        limit=min(limit, 200), offset=offset,
    )


@mcp.tool()
def get_votes(meeting_id: str) -> dict:
    """All parsed vote tallies for one meeting, in agenda (TOP) order, including
    the full per-member roll-call. meeting_id is the RIS slug, e.g. 'termin-10526'."""
    return {"meeting_id": meeting_id, "votes": db.meeting_votes(meeting_id)}


@mcp.tool()
def recent_documents(since: Optional[str] = None, limit: int = 25) -> dict:
    """Most recently downloaded documents. 'since' is an ISO datetime lower bound."""
    return {"documents": db.recent_documents(since=since, limit=min(limit, 200))}


@mcp.tool()
def stats() -> dict:
    """Index statistics: counts of bodies/meetings/documents/votes and last scrape time."""
    return db.stats()


if __name__ == "__main__":
    db.init_db()
    mcp.run(transport="streamable-http")
