"""Geo: match Karlsruhe street/place mentions in a document and classify its Stadtteil.

Two passes, both text-only, no geodata:
  1. street mentions — a gazetteer regex finds candidates, then Jev (typesafe.py) judges each
     occurrence (location vs person / address block / incidental) so letterhead and generic-word
     false positives are dropped. Returns the confirmed street names.
  2. district — Jev picks the primary Stadtteil (Choice over the 27 + 'keine'). This reads
     adjectival and compound forms ("Neureuter", "AltKnielingen") that a point-in-polygon over
     street geometry cannot, so the facet needs no OSM polygons.

Validated on a 100-doc German sample: district 37/37 on docs naming one Stadtteil literally;
street filter rejects address blocks at high confidence. Backfill via geo_backfill.py.
"""
from __future__ import annotations

import os
import re
from typing import Optional

import typesafe

STADTTEILE = [
    "Beiertheim-Bulach", "Daxlanden", "Durlach", "Grötzingen", "Grünwettersbach", "Grünwinkel",
    "Hagsfeld", "Hohenwettersbach", "Innenstadt-Ost", "Innenstadt-West", "Knielingen", "Mühlburg",
    "Neureut", "Nordstadt", "Nordweststadt", "Oberreut", "Oststadt", "Palmbach", "Rintheim",
    "Rüppurr", "Stupferich", "Südstadt", "Südweststadt", "Waldstadt", "Weiherfeld-Dammerstock",
    "Weststadt", "Wolfartsweier",
]
_GAZETTEER = os.path.join(os.path.dirname(__file__), "geo", "streets_ka.txt")

STREET_FIELDS = ("label", "agenda_title", "summary_de", "fulltext")
DISTRICT_FIELDS = ("label", "agenda_title", "summary_de")   # short + relevant: avoid context rot
FULLTEXT_SCAN = 120_000
KEEP_CONF = 0.6

_GEO_CRITERIA = {
    "location": "refers to the Karlsruhe street, square, or place itself (the physical location)",
    "person": "refers to a person (e.g. the individual a street is named after), not the place",
    "address": "part of a postal address, sender/recipient block, letterhead, or contact line",
    "incidental": "an ordinary word or phrase that coincides with the name but is not the place",
}


def _load_streets() -> list[str]:
    with open(_GAZETTEER, encoding="utf-8") as f:
        return [ln.strip() for ln in f if ln.strip() and not ln.startswith("#")]


def _build_regex(names: list[str]) -> re.Pattern:
    # longest-first so "Nordweststadt" wins over "Weststadt"; word-bounded, case-insensitive
    alt = "|".join(re.escape(n) for n in sorted(set(names), key=len, reverse=True))
    return re.compile(rf"(?<!\w)(?:{alt})(?!\w)", re.IGNORECASE)


_STREET_RX = _build_regex(_load_streets())


def street_hits(doc: dict) -> list[dict]:
    """Regex candidates with a marked context window, deduped per (name, field)."""
    hits, seen = [], set()
    for field in STREET_FIELDS:
        text = doc.get(field) or ""
        if field == "fulltext":
            text = text[:FULLTEXT_SCAN]
        for m in _STREET_RX.finditer(text):
            name = m.group(0)
            key = (name.lower(), field)
            if key in seen:
                continue
            seen.add(key)
            s, e, off = m.start(), m.end(), max(0, m.start() - 150)
            w = text[off:e + 150]
            w = w[: s - off] + "«" + name + "»" + w[e - off:]
            hits.append({"name": name, "field": field, "window": " ".join(w.split())})
    return hits


def confirm_streets(doc: dict) -> list[str]:
    """Return the distinct street names Jev confirms as real place references in `doc`."""
    # one request per hit: the window is the hit's local context, and keeping hits separate stops
    # one occurrence's text from distracting another's judgment (jaggedness: filter first)
    kept = []
    for h in street_hits(doc):
        q = typesafe.choice(
            f'The passage contains «{h["name"]}» marked with guillemets. Judge only that '
            f'occurrence: what does it refer to?', _GEO_CRITERIA)
        a = typesafe.system_one(h["window"], {"ref": q})["ref"]
        if a["choice"] == "location" and a["confidence"] >= KEEP_CONF:
            kept.append(h["name"])
    return sorted(set(kept))


def classify_district(doc: dict) -> tuple[Optional[str], Optional[float]]:
    """Jev picks the primary Stadtteil, or (None, conf) for city-wide / no district."""
    state = " ".join((doc.get(f) or "") for f in DISTRICT_FIELDS).strip()[:6000]
    if not state:
        return None, None
    crit: dict[str, Optional[str]] = {t: None for t in STADTTEILE}
    crit["keine"] = "city-wide, or no specific Karlsruhe Stadtteil is the subject"
    a = typesafe.system_one(state, {"d": typesafe.choice(
        "Which Karlsruhe Stadtteil (city district) is this document primarily about? "
        "Choose 'keine' if it is city-wide or names no district.", crit)})["d"]
    if a["choice"] == "keine":
        return None, a["confidence"]
    return a["choice"], a["confidence"]
