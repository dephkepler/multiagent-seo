from __future__ import annotations

import json
import os
import re

from temporalio import activity

from seo_os.api.chat_backend import _normalize_openai_key
from seo_os.db.repos import (
    get_frontier_candidates,
    get_frontier_tiers,
    get_keyword_vectors,
    get_semantic_neighbors_by_vector,
)
from seo_os.db.session import session_begin
from seo_os.keywords.vector_memory import cosine_similarity, embed_texts
from seo_os.temporal.shared_inputs import (
    KeywordBootstrapInput,
    KeywordBootstrapOutput,
    KeywordCluster,
    KeywordRefineInput,
    KeywordRefineOutput,
)


def _slug(s: str) -> str:
    out = re.sub(r"[^a-z0-9]+", "_", s.lower()).strip("_")
    return out[:40] or "cluster"


def _dedupe_keep_order(items: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for item in items:
        v = (item or "").strip()
        key = v.lower()
        if not v or key in seen:
            continue
        seen.add(key)
        out.append(v)
    return out


def _extract_vertical(query: str, fallback: str) -> str:
    q = (query or "").strip().lower()
    if q.endswith(" keywords"):
        return q[:-9].strip() or fallback
    return fallback


def _heuristic_bootstrap(inp: KeywordBootstrapInput) -> KeywordBootstrapOutput:
    vertical = (inp.vertical or "").strip() or _extract_vertical(inp.query, "general")
    query = (inp.query or "").strip() or f"{vertical} keywords"
    base_seeds = _dedupe_keep_order(list(inp.seeds or []))
    if not base_seeds:
        base_seeds = [
            vertical,
            f"{vertical} keywords",
            f"best {vertical} tips",
            f"{vertical} guide",
            f"{vertical} strategy",
        ]
    intents = ["informational", "commercial", "transactional"]
    clusters: list[KeywordCluster] = []
    for i, seed in enumerate(base_seeds[: max(3, min(12, len(base_seeds)))]):
        label = seed.title()
        clusters.append(
            KeywordCluster(
                id=f"{_slug(seed)}_{i+1}",
                label=label,
                intent=intents[i % len(intents)],
                seed_hints=[seed],
            )
        )
    return KeywordBootstrapOutput(
        seed_version=1,
        cluster_version=1,
        query=query,
        seeds=base_seeds[: max(1, inp.max_seed_initial)],
        clusters=clusters,
        rationale="heuristic bootstrap: OPENAI_API_KEY not configured",
    )


def _call_openai_json(prompt: str) -> dict | None:
    key = _normalize_openai_key(os.environ.get("OPENAI_API_KEY") or "")
    if not key:
        return None
    model = (os.environ.get("OPENAI_CHAT_MODEL") or "gpt-4o-mini").strip()
    try:
        from openai import OpenAI

        client = OpenAI(api_key=key, timeout=120.0)
        resp = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "system",
                    "content": "Return strictly valid JSON. No markdown, no prose.",
                },
                {"role": "user", "content": prompt},
            ],
            max_tokens=1800,
        )
        text = (resp.choices[0].message.content if resp.choices else "") or ""
        text = text.strip()
        if not text:
            return None
        return json.loads(text)
    except Exception:
        activity.logger.exception("keyword strategy openai call failed")
        return None


def _clusters_from_payload(items: list[dict]) -> list[KeywordCluster]:
    out: list[KeywordCluster] = []
    for idx, raw in enumerate(items):
        if not isinstance(raw, dict):
            continue
        label = str(raw.get("label") or "").strip()
        if not label:
            continue
        cid = str(raw.get("id") or _slug(label)).strip() or f"cluster_{idx+1}"
        intent = str(raw.get("intent") or "informational").strip() or "informational"
        hints_raw = raw.get("seed_hints") or []
        hints = _dedupe_keep_order([str(x).strip() for x in hints_raw if str(x).strip()])[:10]
        out.append(KeywordCluster(id=cid, label=label, intent=intent, seed_hints=hints))
    return out


def _frontier_bundle(limit: int = 60) -> dict[str, list[str]]:
    try:
        with session_begin() as session:
            rows = get_frontier_candidates(session, limit=max(10, limit))
            tiers = get_frontier_tiers(session, limit_per_tier=max(10, limit // 2))
        baseline = [str(r.get("canonical_keyword") or "").strip() for r in rows if str(r.get("canonical_keyword") or "").strip()]
        return {
            "baseline": baseline[:limit],
            "f1": list(tiers.get("f1") or [])[: limit // 2],
            "f2": list(tiers.get("f2") or [])[: limit // 2],
            "f3": list(tiers.get("f3") or [])[: limit // 2],
        }
    except Exception:
        activity.logger.exception("frontier read failed")
        return {"baseline": [], "f1": [], "f2": [], "f3": []}


def _strategy_mode(*, cluster_count: int, target_clusters: int) -> str:
    tgt = max(1, target_clusters)
    if cluster_count < int(tgt * 0.6):
        return "exploration"
    return "exploitation"


def _semantic_neighbors(query: str, *, limit: int = 25) -> list[str]:
    q = (query or "").strip()
    if not q:
        return []
    dim = int((os.environ.get("KEYWORD_VECTOR_DIM") or "128").strip() or "128")
    q_vecs, _ = embed_texts([q], dim=dim)
    if not q_vecs:
        return []
    q_vec = q_vecs[0]
    try:
        with session_begin() as session:
            sem_rows = get_semantic_neighbors_by_vector(
                session,
                query_vector=q_vec,
                limit=max(limit * 3, 30),
                min_similarity=0.45,
            )
        if sem_rows:
            out: list[str] = []
            seen: set[str] = set()
            for row in sem_rows:
                kw = str(row.get("canonical_keyword") or row.get("normalized_keyword") or "").strip()
                key = kw.lower()
                if not kw or key in seen:
                    continue
                seen.add(key)
                out.append(kw)
                if len(out) >= limit:
                    break
            return out
        # Fallback query in a fresh transaction to avoid carrying failed-tx state.
        with session_begin() as session:
            rows = get_keyword_vectors(session, limit=1200)
    except Exception:
        activity.logger.exception("vector retrieval failed")
        return []
    scored: list[tuple[float, str]] = []
    for row in rows:
        raw_vec = row.get("vector")
        if not isinstance(raw_vec, list):
            continue
        try:
            v = [float(x) for x in raw_vec]
        except Exception:
            continue
        score = cosine_similarity(q_vec, v)
        kw = str(row.get("canonical_keyword") or row.get("normalized_keyword") or "").strip()
        if kw:
            scored.append((score, kw))
    scored.sort(key=lambda x: x[0], reverse=True)
    out: list[str] = []
    seen: set[str] = set()
    for score, kw in scored:
        if score < 0.45:
            continue
        k = kw.lower()
        if k in seen:
            continue
        seen.add(k)
        out.append(kw)
        if len(out) >= limit:
            break
    return out


@activity.defn(name="keyword_llm_bootstrap")
def keyword_llm_bootstrap(inp: KeywordBootstrapInput) -> KeywordBootstrapOutput:
    fallback = _heuristic_bootstrap(inp)
    frontier = _frontier_bundle(limit=60)
    semantic = _semantic_neighbors(inp.query or inp.vertical, limit=20)
    prompt = (
        "Build initial keyword strategy JSON for SEO discovery.\n"
        "Output schema: "
        '{"query":"str","seeds":["..."],"clusters":[{"id":"str","label":"str","intent":"informational|commercial|transactional","seed_hints":["..."]}],"rationale":"str"}\n'
        "Diversity constraints: use mixed intents and avoid near-duplicates.\n"
        "Mode: exploration. Prioritize neighboring topics breadth-first.\n"
        f"vertical={inp.vertical}\nquery={inp.query}\nhl={inp.hl}\ngl={inp.gl}\n"
        f"target_keywords={inp.target_keywords}\ntarget_clusters={inp.target_clusters}\n"
        f"max_seed_initial={inp.max_seed_initial}\n"
        f"known_keywords_frontier={json.dumps(frontier.get('baseline')[:50], ensure_ascii=False)}\n"
        f"frontier_f1_new={json.dumps(frontier.get('f1')[:30], ensure_ascii=False)}\n"
        f"frontier_f2_underdeveloped={json.dumps(frontier.get('f2')[:30], ensure_ascii=False)}\n"
        f"frontier_f3_resurfacing_angles={json.dumps(frontier.get('f3')[:30], ensure_ascii=False)}\n"
        f"semantic_neighbors={json.dumps(semantic, ensure_ascii=False)}\n"
    )
    data = _call_openai_json(prompt)
    if not data:
        return fallback
    try:
        query = str(data.get("query") or fallback.query).strip() or fallback.query
        seeds = _dedupe_keep_order([str(x).strip() for x in (data.get("seeds") or []) if str(x).strip()])
        if not seeds:
            seeds = fallback.seeds
        seeds = seeds[: max(1, inp.max_seed_initial)]
        clusters = _clusters_from_payload(list(data.get("clusters") or [])) or fallback.clusters
        rationale = str(data.get("rationale") or "llm bootstrap").strip() or "llm bootstrap"
        return KeywordBootstrapOutput(
            seed_version=1,
            cluster_version=1,
            query=query,
            seeds=seeds,
            clusters=clusters,
            rationale=rationale,
        )
    except Exception:
        activity.logger.exception("keyword bootstrap parse failed")
        return fallback


def _heuristic_refine(inp: KeywordRefineInput) -> KeywordRefineOutput:
    prev = _dedupe_keep_order(list(inp.seeds_prev))
    additions = _dedupe_keep_order(list(inp.found_keywords_sample))[: inp.max_new_seeds]
    seeds_next = _dedupe_keep_order(prev + additions)[: inp.max_total_seeds]
    clusters_next = list(inp.clusters_prev)
    added_clusters: list[str] = []
    for kw in additions[: inp.max_new_clusters]:
        cid = _slug(kw)
        if any(c.id == cid for c in clusters_next):
            continue
        clusters_next.append(
            KeywordCluster(
                id=cid,
                label=kw.title(),
                intent="informational",
                seed_hints=[kw],
            )
        )
        added_clusters.append(cid)
    return KeywordRefineOutput(
        seed_version=inp.iteration,
        cluster_version=inp.iteration,
        query_next=inp.query,
        seeds_next=seeds_next,
        clusters_next=clusters_next,
        added_seeds=[s for s in additions if s.lower() not in {x.lower() for x in prev}],
        added_clusters=added_clusters,
        dropped_clusters=[],
        rationale="heuristic refine: OPENAI_API_KEY not configured",
    )


@activity.defn(name="keyword_llm_refine")
def keyword_llm_refine(inp: KeywordRefineInput) -> KeywordRefineOutput:
    fallback = _heuristic_refine(inp)
    frontier = _frontier_bundle(limit=150)
    semantic = _semantic_neighbors(inp.query, limit=35)
    mode = _strategy_mode(cluster_count=max(0, inp.cluster_count_estimate), target_clusters=max(1, inp.target_clusters))
    prompt = (
        "Refine keyword strategy for next iteration.\n"
        "Return strict JSON schema: "
        '{"query_next":"str","seeds_next":["..."],"clusters_next":[{"id":"str","label":"str","intent":"informational|commercial|transactional","seed_hints":["..."]}],"rationale":"str"}\n'
        f"Current mode={mode}.\n"
        "If mode=exploration: maximize breadth using neighboring topics.\n"
        "If mode=exploitation: expand underdeveloped clusters without duplicating exhausted branches.\n"
        "Diversity constraints: at least 2 informational + 2 commercial + 2 problem/comparison seeds when possible.\n"
        "Use multi-tier frontier budget: F1 new 50%, F2 underdeveloped 30%, F3 resurfacing with new angle 20%.\n"
        f"iteration={inp.iteration}\nquery={inp.query}\nhl={inp.hl}\ngl={inp.gl}\n"
        f"keyword_count={inp.keyword_count}\ncluster_count_estimate={inp.cluster_count_estimate}\n"
        f"target_keywords={inp.target_keywords}\ntarget_clusters={inp.target_clusters}\n"
        f"max_new_seeds={inp.max_new_seeds}\nmax_total_seeds={inp.max_total_seeds}\nmax_new_clusters={inp.max_new_clusters}\n"
        f"seeds_prev={json.dumps(inp.seeds_prev, ensure_ascii=False)}\n"
        f"clusters_prev={json.dumps([c.__dict__ for c in inp.clusters_prev], ensure_ascii=False)}\n"
        f"found_keywords_sample={json.dumps(inp.found_keywords_sample[:50], ensure_ascii=False)}\n"
        f"known_keywords_frontier={json.dumps(frontier.get('baseline')[:120], ensure_ascii=False)}\n"
        f"frontier_f1_new={json.dumps(frontier.get('f1')[:60], ensure_ascii=False)}\n"
        f"frontier_f2_underdeveloped={json.dumps(frontier.get('f2')[:60], ensure_ascii=False)}\n"
        f"frontier_f3_resurfacing_angles={json.dumps(frontier.get('f3')[:60], ensure_ascii=False)}\n"
        f"semantic_neighbors={json.dumps(semantic, ensure_ascii=False)}\n"
    )
    data = _call_openai_json(prompt)
    if not data:
        return fallback
    try:
        query_next = str(data.get("query_next") or inp.query).strip() or inp.query
        seeds_next = _dedupe_keep_order([str(x).strip() for x in (data.get("seeds_next") or []) if str(x).strip()])
        if not seeds_next:
            seeds_next = fallback.seeds_next
        seeds_next = seeds_next[: inp.max_total_seeds]
        clusters_next = _clusters_from_payload(list(data.get("clusters_next") or [])) or fallback.clusters_next
        prev_seed_keys = {s.lower() for s in inp.seeds_prev}
        prev_cluster_keys = {c.id for c in inp.clusters_prev}
        added_seeds = [s for s in seeds_next if s.lower() not in prev_seed_keys]
        added_clusters = [c.id for c in clusters_next if c.id not in prev_cluster_keys]
        dropped_clusters = [c.id for c in inp.clusters_prev if c.id not in {x.id for x in clusters_next}]
        return KeywordRefineOutput(
            seed_version=inp.iteration,
            cluster_version=inp.iteration,
            query_next=query_next,
            seeds_next=seeds_next,
            clusters_next=clusters_next,
            added_seeds=added_seeds[: inp.max_new_seeds],
            added_clusters=added_clusters[: inp.max_new_clusters],
            dropped_clusters=dropped_clusters,
            rationale=str(data.get("rationale") or "llm refine").strip() or "llm refine",
        )
    except Exception:
        activity.logger.exception("keyword refine parse failed")
        return fallback
