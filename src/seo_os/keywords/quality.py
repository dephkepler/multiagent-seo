"""Lightweight quality stack for direct source discovery."""
from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


def normalize_keyword(text: str) -> str:
    t = (text or "").strip().lower()
    t = re.sub(r"\s+", " ", t)
    return t


def dedupe_hits(hits: list[dict[str, Any]], *, topic_cap: int = 2) -> list[dict[str, Any]]:
    seen_kw: set[str] = set()
    topic_counts: dict[str, int] = {}
    out: list[dict[str, Any]] = []
    for row in hits:
        kw = normalize_keyword(str(row.get("keyword") or ""))
        if not kw or kw in seen_kw:
            continue
        topic = normalize_keyword(str(row.get("topic") or "")) or kw
        tc = topic_counts.get(topic, 0)
        if tc >= max(1, topic_cap):
            continue
        seen_kw.add(kw)
        topic_counts[topic] = tc + 1
        row2 = dict(row)
        row2["keyword"] = kw
        out.append(row2)
    return out


def score_hits(hits: list[dict[str, Any]]) -> list[dict[str, Any]]:
    source_bonus = {
        "autocomplete": 0.72,
        "trends": 0.75,
        "youtube": 0.68,
        "ahrefs": 0.9,
    }
    out: list[dict[str, Any]] = []
    for row in hits:
        src = str(row.get("source") or "").strip().lower()
        base = source_bonus.get(src, 0.6)
        kw = str(row.get("keyword") or "")
        bonus = 0.05 if len(kw.split()) >= 3 else 0.0
        row2 = dict(row)
        row2["score"] = round(min(1.0, base + bonus), 4)
        out.append(row2)
    return out


def split_novelty(
    hits: list[dict[str, Any]],
    *,
    seen_file: str,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    p = Path(seen_file)
    if p.parent and not p.parent.exists():
        p.parent.mkdir(parents=True, exist_ok=True)
    seen: set[str] = set()
    if p.exists():
        for line in p.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if line:
                seen.add(line)
    new_rows: list[dict[str, Any]] = []
    resurfaced: list[dict[str, Any]] = []
    for row in hits:
        kw = normalize_keyword(str(row.get("keyword") or ""))
        if not kw:
            continue
        if kw in seen:
            resurfaced.append(row)
            continue
        new_rows.append(row)
        seen.add(kw)
    p.write_text("\n".join(sorted(seen)) + ("\n" if seen else ""), encoding="utf-8")
    return new_rows, resurfaced


def source_counts(hits: list[dict[str, Any]]) -> dict[str, int]:
    out: dict[str, int] = {}
    for row in hits:
        src = str(row.get("source") or "unknown").strip().lower() or "unknown"
        out[src] = out.get(src, 0) + 1
    return out


def brief_meta(hits: list[dict[str, Any]], *, new_count: int, resurfaced_count: int) -> str:
    payload = {
        "total": len(hits),
        "new": new_count,
        "resurfaced": resurfaced_count,
        "by_source": source_counts(hits),
    }
    return json.dumps(payload, ensure_ascii=False)

