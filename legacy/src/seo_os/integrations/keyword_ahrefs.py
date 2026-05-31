from __future__ import annotations

import os
from typing import Any

import httpx


def fetch_ahrefs(
    *,
    query: str,
    seeds: list[str] | None,
    limit: int,
    timeout_sec: float,
) -> list[dict[str, Any]]:
    """Minimal Ahrefs direct source call.

    Expects env:
    - AHREFS_API_BASE_URL (optional, default official endpoint)
    - AHREFS_API_TOKEN (required)
    """
    token = (os.environ.get("AHREFS_API_TOKEN") or "").strip()
    if not token:
        return []
    endpoint = (os.environ.get("AHREFS_API_BASE_URL") or "https://api.ahrefs.com/v3/site-explorer/keywords").strip()
    terms: list[str] = []
    q = (query or "").strip()
    if q:
        terms.append(q)
    if seeds:
        terms.extend([s.strip() for s in seeds if s and s.strip()])
    out: list[dict[str, Any]] = []
    with httpx.Client(timeout=timeout_sec) as c:
        for term in terms[:10]:
            r = c.get(
                endpoint,
                params={
                    "token": token,
                    "target": term,
                    "limit": max(1, min(limit, 200)),
                },
            )
            r.raise_for_status()
            payload = r.json()
            rows = payload.get("keywords") or payload.get("data") or []
            for row in rows:
                kw = str((row or {}).get("keyword") or "").strip().lower()
                if not kw:
                    continue
                out.append(
                    {
                        "keyword": kw,
                        "source": "ahrefs",
                        "topic": term,
                        "locale": "global",
                        "raw_ref": None,
                    }
                )
    return out[: max(1, min(limit, 500))]

