"""
Derive a short submitter code for a document:
  - Stadtverwaltung Karlsruhe (the administration) -> "SVK" for administrative
    document types (Beschlussvorlage, Vorlage, Protokoll, Abstimmungsergebnis, …).
  - Anträge / Anfragen -> the submitting council fraction(s), matched from the label
    and the first lines of the text ("Antrag der CDU-Fraktion", "Anfrage der FWV-…").

Returns a list of codes (joint motions can name several fractions); empty if unknown.
This is a heuristic for a UI chip, not authoritative attribution.
"""
from __future__ import annotations

import re
from typing import Optional

# Karlsruhe Gemeinderat fractions/groups → 3-letter code. Order matters only for
# display; matching collects all that appear. Patterns are case-insensitive and
# word-boundaried to limit false positives.
FRACTIONS: list[tuple[str, str, list[str]]] = [
    ("CDU", "CDU",                       [r"\bCDU\b"]),
    ("B90", "Bündnis 90/Die Grünen",     [r"B[üu]ndnis\s*90", r"\bGR[ÜU]NE[NR]?\b", r"\bGruene[nr]?\b", r"Die\s+Gr[üu]nen"]),
    ("SPD", "SPD",                       [r"\bSPD\b"]),
    ("LIN", "Die Linke",                 [r"\bDIE\s+LINKE\b", r"\bLINKE\b", r"\bLinke\b"]),
    ("FDP", "FDP",                       [r"\bFDP\b"]),
    ("AFD", "AfD",                       [r"\bAfD\b", r"\bAFD\b"]),
    ("FWV", "Freie Wähler",              [r"\bFWV\b", r"\bFW\b", r"Freie\s+W[äa]hler"]),
    ("KAL", "Karlsruher Liste",          [r"\bKAL\b", r"Karlsruher\s+Liste"]),
    ("VOL", "Volt",                      [r"\bVolt\b"]),
    ("PAR", "Die PARTEI",                [r"Die\s+PARTEI", r"\bPARTEI\b"]),
    ("HEI", "Die Heimat",                [r"Die\s+Heimat"]),
]

# Legend exposed to the UI / API consumers.
LEGEND: dict[str, str] = {"SVK": "Stadtverwaltung Karlsruhe", **{c: n for c, n, _ in FRACTIONS}}

_COMPILED = [(code, [re.compile(p) for p in pats]) for code, _, pats in FRACTIONS]

# Document types the administration produces.
ADMIN_TYPES = {
    "Beschlussvorlage", "Vorlage", "Niederschrift", "Protokoll", "Wortprotokoll",
    "Tagesordnung", "Einladung", "Abstimmungsergebnis", "Geschäftsordnung",
    "Präsentation", "Plan", "Karte", "Beschluss",
}
# Types whose submitter is a fraction.
PARTY_TYPES = {"Antrag", "Anfrage"}


def _scan(text: str) -> list[str]:
    found: list[str] = []
    for code, regexes in _COMPILED:
        if any(rx.search(text) for rx in regexes):
            found.append(code)
    return found


def derive(label: Optional[str], fulltext: Optional[str], doc_type: Optional[str]) -> list[str]:
    label = label or ""
    doc_type = doc_type or ""

    if doc_type in PARTY_TYPES:
        # label first (usually explicit), then the first lines of the document
        codes = _scan(label) or _scan((fulltext or "")[:800])
        return codes

    if doc_type in ADMIN_TYPES:
        return ["SVK"]

    # Anlage / Sonstiges and the like: only trust an explicit fraction in the label.
    return _scan(label)
