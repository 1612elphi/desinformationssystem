"""Minimal TypeSafe (Jev / System One) client — typed decisions with calibrated confidence.

Used by geo.py for the street precision filter and the Stadtteil classification. Text-only,
stdlib-only. See https://docs.typesafe.ai/api.

Endpoint: TYPESAFE_BASE_URL + /v1/systemone. Point TYPESAFE_BASE_URL at the switchboard proxy
(http://switchboard:3000/typesafe) and no key is needed — switchboard injects it. Set
TYPESAFE_API_KEY only when calling api.typesafe.ai directly.

One request evaluates a `state` against a map of typed `questions` (choice/noul/score) and
returns one answer each. Questions in a request are independent and run in parallel, so batch
freely. Input tokens are billed ($42/Btok); output is free.
"""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.request
from typing import Any

BASE_URL = os.environ.get("TYPESAFE_BASE_URL", "https://api.typesafe.ai").rstrip("/")
ENDPOINT = BASE_URL + "/v1/systemone"
MODEL = os.environ.get("TYPESAFE_MODEL", "jev-latest")


def available() -> bool:
    # a proxy base (switchboard) authenticates upstream; direct calls need a key
    return bool(os.environ.get("TYPESAFE_BASE_URL") or os.environ.get("TYPESAFE_API_KEY"))


def system_one(state: Any, questions: dict[str, dict], retries: int = 5) -> dict[str, dict]:
    """Evaluate `questions` against `state`; return {id: answer}. Retries 429/529 with backoff."""
    headers = {"Content-Type": "application/json"}
    key = os.environ.get("TYPESAFE_API_KEY")
    if key:
        headers["Authorization"] = f"Bearer {key}"
    body = json.dumps({"model": MODEL, "state": state, "questions": questions}).encode()
    req = urllib.request.Request(ENDPOINT, data=body, headers=headers)
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.load(r).get("answers", {})
        except urllib.error.HTTPError as e:
            if e.code in (429, 529) and attempt < retries - 1:
                time.sleep(float(e.headers.get("retry-after") or 2 ** attempt))
                continue
            raise
        except (TimeoutError, urllib.error.URLError):
            if attempt == retries - 1:
                raise
            time.sleep(2 ** attempt)
    return {}


def choice(instructions: str, criteria: dict[str, Any]) -> dict:
    return {"type": "choice", "instructions": instructions, "criteria": criteria}
