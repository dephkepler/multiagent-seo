"""Activity: keyword search via adapter or direct sources."""
from __future__ import annotations

import json
import os
import uuid
from typing import Any

from temporalio import activity

from seo_os.db.repos import (
    insert_keyword_run_snapshot,
    upsert_cluster_memory,
    upsert_keyword_cluster_link,
    upsert_keyword_memory,
    upsert_keyword_vector,
)
from seo_os.db.session import session_begin
from seo_os.integrations.keyword_ahrefs import fetch_ahrefs
from seo_os.integrations.keyword_autocomplete import fetch_autocomplete
from seo_os.integrations.keyword_search import KeywordSearchClient
from seo_os.integrations.keyword_trends import fetch_trends
from seo_os.integrations.keyword_youtube import fetch_youtube
from seo_os.keywords.quality import brief_meta, dedupe_hits, score_hits, split_novelty
from seo_os.keywords.vector_memory import embed_texts
from seo_os.temporal.shared_inputs import KeywordSearchInput, KeywordSearchOutput


def _format_keywords_payload(data: dict, render_limit: int) -> str:
    keywords = data.get("keywords") or []
    meta = data.get("meta") or {}
    lines: list[str] = []
    lines.append(f"totals: keywords={len(keywords)}")
    if meta:
        lines.append(
            "meta: "
            + json.dumps(meta, ensure_ascii=False)[:2000]
        )
    clipped = max(1, render_limit)
    for item in keywords[:clipped]:
        if isinstance(item, dict):
            kw = item.get("keyword") or item.get("text") or ""
            src = item.get("source") or ""
            extra = {k: v for k, v in item.items() if k not in ("keyword", "text", "source")}
            if extra:
                lines.append(f"- {kw} [{src}] {json.dumps(extra, ensure_ascii=False)[:200]}")
            else:
                lines.append(f"- {kw} [{src}]")
        else:
            lines.append(f"- {item}")
    if len(keywords) > clipped:
        lines.append(f"... and {len(keywords) - clipped} more")
    if not lines:
        return "(empty keywords array)"
    return "\n".join(lines)


def _correlation(inp: KeywordSearchInput) -> str:
    return f"{inp.campaign_id}_{inp.pipeline_id}_{inp.job_id}"


def _keyword_stats(keywords: list[dict[str, Any]]) -> dict[str, Any]:
    source_keywords: dict[str, int] = {}
    source_clusters: dict[str, set[str]] = {}
    cluster_keywords: dict[str, list[str]] = {}

    for row in keywords:
        if not isinstance(row, dict):
            continue
        kw = str(row.get("keyword") or row.get("text") or "").strip()
        src = str(row.get("source") or "unknown").strip().lower() or "unknown"
        cluster = str(row.get("topic") or "").strip() or "(no_topic)"
        source_keywords[src] = source_keywords.get(src, 0) + 1
        source_clusters.setdefault(src, set()).add(cluster)
        if cluster not in cluster_keywords:
            cluster_keywords[cluster] = []
        if kw and kw not in cluster_keywords[cluster]:
            cluster_keywords[cluster].append(kw)

    source_summary: dict[str, dict[str, Any]] = {}
    for src, kw_count in source_keywords.items():
        source_summary[src] = {
            "keywords": kw_count,
            "clusters": len(source_clusters.get(src, set())),
        }

    cluster_items = sorted(
        cluster_keywords.items(),
        key=lambda kv: len(kv[1]),
        reverse=True,
    )
    cluster_summary: dict[str, list[str]] = {}
    for name, kws in cluster_items[:20]:
        cluster_summary[name] = kws[:8]

    return {
        "source_summary": source_summary,
        "cluster_keywords": cluster_summary,
    }


def _persist_keyword_memory(
    *,
    inp: KeywordSearchInput,
    keywords: list[dict[str, Any]],
    new_keywords: set[str],
    stats: dict[str, Any],
) -> dict[str, Any]:
    try:
        campaign_uuid = uuid.UUID(inp.campaign_id)
    except Exception:
        return {"ok": False, "error": "invalid campaign_id"}

    try:
        with session_begin() as session:
            cluster_rows: dict[str, list[dict[str, Any]]] = {}
            for row in keywords:
                if not isinstance(row, dict):
                    continue
                cluster = str(row.get("topic") or "").strip() or "(no_topic)"
                cluster_rows.setdefault(cluster, []).append(row)

            cluster_ids: dict[str, uuid.UUID] = {}
            for cluster_name, rows in cluster_rows.items():
                expected_size = max(20, min(300, len(rows) * 4))
                cluster_ids[cluster_name] = upsert_cluster_memory(
                    session,
                    cluster_key=cluster_name.strip().lower(),
                    label=cluster_name,
                    keyword_count_increment=len(rows),
                    expected_size=expected_size,
                    campaign_id=campaign_uuid,
                    pipeline_id=inp.pipeline_id,
                    extra={"hl": inp.hl, "gl": inp.gl},
                )

            keyword_saved = 0
            vectors_saved = 0
            dim = int((os.environ.get("KEYWORD_VECTOR_DIM") or "128").strip() or "128")
            vector_limit = max(
                20,
                min(
                    250,
                    int((os.environ.get("KEYWORD_VECTOR_BATCH_LIMIT") or "120").strip() or "120"),
                ),
            )
            vector_candidates: list[tuple[str, str]] = []
            for row in keywords:
                if not isinstance(row, dict):
                    continue
                raw_kw = str(row.get("keyword") or row.get("text") or "").strip()
                normalized_kw = raw_kw.lower()
                if not normalized_kw:
                    continue
                km_id = upsert_keyword_memory(
                    session,
                    normalized_keyword=normalized_kw,
                    canonical_keyword=raw_kw,
                    source=str(row.get("source") or "unknown").strip().lower(),
                    score=float(row.get("score")) if row.get("score") is not None else None,
                    is_new=normalized_kw in new_keywords,
                    campaign_id=campaign_uuid,
                    pipeline_id=inp.pipeline_id,
                    extra={"topic": row.get("topic"), "locale": row.get("locale")},
                )
                cluster_name = str(row.get("topic") or "").strip() or "(no_topic)"
                cl_id = cluster_ids.get(cluster_name)
                if cl_id:
                    upsert_keyword_cluster_link(
                        session,
                        keyword_memory_id=km_id,
                        cluster_memory_id=cl_id,
                        campaign_id=campaign_uuid,
                        pipeline_id=inp.pipeline_id,
                    )
                keyword_saved += 1
                if len(vector_candidates) < vector_limit:
                    vector_candidates.append((normalized_kw, raw_kw))

            vector_model: str | None = None
            if vector_candidates:
                vec_texts = [x[1] for x in vector_candidates]
                vectors, vector_model = embed_texts(vec_texts, dim=dim)
                for idx, (normalized_kw, _) in enumerate(vector_candidates):
                    if idx >= len(vectors):
                        break
                    upsert_keyword_vector(
                        session,
                        normalized_keyword=normalized_kw,
                        embedding_model=vector_model,
                        embedding_dim=dim,
                        vector=vectors[idx],
                    )
                    vectors_saved += 1

            insert_keyword_run_snapshot(
                session,
                campaign_id=campaign_uuid,
                pipeline_id=inp.pipeline_id,
                job_id=inp.job_id,
                payload={
                    "route": inp.adapter_route,
                    "query": inp.query,
                    "seed_count": len(inp.seeds or []),
                    "keyword_count": len(keywords),
                    "new_keyword_count": len(new_keywords),
                    "stats": stats,
                    "vector_model": vector_model,
                    "vectors_saved": vectors_saved,
                },
            )
        return {
            "ok": True,
            "keywords_saved": keyword_saved,
            "clusters_saved": len(cluster_rows),
            "vectors_saved": vectors_saved,
            "vector_enabled": True,
        }
    except Exception as e:
        activity.logger.exception("keyword memory persistence failed")
        return {"ok": False, "error": str(e)[:300]}


def _format_direct_payload(
    *,
    route: str,
    keywords: list[dict[str, Any]],
    errors: dict[str, str] | None = None,
    new_count: int | None = None,
    resurfaced_count: int | None = None,
    render_limit: int,
) -> str:
    meta: dict[str, Any] = {
        "mode": route,
        "total": len(keywords),
    }
    if new_count is not None:
        meta["new"] = new_count
    if resurfaced_count is not None:
        meta["resurfaced"] = resurfaced_count
    if errors:
        meta["errors"] = errors
    payload = {"keywords": keywords, "meta": meta}
    return f"[adapter: {route}]\n" + _format_keywords_payload(payload, render_limit=render_limit)


def _collect_direct(inp: KeywordSearchInput, route: str) -> tuple[list[dict[str, Any]], dict[str, str]]:
    timeout_default = float((os.environ.get("KEYWORD_DIRECT_TIMEOUT_SEC") or "30").strip() or "30")
    timeouts = inp.source_timeouts or {}
    source_timeout = lambda name: float(timeouts.get(name) or timeout_default)
    errors: dict[str, str] = {}
    out: list[dict[str, Any]] = []

    def call_source(name: str) -> None:
        try:
            if name == "autocomplete":
                out.extend(
                    fetch_autocomplete(
                        query=inp.query,
                        seeds=inp.seeds,
                        hl=inp.hl,
                        gl=inp.gl,
                        limit=inp.limit,
                        timeout_sec=source_timeout("autocomplete"),
                    )
                )
            elif name == "trends":
                out.extend(
                    fetch_trends(
                        query=inp.query,
                        seeds=inp.seeds,
                        hl=inp.hl,
                        gl=inp.gl,
                        limit=inp.limit,
                        timeout_sec=source_timeout("trends"),
                    )
                )
            elif name == "youtube":
                out.extend(
                    fetch_youtube(
                        query=inp.query,
                        seeds=inp.seeds,
                        region_code=inp.youtube_region_code,
                        relevance_language=inp.youtube_relevance_language,
                        max_results_per_query=inp.youtube_max_results,
                        limit=inp.limit,
                        timeout_sec=source_timeout("youtube"),
                    )
                )
            elif name == "ahrefs":
                out.extend(
                    fetch_ahrefs(
                        query=inp.query,
                        seeds=inp.seeds,
                        limit=inp.limit,
                        timeout_sec=source_timeout("ahrefs"),
                    )
                )
        except Exception as e:
            errors[name] = str(e)[:300]

    if route == "direct_autocomplete":
        call_source("autocomplete")
    elif route == "direct_trends":
        call_source("trends")
    elif route == "direct_youtube":
        call_source("youtube")
    elif route == "direct_ahrefs":
        call_source("ahrefs")
    elif route == "direct_discovery":
        srcs = [s.strip().lower() for s in (inp.sources or ["autocomplete", "trends", "youtube"]) if s and s.strip()]
        for s in srcs:
            call_source(s)
    return out, errors


@activity.defn(name="keyword_search")
def keyword_search(inp: KeywordSearchInput) -> KeywordSearchOutput:
    route = (inp.adapter_route or "search").strip().lower()
    try:
        if route.startswith("direct_"):
            direct_rows, errors = _collect_direct(inp, route)
            deduped = dedupe_hits(direct_rows, topic_cap=max(1, int(inp.topic_cap or 2)))
            scored = score_hits(deduped)
            seen_file = (
                inp.seen_file
                or (os.environ.get("KEYWORD_SEEN_FILE") or "runtime/keyword_seen_keywords.txt").strip()
                or "runtime/keyword_seen_keywords.txt"
            )
            new_rows, resurfaced = split_novelty(scored, seen_file=seen_file)
            keywords = new_rows if inp.novelty_only else scored
            msg = _format_direct_payload(
                route=route,
                keywords=keywords,
                errors=errors or None,
                new_count=len(new_rows),
                resurfaced_count=len(resurfaced),
                render_limit=max(20, inp.limit),
            )
            if len(msg) > 12000:
                msg = msg[:11900] + "\n… (truncated)"
            cluster_count = len({str(r.get("topic") or "").strip().lower() for r in keywords if r.get("topic")})
            stats = _keyword_stats(keywords)
            memory = _persist_keyword_memory(
                inp=inp,
                keywords=keywords,
                new_keywords={str(r.get("keyword") or "").strip().lower() for r in new_rows},
                stats=stats,
            )
            return KeywordSearchOutput(
                ok=True,
                skipped=False,
                message=msg,
                detail=json.dumps(
                    {
                        "meta": brief_meta(scored, new_count=len(new_rows), resurfaced_count=len(resurfaced)),
                        "stats": stats,
                        "memory": memory,
                    },
                    ensure_ascii=False,
                ),
                keyword_count=len(keywords),
                cluster_count_estimate=cluster_count or None,
            )

        if not (os.environ.get("KEYWORD_SEARCH_BASE_URL") or "").strip():
            return KeywordSearchOutput(
                ok=True,
                skipped=True,
                message="",
                detail="KEYWORD_SEARCH_BASE_URL not set",
            )

        client = KeywordSearchClient.from_env()
        data: dict[str, Any]
        if route == "search":
            body: dict[str, Any] = {
                "query": inp.query or "",
                "seeds": list(inp.seeds) if inp.seeds else [],
                "locale": inp.locale or "en",
                "limit": max(1, min(inp.limit, 500)),
                "mode": inp.mode or "cached",
                "correlation_id": _correlation(inp),
            }
            if inp.sources:
                body["sources"] = list(inp.sources)
            data = client.search(body)
        elif route == "live_autocomplete":
            data = client.live_autocomplete(
                {
                    "query": inp.query or "",
                    "seeds": list(inp.seeds) if inp.seeds else [],
                    "limit": max(1, min(inp.limit, 200)),
                    "hl": inp.hl or "en",
                    "gl": inp.gl or "us",
                    "correlation_id": _correlation(inp),
                    "include_raw_ref": inp.include_raw_ref,
                }
            )
        elif route == "live_trends":
            td: dict[str, Any] = {
                "query": inp.query or "",
                "seeds": list(inp.seeds) if inp.seeds else [],
                "limit": max(1, min(inp.limit, 200)),
                "hl": inp.hl or "en",
                "gl": inp.gl or "us",
                "correlation_id": _correlation(inp),
                "include_raw_ref": inp.include_raw_ref,
                "config_path": inp.trends_config_path or "pipeline/config.json",
                "filter_by_query": inp.trends_filter_by_query,
                "trends_seed_mode": inp.trends_seed_mode,
            }
            if inp.trends_subprocess_timeout_sec is not None:
                td["subprocess_timeout_sec"] = inp.trends_subprocess_timeout_sec
            data = client.live_trends(td)
        elif route == "live_youtube":
            data = client.live_youtube(
                {
                    "query": inp.query or "",
                    "seeds": list(inp.seeds) if inp.seeds else [],
                    "limit": max(1, min(inp.limit, 200)),
                    "hl": inp.hl or "en",
                    "gl": inp.gl or "us",
                    "correlation_id": _correlation(inp),
                    "include_raw_ref": inp.include_raw_ref,
                    "region_code": inp.youtube_region_code or "US",
                    "relevance_language": inp.youtube_relevance_language or "en",
                    "max_results_per_query": max(1, min(inp.youtube_max_results, 50)),
                }
            )
        else:
            return KeywordSearchOutput(ok=False, skipped=False, message="", detail=f"unknown adapter_route: {route}")

        keywords: list[dict[str, Any]] = data.get("keywords") or []
        cluster_keys: set[str] = set()
        for item in keywords:
            if not isinstance(item, dict):
                continue
            topic = str(item.get("topic") or "").strip().lower()
            src = str(item.get("source") or "").strip().lower()
            if topic:
                cluster_keys.add(f"topic:{topic}")
            elif src:
                cluster_keys.add(f"source:{src}")
        cluster_count = len(cluster_keys) if cluster_keys else None
        stats = _keyword_stats(keywords)
        memory = _persist_keyword_memory(
            inp=inp,
            keywords=keywords,
            new_keywords={str(r.get("keyword") or "").strip().lower() for r in keywords},
            stats=stats,
        )
        msg = f"[adapter: {route}]\n" + _format_keywords_payload(data, render_limit=max(20, inp.limit))
        if len(msg) > 12000:
            msg = msg[:11900] + "\n… (truncated)"
        return KeywordSearchOutput(
            ok=True,
            skipped=False,
            message=msg,
            detail=json.dumps({"stats": stats, "memory": memory}, ensure_ascii=False),
            keyword_count=len(keywords),
            cluster_count_estimate=cluster_count,
        )
    except Exception as e:
        activity.logger.exception("keyword_search failed")
        return KeywordSearchOutput(
            ok=False,
            skipped=False,
            message="",
            detail=str(e)[:800],
            keyword_count=None,
            cluster_count_estimate=None,
        )
