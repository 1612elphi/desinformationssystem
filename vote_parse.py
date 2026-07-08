"""
Vote-tally parser for the Karlsruhe "SITZUNG LIVE" panels.

The council's vote results are published only as images (live-ticker JPGs and the
image-only "Abstimmungsergebnis" PDFs) — a clean, templated panel with big JA / NEIN /
ENTHALTUNG counters plus a per-member grid coloured by vote (yellow=Ja, red=Nein,
grey=Enthaltung/abwesend). We read them with an OpenRouter vision model into structured
JSON. Benchmarked accurate on real live + archived tally images (gemini-3.1-flash-lite,
gemini-2.5-flash, gpt-4.1-mini, gpt-4o-mini all read the aggregate counts exactly);
default model is google/gemini-3.1-flash-lite (VISION_MODEL).
"""
from __future__ import annotations

import base64
import json
import logging
import os
import time
from typing import Optional

import httpx

log = logging.getLogger("vote_parse")

OPENROUTER_BASE = os.environ.get("OPENROUTER_BASE", "https://openrouter.ai/api/v1")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY", "")
VISION_MODEL = os.environ.get("VISION_MODEL", "google/gemini-3.1-flash-lite")
VISION_TIMEOUT = float(os.environ.get("VISION_TIMEOUT", "90"))
VISION_RETRIES = int(os.environ.get("VISION_RETRIES", "3"))

PROMPT = (
    "Dies ist ein Abstimmungspanel eines deutschen Gemeinderats (\"SITZUNG LIVE\"). "
    "Oben rechts stehen drei große Zähler: JA, NEIN, ENTHALTUNG. Darunter ist ein Raster "
    "der Mitglieder, gruppiert nach Fraktion; die Kachelfarbe codiert die Stimme: "
    "GELB = Ja, ROT = Nein, GRAU = Enthaltung/abwesend. "
    "Gib NUR ein JSON-Objekt zurück, keine Erklärung, mit den Feldern: "
    "ja (int), nein (int), enthaltung (int), "
    "members (Liste von {\"name\": str, \"vote\": \"ja\"|\"nein\"|\"enthaltung\"|\"abwesend\"}). "
    "Lies die drei Zähler exakt ab; members nur für klar erkennbare farbige Kacheln."
)


def enabled() -> bool:
    return bool(OPENROUTER_API_KEY) and os.environ.get("ENABLE_VOTE_PARSE", "1") == "1"


def _parse_json(content: str) -> dict:
    content = content.strip()
    if content.startswith("```"):
        content = content.strip("`")
        content = content[content.find("{"):]
    try:
        return json.loads(content)
    except json.JSONDecodeError:
        s, e = content.find("{"), content.rfind("}")
        if s >= 0 and e > s:
            return json.loads(content[s:e + 1])
        raise


def parse_vote_image(image_bytes: bytes, mime: str = "image/jpeg") -> Optional[dict]:
    """Return {ja,nein,enthaltung,members[]} or None on failure."""
    if not enabled():
        return None
    data_uri = f"data:{mime};base64,{base64.b64encode(image_bytes).decode()}"
    payload = {
        "model": VISION_MODEL,
        "temperature": 0,
        "messages": [{"role": "user", "content": [
            {"type": "text", "text": PROMPT},
            {"type": "image_url", "image_url": {"url": data_uri}},
        ]}],
    }
    headers = {
        "Authorization": f"Bearer {OPENROUTER_API_KEY}",
        "X-Title": "Desinformationssystem",
        "Content-Type": "application/json",
    }
    last = None
    for attempt in range(VISION_RETRIES):
        try:
            r = httpx.post(f"{OPENROUTER_BASE}/chat/completions", headers=headers,
                           json=payload, timeout=VISION_TIMEOUT)
            if r.status_code == 429 or r.status_code >= 500:
                raise httpx.HTTPStatusError(f"status {r.status_code}", request=r.request, response=r)
            r.raise_for_status()
            raw = _parse_json(r.json()["choices"][0]["message"]["content"])
            return _normalise(raw)
        except Exception as e:  # noqa: BLE001
            last = e
            log.warning("vote parse attempt %d/%d failed: %s", attempt + 1, VISION_RETRIES, e)
            time.sleep(min(20, 2 ** attempt))
    log.error("vote parse gave up: %s", last)
    return None


def _to_int(x) -> Optional[int]:
    try:
        return int(round(float(str(x).strip())))  # tolerate "5" / "5.0" / 5.0
    except (ValueError, TypeError):
        return None


_VOTE_ALIASES = {
    "ja": "ja", "yes": "ja", "dafür": "ja", "zustimmung": "ja",
    "nein": "nein", "no": "nein", "dagegen": "nein", "ablehnung": "nein",
    "enthaltung": "enthaltung", "enthalten": "enthaltung", "abstention": "enthaltung",
    "abwesend": "abwesend", "absent": "abwesend", "nicht anwesend": "abwesend",
}


def _norm_vote(v) -> str:
    s = str(v or "").lower().strip()
    return _VOTE_ALIASES.get(s, "abwesend")  # unknown → treat as not-voting


def _normalise(raw: dict) -> Optional[dict]:
    ja, nein, enth = _to_int(raw.get("ja")), _to_int(raw.get("nein")), _to_int(raw.get("enthaltung"))
    if ja is None and nein is None and enth is None:
        return None
    members = raw.get("members") or []
    clean = []
    if isinstance(members, list):
        for m in members:
            if isinstance(m, dict) and m.get("name"):
                clean.append({"name": str(m["name"]).strip(), "vote": _norm_vote(m.get("vote"))})
    return {"ja": ja, "nein": nein, "enthaltung": enth, "members": clean}


if __name__ == "__main__":
    import sys
    logging.basicConfig(level=logging.INFO)
    if len(sys.argv) > 1:
        print(parse_vote_image(open(sys.argv[1], "rb").read()))
    else:
        print("enabled:", enabled(), "model:", VISION_MODEL)
