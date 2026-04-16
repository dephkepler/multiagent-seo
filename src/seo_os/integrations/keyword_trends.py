from __future__ import annotations

from typing import Any

from seo_os.integrations.keyword_autocomplete import fetch_autocomplete


def fetch_trends(
    *,
    query: str,
    seeds: list[str] | None,
    hl: str,
    gl: str,
    limit: int,
    timeout_sec: float,
) -> list[dict[str, Any]]:
    """MVP trends source: query expansion via trends-like modifiers over suggest.

    This is a direct in-project source path and can be replaced later
    with pytrends-backed implementation without touching workflow contracts.
    """
    modifiers = ["trends", "today", "2026", "rising", "breakout"]
    trend_seeds: list[str] = []
    q = (query or "").strip()
    if q:
        trend_seeds.extend([f"{q} {m}" for m in modifiers])
    if seeds:
        for s in seeds[:10]:
            ss = (s or "").strip()
            if not ss:
                continue
            trend_seeds.append(ss)
            trend_seeds.append(f"{ss} trends")
    rows = fetch_autocomplete(
        query=q,
        seeds=trend_seeds,
        hl=hl,
        gl=gl,
        limit=max(limit, 20),
        timeout_sec=timeout_sec,
    )
    out: list[dict[str, Any]] = []
    for r in rows:
        row = dict(r)
        row["source"] = "trends"
        out.append(row)
    return out[: max(1, min(limit, 500))]

