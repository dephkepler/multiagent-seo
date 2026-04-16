from __future__ import annotations

from typing import Any

import httpx


def fetch_autocomplete(
    *,
    query: str,
    seeds: list[str] | None,
    hl: str,
    gl: str,
    limit: int,
    timeout_sec: float,
) -> list[dict[str, Any]]:
    if not query and not seeds:
        return []
    base_terms: list[str] = []
    if query.strip():
        base_terms.append(query.strip())
    if seeds:
        base_terms.extend([s.strip() for s in seeds if s and s.strip()])
    out: list[dict[str, Any]] = []
    with httpx.Client(timeout=timeout_sec) as c:
        for term in base_terms[:20]:
            r = c.get(
                "https://suggestqueries.google.com/complete/search",
                params={"client": "firefox", "q": term, "hl": hl or "en", "gl": gl or "us"},
            )
            r.raise_for_status()
            payload = r.json()
            suggestions = payload[1] if isinstance(payload, list) and len(payload) > 1 else []
            for s in suggestions:
                txt = str(s).strip()
                if not txt:
                    continue
                out.append(
                    {
                        "keyword": txt,
                        "source": "autocomplete",
                        "topic": term,
                        "locale": (gl or "us").lower(),
                        "raw_ref": None,
                    }
                )
    return out[: max(1, min(limit, 500))]

