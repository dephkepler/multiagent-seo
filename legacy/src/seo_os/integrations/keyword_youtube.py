from __future__ import annotations

import os
from typing import Any

import httpx


def fetch_youtube(
    *,
    query: str,
    seeds: list[str] | None,
    region_code: str,
    relevance_language: str,
    max_results_per_query: int,
    limit: int,
    timeout_sec: float,
) -> list[dict[str, Any]]:
    key = (os.environ.get("YOUTUBE_API_KEY") or "").strip()
    if not key:
        return []
    terms: list[str] = []
    q = (query or "").strip()
    if q:
        terms.append(q)
    if seeds:
        terms.extend([s.strip() for s in seeds if s and s.strip()])
    out: list[dict[str, Any]] = []
    with httpx.Client(timeout=timeout_sec) as c:
        for term in terms[:20]:
            r = c.get(
                "https://www.googleapis.com/youtube/v3/search",
                params={
                    "part": "snippet",
                    "type": "video",
                    "q": term,
                    "maxResults": max(1, min(max_results_per_query, 50)),
                    "regionCode": (region_code or "US").upper(),
                    "relevanceLanguage": (relevance_language or "en").lower(),
                    "key": key,
                },
            )
            r.raise_for_status()
            payload = r.json()
            for item in payload.get("items") or []:
                sn = item.get("snippet") or {}
                title = str(sn.get("title") or "").strip()
                if not title:
                    continue
                out.append(
                    {
                        "keyword": title.lower(),
                        "source": "youtube",
                        "topic": term,
                        "locale": (region_code or "US").upper(),
                        "raw_ref": item.get("id"),
                    }
                )
    return out[: max(1, min(limit, 500))]

