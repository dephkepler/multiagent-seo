"""Запись в agent_decisions, cost_tracking, проверка campaign_budgets."""
from __future__ import annotations

import json
import uuid
from decimal import Decimal
from typing import Any

from sqlalchemy import text
from sqlalchemy.orm import Session


class BudgetExceededError(Exception):
    """Превышен лимит campaign_budgets."""


def insert_agent_decision(
    session: Session,
    *,
    job_id: uuid.UUID,
    campaign_id: uuid.UUID,
    graph_name: str,
    node: str,
    input_summary: str | None = None,
    output_summary: str | None = None,
    tool_calls: list[dict[str, Any]] | None = None,
    latency_ms: int | None = None,
) -> None:
    session.execute(
        text(
            """
            INSERT INTO agent_decisions (
                id, job_id, campaign_id, graph_name, node,
                input_summary, output_summary, tool_calls, latency_ms
            ) VALUES (
                gen_random_uuid(), :job_id, :campaign_id, :graph_name, :node,
                :input_summary, :output_summary, CAST(:tool_calls AS jsonb), :latency_ms
            )
            """
        ),
        {
            "job_id": job_id,
            "campaign_id": campaign_id,
            "graph_name": graph_name,
            "node": node,
            "input_summary": input_summary,
            "output_summary": output_summary,
            "tool_calls": json.dumps(tool_calls or []),
            "latency_ms": latency_ms,
        },
    )


def get_budget_row(session: Session, campaign_id: uuid.UUID) -> dict[str, Any] | None:
    row = session.execute(
        text(
            "SELECT budget_limit, spent, currency FROM campaign_budgets WHERE campaign_id = :cid"
        ),
        {"cid": campaign_id},
    ).mappings().first()
    return dict(row) if row else None


def assert_budget_allows_spend(session: Session, campaign_id: uuid.UUID, estimated_usd: Decimal) -> None:
    row = get_budget_row(session, campaign_id)
    if not row or row.get("budget_limit") is None:
        return
    limit = Decimal(str(row["budget_limit"]))
    spent = Decimal(str(row["spent"]))
    if spent + estimated_usd > limit:
        raise BudgetExceededError(
            f"Budget exceeded: spent={spent} + est={estimated_usd} > limit={limit}"
        )


def record_llm_cost(
    session: Session,
    *,
    campaign_id: uuid.UUID,
    amount_usd: Decimal,
    operation_type: str,
    idempotency_key: str,
    keyword_id: uuid.UUID | None = None,
    article_draft_id: uuid.UUID | None = None,
) -> None:
    session.execute(
        text(
            """
            INSERT INTO cost_tracking (
                id, campaign_id, keyword_id, article_draft_id,
                operation_type, amount, currency, provider, idempotency_key
            ) VALUES (
                gen_random_uuid(), :campaign_id, :keyword_id, :article_draft_id,
                :operation_type, :amount, 'USD', 'skeleton', :idempotency_key
            )
            ON CONFLICT (idempotency_key) DO NOTHING
            """
        ),
        {
            "campaign_id": campaign_id,
            "keyword_id": keyword_id,
            "article_draft_id": article_draft_id,
            "operation_type": operation_type,
            "amount": str(amount_usd),
            "idempotency_key": idempotency_key,
        },
    )
    session.execute(
        text(
            """
            UPDATE campaign_budgets
            SET spent = spent + :amount, updated_at = now()
            WHERE campaign_id = :campaign_id
            """
        ),
        {"amount": str(amount_usd), "campaign_id": campaign_id},
    )


__all__ = [
    "BudgetExceededError",
    "insert_agent_decision",
    "get_budget_row",
    "assert_budget_allows_spend",
    "record_llm_cost",
    "upsert_keyword_memory",
    "upsert_cluster_memory",
    "upsert_keyword_cluster_link",
    "insert_keyword_run_snapshot",
    "get_frontier_candidates",
    "get_frontier_tiers",
    "upsert_keyword_vector",
    "get_keyword_vectors",
    "get_semantic_neighbors_by_vector",
]


def upsert_keyword_memory(
    session: Session,
    *,
    normalized_keyword: str,
    canonical_keyword: str,
    source: str | None,
    score: float | None,
    is_new: bool,
    campaign_id: uuid.UUID | None,
    pipeline_id: str | None,
    extra: dict[str, Any] | None = None,
) -> uuid.UUID:
    row = session.execute(
        text(
            """
            INSERT INTO keyword_memory (
                normalized_keyword, canonical_keyword, first_seen_at, last_seen_at,
                seen_count, new_count, resurfaced_count, avg_score, best_score,
                last_source, last_campaign_id, last_pipeline_id, extra
            ) VALUES (
                :normalized_keyword, :canonical_keyword, now(), now(),
                1, :new_init, :resurfaced_init, :avg_score, :best_score,
                :last_source, :last_campaign_id, :last_pipeline_id, CAST(:extra AS jsonb)
            )
            ON CONFLICT (normalized_keyword) DO UPDATE SET
                canonical_keyword = EXCLUDED.canonical_keyword,
                last_seen_at = now(),
                seen_count = keyword_memory.seen_count + 1,
                new_count = keyword_memory.new_count + :new_inc,
                resurfaced_count = keyword_memory.resurfaced_count + :resurfaced_inc,
                avg_score = CASE
                    WHEN :avg_score IS NULL THEN keyword_memory.avg_score
                    WHEN keyword_memory.avg_score IS NULL THEN :avg_score
                    ELSE ((keyword_memory.avg_score * keyword_memory.seen_count) + :avg_score) / (keyword_memory.seen_count + 1)
                END,
                best_score = CASE
                    WHEN :best_score IS NULL THEN keyword_memory.best_score
                    WHEN keyword_memory.best_score IS NULL THEN :best_score
                    WHEN :best_score > keyword_memory.best_score THEN :best_score
                    ELSE keyword_memory.best_score
                END,
                last_source = COALESCE(:last_source, keyword_memory.last_source),
                last_campaign_id = COALESCE(:last_campaign_id, keyword_memory.last_campaign_id),
                last_pipeline_id = COALESCE(:last_pipeline_id, keyword_memory.last_pipeline_id),
                extra = CASE
                    WHEN :extra = '{}' THEN keyword_memory.extra
                    ELSE keyword_memory.extra || CAST(:extra AS jsonb)
                END
            RETURNING id
            """
        ),
        {
            "normalized_keyword": normalized_keyword,
            "canonical_keyword": canonical_keyword,
            "new_init": 1 if is_new else 0,
            "resurfaced_init": 0 if is_new else 1,
            "new_inc": 1 if is_new else 0,
            "resurfaced_inc": 0 if is_new else 1,
            "avg_score": score,
            "best_score": score,
            "last_source": source,
            "last_campaign_id": campaign_id,
            "last_pipeline_id": pipeline_id,
            "extra": json.dumps(extra or {}),
        },
    ).mappings().first()
    return row["id"]


def upsert_cluster_memory(
    session: Session,
    *,
    cluster_key: str,
    label: str,
    keyword_count_increment: int,
    expected_size: int,
    campaign_id: uuid.UUID | None,
    pipeline_id: str | None,
    extra: dict[str, Any] | None = None,
) -> uuid.UUID:
    row = session.execute(
        text(
            """
            INSERT INTO cluster_memory (
                cluster_key, label, first_seen_at, last_seen_at, seen_count,
                keyword_count, expected_size, saturation_score, status,
                last_campaign_id, last_pipeline_id, extra
            ) VALUES (
                :cluster_key, :label, now(), now(), 1,
                :keyword_count_increment, :expected_size,
                :sat_init, 'active',
                :last_campaign_id, :last_pipeline_id, CAST(:extra AS jsonb)
            )
            ON CONFLICT (cluster_key) DO UPDATE SET
                label = EXCLUDED.label,
                last_seen_at = now(),
                seen_count = cluster_memory.seen_count + 1,
                keyword_count = cluster_memory.keyword_count + :keyword_count_increment,
                expected_size = GREATEST(cluster_memory.expected_size, :expected_size),
                saturation_score = (
                    (cluster_memory.keyword_count + :keyword_count_increment)::numeric /
                    GREATEST(GREATEST(cluster_memory.expected_size, :expected_size), 1)
                ),
                last_campaign_id = COALESCE(:last_campaign_id, cluster_memory.last_campaign_id),
                last_pipeline_id = COALESCE(:last_pipeline_id, cluster_memory.last_pipeline_id),
                extra = CASE
                    WHEN :extra = '{}' THEN cluster_memory.extra
                    ELSE cluster_memory.extra || CAST(:extra AS jsonb)
                END
            RETURNING id
            """
        ),
        {
            "cluster_key": cluster_key,
            "label": label,
            "keyword_count_increment": max(0, keyword_count_increment),
            "expected_size": max(1, expected_size),
            "sat_init": (max(0, keyword_count_increment) / max(1, expected_size)),
            "last_campaign_id": campaign_id,
            "last_pipeline_id": pipeline_id,
            "extra": json.dumps(extra or {}),
        },
    ).mappings().first()
    return row["id"]


def upsert_keyword_cluster_link(
    session: Session,
    *,
    keyword_memory_id: uuid.UUID,
    cluster_memory_id: uuid.UUID,
    campaign_id: uuid.UUID | None,
    pipeline_id: str | None,
) -> None:
    session.execute(
        text(
            """
            INSERT INTO keyword_cluster_links (
                keyword_memory_id, cluster_memory_id, first_seen_at, last_seen_at, seen_count,
                last_campaign_id, last_pipeline_id
            ) VALUES (
                :keyword_memory_id, :cluster_memory_id, now(), now(), 1, :last_campaign_id, :last_pipeline_id
            )
            ON CONFLICT (keyword_memory_id, cluster_memory_id) DO UPDATE SET
                last_seen_at = now(),
                seen_count = keyword_cluster_links.seen_count + 1,
                last_campaign_id = COALESCE(:last_campaign_id, keyword_cluster_links.last_campaign_id),
                last_pipeline_id = COALESCE(:last_pipeline_id, keyword_cluster_links.last_pipeline_id)
            """
        ),
        {
            "keyword_memory_id": keyword_memory_id,
            "cluster_memory_id": cluster_memory_id,
            "last_campaign_id": campaign_id,
            "last_pipeline_id": pipeline_id,
        },
    )


def insert_keyword_run_snapshot(
    session: Session,
    *,
    campaign_id: uuid.UUID,
    pipeline_id: str,
    job_id: str,
    payload: dict[str, Any],
) -> None:
    session.execute(
        text(
            """
            INSERT INTO keyword_run_snapshots (
                campaign_id, pipeline_id, job_id, payload, created_at
            ) VALUES (
                :campaign_id, :pipeline_id, :job_id, CAST(:payload AS jsonb), now()
            )
            ON CONFLICT (pipeline_id, job_id) DO UPDATE SET
                payload = EXCLUDED.payload
            """
        ),
        {
            "campaign_id": campaign_id,
            "pipeline_id": pipeline_id,
            "job_id": job_id,
            "payload": json.dumps(payload or {}),
        },
    )


def get_frontier_candidates(
    session: Session,
    *,
    limit: int = 200,
) -> list[dict[str, Any]]:
    rows = session.execute(
        text(
            """
            SELECT
                km.normalized_keyword,
                km.canonical_keyword,
                km.seen_count,
                km.new_count,
                km.resurfaced_count,
                km.last_seen_at
            FROM keyword_memory km
            ORDER BY
                (km.new_count - km.resurfaced_count) DESC,
                km.seen_count ASC,
                km.last_seen_at DESC
            LIMIT :limit
            """
        ),
        {"limit": max(1, limit)},
    ).mappings().all()
    return [dict(r) for r in rows]


def get_frontier_tiers(
    session: Session,
    *,
    limit_per_tier: int = 40,
) -> dict[str, list[str]]:
    lim = max(5, limit_per_tier)

    f1_rows = session.execute(
        text(
            """
            SELECT km.canonical_keyword
            FROM keyword_memory km
            WHERE km.seen_count <= 2
            ORDER BY km.new_count DESC, km.last_seen_at DESC
            LIMIT :lim
            """
        ),
        {"lim": lim},
    ).scalars().all()

    f2_rows = session.execute(
        text(
            """
            SELECT t.canonical_keyword
            FROM (
                SELECT
                    km.canonical_keyword,
                    cm.saturation_score,
                    km.last_seen_at,
                    ROW_NUMBER() OVER (
                        PARTITION BY km.canonical_keyword
                        ORDER BY cm.saturation_score ASC, km.last_seen_at DESC
                    ) AS rn
                FROM cluster_memory cm
                JOIN keyword_cluster_links kcl ON kcl.cluster_memory_id = cm.id
                JOIN keyword_memory km ON km.id = kcl.keyword_memory_id
                WHERE
                    cm.status = 'active'
                    AND cm.saturation_score < 0.5
                    AND km.canonical_keyword IS NOT NULL
                    AND km.canonical_keyword <> ''
            ) t
            WHERE t.rn = 1
            ORDER BY t.saturation_score ASC, t.last_seen_at DESC
            LIMIT :lim
            """
        ),
        {"lim": lim},
    ).scalars().all()

    f3_rows = session.execute(
        text(
            """
            SELECT km.canonical_keyword
            FROM keyword_memory km
            WHERE km.resurfaced_count > 0 AND km.new_count > 0
            ORDER BY (km.new_count - km.resurfaced_count) DESC, km.last_seen_at DESC
            LIMIT :lim
            """
        ),
        {"lim": lim},
    ).scalars().all()

    def _clean(values: list[str]) -> list[str]:
        out: list[str] = []
        seen: set[str] = set()
        for v in values:
            s = str(v or "").strip()
            k = s.lower()
            if not s or k in seen:
                continue
            seen.add(k)
            out.append(s)
        return out

    return {
        "f1": _clean(f1_rows),
        "f2": _clean(f2_rows),
        "f3": _clean(f3_rows),
    }


def upsert_keyword_vector(
    session: Session,
    *,
    normalized_keyword: str,
    embedding_model: str,
    embedding_dim: int,
    vector: list[float],
) -> None:
    vector_str = "[" + ",".join(f"{float(v):.8f}" for v in vector) + "]"
    session.execute(
        text(
            """
            INSERT INTO keyword_vectors (
                normalized_keyword, embedding_model, embedding_dim, vector, created_at, updated_at
            ) VALUES (
                :normalized_keyword, :embedding_model, :embedding_dim, CAST(:vector AS jsonb), now(), now()
            )
            ON CONFLICT (normalized_keyword) DO UPDATE SET
                embedding_model = EXCLUDED.embedding_model,
                embedding_dim = EXCLUDED.embedding_dim,
                vector = EXCLUDED.vector,
                updated_at = now()
            """
        ),
        {
            "normalized_keyword": normalized_keyword,
            "embedding_model": embedding_model,
            "embedding_dim": max(1, int(embedding_dim)),
            "vector": json.dumps(vector),
        },
    )
    if int(embedding_dim) != 128:
        return
    try:
        has_vector_type = bool(
            session.execute(
                text("SELECT EXISTS(SELECT 1 FROM pg_type WHERE typname = 'vector')")
            ).scalar()
        )
        if not has_vector_type:
            return
        session.execute(
            text(
                """
                UPDATE keyword_vectors
                SET vector_pg = CAST(:vector_str AS vector), updated_at = now()
                WHERE normalized_keyword = :normalized_keyword
                """
            ),
            {"vector_str": vector_str, "normalized_keyword": normalized_keyword},
        )
    except Exception:
        # Optional pgvector acceleration; JSON vector storage stays primary fallback.
        return


def get_keyword_vectors(
    session: Session,
    *,
    limit: int = 1500,
) -> list[dict[str, Any]]:
    rows = session.execute(
        text(
            """
            SELECT
                kv.normalized_keyword,
                km.canonical_keyword,
                kv.embedding_model,
                kv.embedding_dim,
                kv.vector,
                kv.updated_at
            FROM keyword_vectors kv
            LEFT JOIN keyword_memory km
                ON km.normalized_keyword = kv.normalized_keyword
            ORDER BY kv.updated_at DESC
            LIMIT :lim
            """
        ),
        {"lim": max(1, limit)},
    ).mappings().all()
    return [dict(r) for r in rows]


def get_semantic_neighbors_by_vector(
    session: Session,
    *,
    query_vector: list[float],
    limit: int = 30,
    min_similarity: float = 0.45,
) -> list[dict[str, Any]]:
    if not query_vector:
        return []
    vec_str = "[" + ",".join(f"{float(v):.8f}" for v in query_vector) + "]"
    try:
        rows = session.execute(
            text(
                """
                SELECT
                    kv.normalized_keyword,
                    km.canonical_keyword,
                    (1 - (kv.vector_pg <=> CAST(:vec AS vector))) AS similarity
                FROM keyword_vectors kv
                LEFT JOIN keyword_memory km
                    ON km.normalized_keyword = kv.normalized_keyword
                WHERE kv.vector_pg IS NOT NULL
                ORDER BY kv.vector_pg <=> CAST(:vec AS vector)
                LIMIT :lim
                """
            ),
            {"vec": vec_str, "lim": max(1, limit)},
        ).mappings().all()
        out = []
        for r in rows:
            sim = float(r.get("similarity") or 0.0)
            if sim < min_similarity:
                continue
            out.append(dict(r))
        return out
    except Exception:
        # Reset transaction state so caller can safely run fallback queries in same session.
        try:
            session.rollback()
        except Exception:
            pass
        # pgvector extension/index may be absent in some envs; caller can fallback.
        return []
