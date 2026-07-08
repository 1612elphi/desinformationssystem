"""
Desinformationssystem — LLM enrichment via OpenRouter (DeepSeek V4 Pro).

For each freshly-extracted document we ask the model for a bilingual summary, a
single top-level category (controlled vocabulary), free-form topic tags and named
entities. Output is forced to JSON. Rate-limited + retried; never crashes the
scrape — a failure marks the row 'error' (retried next run) and moves on.
Disabled wholesale with ENABLE_LLM=0, or per-missing-key (marks rows 'skipped').
"""
from __future__ import annotations

import json
import logging
import os
import threading
import time
from typing import Optional

import httpx

import db

log = logging.getLogger("enrich")

OPENROUTER_BASE = os.environ.get("OPENROUTER_BASE", "https://openrouter.ai/api/v1")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY", "")
LLM_MODEL = os.environ.get("LLM_MODEL", "deepseek/deepseek-v4-pro")
ENRICH_MAXCHARS = int(os.environ.get("ENRICH_MAXCHARS", "24000"))
LLM_TIMEOUT = float(os.environ.get("LLM_TIMEOUT", "120"))
LLM_RETRIES = int(os.environ.get("LLM_RETRIES", "3"))
LLM_MIN_INTERVAL = float(os.environ.get("LLM_MIN_INTERVAL", "1.0"))
HTTP_REFERER = os.environ.get("OPENROUTER_REFERER", "https://desinformationssystem.local")
APP_TITLE = "Desinformationssystem"

# Controlled top-level categories for Karlsruhe municipal documents.
CATEGORIES = [
    "Bauen & Stadtentwicklung",
    "Umwelt & Klima",
    "Verkehr & Mobilität",
    "Soziales & Gesundheit",
    "Bildung & Kultur",
    "Finanzen & Haushalt",
    "Wirtschaft & Wissenschaft",
    "Verwaltung & Organisation",
    "Sicherheit & Ordnung",
    "Sonstiges",
]

SYSTEM_PROMPT = (
    "Du bist ein Archivar, der Dokumente aus dem Karlsruher Ratsinformationssystem "
    "erschließt. Antworte ausschließlich mit einem JSON-Objekt, keine Erklärungen."
)

_lock = threading.Lock()
_last_call = 0.0


def enabled() -> bool:
    return bool(OPENROUTER_API_KEY) and os.environ.get("ENABLE_LLM", "1") == "1"


def _throttle() -> None:
    global _last_call
    with _lock:
        wait = LLM_MIN_INTERVAL - (time.monotonic() - _last_call)
        if wait > 0:
            time.sleep(wait)
        _last_call = time.monotonic()


def _build_prompt(doc: dict) -> str:
    text = (doc.get("fulltext") or "")[:ENRICH_MAXCHARS]
    cats = ", ".join(CATEGORIES)
    return (
        f"Kontext:\n"
        f"- Gremium: {doc.get('body_name') or 'unbekannt'}\n"
        f"- Sitzung: {doc.get('meeting_title') or ''} ({doc.get('meeting_date') or ''})\n"
        f"- Dokumenttyp: {doc.get('doc_type') or ''}\n"
        f"- Bezeichnung: {doc.get('label') or ''}\n\n"
        f"Dokumenttext (ggf. gekürzt):\n\"\"\"\n{text}\n\"\"\"\n\n"
        f"Erzeuge ein JSON-Objekt mit genau diesen Feldern:\n"
        f"  summary_de: 2-4 Sätze deutsche Zusammenfassung des Inhalts.\n"
        f"  summary_en: dieselbe Zusammenfassung auf Englisch.\n"
        f"  category: GENAU einer dieser Werte: {cats}.\n"
        f"  topics: Liste von 3-7 kurzen deutschen Schlagworten.\n"
        f"  entities: Objekt mit den Listen people, orgs, locations "
        f"(Personen, Organisationen/Fraktionen/Ämter, Orte/Straßen), je 0-8 Einträge.\n"
        f"Wenn der Text leer oder unbrauchbar ist, gib leere Zusammenfassungen und "
        f"category \"Sonstiges\" zurück."
    )


def _call_openrouter(prompt: str) -> dict:
    headers = {
        "Authorization": f"Bearer {OPENROUTER_API_KEY}",
        "HTTP-Referer": HTTP_REFERER,
        "X-Title": APP_TITLE,
        "Content-Type": "application/json",
    }
    payload = {
        "model": LLM_MODEL,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": prompt},
        ],
        "response_format": {"type": "json_object"},
        "temperature": 0.2,
    }
    last_err: Optional[Exception] = None
    for attempt in range(LLM_RETRIES):
        _throttle()
        try:
            r = httpx.post(f"{OPENROUTER_BASE}/chat/completions", headers=headers,
                           json=payload, timeout=LLM_TIMEOUT)
            if r.status_code == 429 or r.status_code >= 500:
                raise httpx.HTTPStatusError(f"status {r.status_code}", request=r.request, response=r)
            r.raise_for_status()
            content = r.json()["choices"][0]["message"]["content"]
            return _parse_json(content)
        except Exception as e:  # noqa: BLE001
            last_err = e
            log.warning("openrouter attempt %d/%d failed: %s", attempt + 1, LLM_RETRIES, e)
            time.sleep(min(30, 2 ** attempt))
    raise RuntimeError(f"openrouter failed after {LLM_RETRIES} attempts: {last_err}")


def _parse_json(content: str) -> dict:
    content = content.strip()
    if content.startswith("```"):
        content = content.strip("`")
        content = content[content.find("{"):]
    try:
        return json.loads(content)
    except json.JSONDecodeError:
        start, end = content.find("{"), content.rfind("}")
        if start >= 0 and end > start:
            return json.loads(content[start:end + 1])
        raise


def _normalise(result: dict) -> dict:
    cat = (result.get("category") or "Sonstiges").strip()
    if cat not in CATEGORIES:
        cat = next((c for c in CATEGORIES if c.lower() == cat.lower()), "Sonstiges")
    topics = result.get("topics") or []
    if isinstance(topics, str):
        topics = [t.strip() for t in topics.split(",") if t.strip()]
    ents = result.get("entities") or {}
    if not isinstance(ents, dict):
        ents = {}
    entities = {k: (ents.get(k) or []) for k in ("people", "orgs", "locations")}
    return {
        "summary_de": (result.get("summary_de") or "").strip(),
        "summary_en": (result.get("summary_en") or "").strip(),
        "category": cat,
        "topics": [str(t).strip() for t in topics if str(t).strip()][:7],
        "entities": entities,
    }


def enrich_file(file_id: str) -> bool:
    """Enrich one file. Returns True on success. Sets enrich_status accordingly."""
    doc = db.get_document(file_id)
    if not doc:
        return False
    if not enabled():
        with db.write_conn() as c:
            c.execute("UPDATE files SET enrich_status='skipped' WHERE id=?", (file_id,))
        return False
    try:
        raw = _call_openrouter(_build_prompt(doc))
        data = _normalise(raw)
    except Exception as e:  # noqa: BLE001
        log.warning("enrich error for %s: %s", file_id, e)
        with db.write_conn() as c:
            c.execute("UPDATE files SET enrich_status='error' WHERE id=?", (file_id,))
        return False
    with db.write_conn() as c:
        c.execute(
            """UPDATE files SET summary_de=?, summary_en=?, category=?, topics=?,
                                entities=?, enrich_status='ok', enriched_at=datetime('now')
               WHERE id=?""",
            (data["summary_de"], data["summary_en"], data["category"],
             json.dumps(data["topics"], ensure_ascii=False),
             json.dumps(data["entities"], ensure_ascii=False), file_id),
        )
        db.reindex_file(c, file_id)
    return True


if __name__ == "__main__":
    import sys
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    if len(sys.argv) > 1:
        print("enriched" if enrich_file(sys.argv[1]) else "not enriched")
    else:
        print("enabled:", enabled(), "| model:", LLM_MODEL)
