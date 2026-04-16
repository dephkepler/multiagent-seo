"""Сид демо-кампании и старт SeoCampaignWorkflow (общий код для demo и Telegram)."""
from __future__ import annotations

import os
import uuid
from decimal import Decimal

from sqlalchemy import text
from temporalio.client import Client

from seo_os.db.session import get_engine
from seo_os.temporal.shared_inputs import CampaignInput, CampaignResult
from seo_os.temporal.worker import TASK_QUEUE
from seo_os.temporal.workflows.campaign import SeoCampaignWorkflow


def seed_demo_campaign() -> uuid.UUID:
    """Вставить sites / campaigns / campaign_budgets как в seo-os-demo."""
    engine = get_engine()
    site_id = uuid.uuid4()
    campaign_id = uuid.uuid4()
    with engine.connect() as conn:
        conn.execute(
            text(
                """
                INSERT INTO sites (id, name, base_url)
                VALUES (:id, 'demo-site', 'https://example.com')
                """
            ),
            {"id": site_id},
        )
        conn.execute(
            text(
                """
                INSERT INTO campaigns (id, site_id, name, status)
                VALUES (:id, :site_id, 'demo-campaign', 'draft')
                """
            ),
            {"id": campaign_id, "site_id": site_id},
        )
        conn.execute(
            text(
                """
                INSERT INTO campaign_budgets (campaign_id, budget_limit, spent, currency)
                VALUES (:cid, :limit, 0, 'USD')
                """
            ),
            {"cid": campaign_id, "limit": Decimal("100.00")},
        )
        conn.commit()
    return campaign_id


async def _connect_temporal() -> Client:
    host = os.environ.get("TEMPORAL_ADDRESS", "localhost:7233")
    return await Client.connect(host)


def _campaign_input(
    campaign_id: str,
    telegram_chat_id: str | None,
    telegram_report_chat_id: str | None = None,
) -> CampaignInput:
    """Читает KEYWORD_ADAPTER_ROUTE и KEYWORD_QUERY из окружения (клиент старта, не workflow)."""
    def _int_env(name: str, default: int) -> int:
        raw = (os.environ.get(name) or "").strip()
        if not raw:
            return default
        try:
            return int(raw)
        except ValueError:
            return default

    def _bool_env(name: str, default: bool) -> bool:
        raw = (os.environ.get(name) or "").strip().lower()
        if not raw:
            return default
        return raw in {"1", "true", "yes", "on"}

    def _csv_env(name: str) -> list[str] | None:
        raw = (os.environ.get(name) or "").strip()
        if not raw:
            return None
        vals = [x.strip() for x in raw.split(",") if x.strip()]
        return vals or None

    def _kv_float_env(name: str) -> dict[str, float] | None:
        raw = (os.environ.get(name) or "").strip()
        if not raw:
            return None
        out: dict[str, float] = {}
        for part in raw.split(","):
            p = part.strip()
            if not p or ":" not in p:
                continue
            k, v = p.split(":", 1)
            try:
                out[k.strip()] = float(v.strip())
            except ValueError:
                continue
        return out or None

    def _kv_int_env(name: str) -> dict[str, int] | None:
        raw = (os.environ.get(name) or "").strip()
        if not raw:
            return None
        out: dict[str, int] = {}
        for part in raw.split(","):
            p = part.strip()
            if not p or ":" not in p:
                continue
            k, v = p.split(":", 1)
            try:
                out[k.strip()] = int(v.strip())
            except ValueError:
                continue
        return out or None

    route = (os.environ.get("KEYWORD_ADAPTER_ROUTE") or "direct_discovery").strip() or "direct_discovery"
    q = (os.environ.get("KEYWORD_QUERY") or "").strip()
    mode = (os.environ.get("KEYWORD_MODE") or "cheap_live").strip() or "cheap_live"
    trends_seed_mode = (os.environ.get("KEYWORD_TRENDS_SEED_MODE") or "merge").strip() or "merge"
    return CampaignInput(
        campaign_id=campaign_id,
        telegram_chat_id=telegram_chat_id,
        telegram_report_chat_id=(telegram_report_chat_id or "").strip() or None,
        keyword_adapter_route=route,
        keyword_query=q,
        keyword_seeds=_csv_env("KEYWORD_SEEDS"),
        keyword_mode=mode,
        keyword_limit=max(1, _int_env("KEYWORD_LIMIT", 80)),
        keyword_sources=_csv_env("KEYWORD_SOURCES"),
        keyword_source_profile=(os.environ.get("KEYWORD_SOURCE_PROFILE") or "cheap").strip() or "cheap",
        keyword_source_timeouts=_kv_float_env("KEYWORD_SOURCE_TIMEOUTS"),
        keyword_source_budgets=_kv_int_env("KEYWORD_SOURCE_BUDGETS"),
        keyword_merge_mode=(os.environ.get("KEYWORD_MERGE_MODE") or "topic_cap").strip() or "topic_cap",
        keyword_topic_cap=max(1, _int_env("KEYWORD_TOPIC_CAP", 2)),
        keyword_hl=(os.environ.get("KEYWORD_HL") or "en").strip() or "en",
        keyword_gl=(os.environ.get("KEYWORD_GL") or "us").strip() or "us",
        keyword_trends_config_path=(
            os.environ.get("KEYWORD_TRENDS_CONFIG_PATH") or "pipeline/config.json"
        ).strip()
        or "pipeline/config.json",
        keyword_trends_subprocess_timeout_sec=_int_env("KEYWORD_TRENDS_SUBPROCESS_TIMEOUT_SEC", 180),
        keyword_trends_filter_by_query=_bool_env("KEYWORD_TRENDS_FILTER_BY_QUERY", True),
        keyword_trends_seed_mode=trends_seed_mode,
        keyword_youtube_region_code=(os.environ.get("KEYWORD_YOUTUBE_REGION_CODE") or "US").strip() or "US",
        keyword_youtube_relevance_language=(
            os.environ.get("KEYWORD_YOUTUBE_RELEVANCE_LANGUAGE") or "en"
        ).strip()
        or "en",
        keyword_youtube_max_results=max(1, _int_env("KEYWORD_YOUTUBE_MAX_RESULTS", 15)),
        keyword_include_raw_ref=_bool_env("KEYWORD_INCLUDE_RAW_REF", False),
        keyword_target_clusters=max(1, _int_env("KEYWORD_TARGET_CLUSTERS", 40)),
        keyword_target_keywords=max(1, _int_env("KEYWORD_TARGET_KEYWORDS", 2000)),
        keyword_max_iterations=max(1, _int_env("KEYWORD_MAX_ITERATIONS", 4)),
    )


def campaign_input_with_overrides(
    campaign_id: str,
    telegram_chat_id: str | None,
    telegram_report_chat_id: str | None = None,
    *,
    overrides: dict[str, object] | None = None,
) -> CampaignInput:
    base = _campaign_input(campaign_id, telegram_chat_id, telegram_report_chat_id=telegram_report_chat_id)
    if not overrides:
        return base
    vals = dict(base.__dict__)
    for k, v in overrides.items():
        if k in vals and v is not None:
            vals[k] = v
    return CampaignInput(**vals)


async def start_seo_campaign_fire_and_forget(
    workflow_id: str | None = None,
    telegram_chat_id: str | None = None,
    telegram_report_chat_id: str | None = None,
) -> tuple[uuid.UUID, str]:
    """Сид + старт workflow без ожидания завершения. Возвращает (campaign_id, temporal_workflow_id).

    telegram_chat_id — опционально: отчёты о фазах в этот чат (или только TELEGRAM_REPORT_CHAT_ID в worker).
    """
    cid = seed_demo_campaign()
    client = await _connect_temporal()
    wid = workflow_id or f"seo-campaign-{cid}-{uuid.uuid4()}"
    await client.start_workflow(
        SeoCampaignWorkflow.run,
        _campaign_input(str(cid), telegram_chat_id, telegram_report_chat_id=telegram_report_chat_id),
        id=wid,
        task_queue=TASK_QUEUE,
    )
    return cid, wid


async def start_seo_campaign_with_overrides_fire_and_forget(
    *,
    overrides: dict[str, object],
    workflow_id: str | None = None,
    telegram_chat_id: str | None = None,
    telegram_report_chat_id: str | None = None,
) -> tuple[uuid.UUID, str]:
    """Сид + старт workflow с override параметров keyword-фазы."""
    cid = seed_demo_campaign()
    client = await _connect_temporal()
    wid = workflow_id or f"seo-campaign-{cid}-{uuid.uuid4()}"
    await client.start_workflow(
        SeoCampaignWorkflow.run,
        campaign_input_with_overrides(
            str(cid),
            telegram_chat_id,
            telegram_report_chat_id=telegram_report_chat_id,
            overrides=overrides,
        ),
        id=wid,
        task_queue=TASK_QUEUE,
    )
    return cid, wid


async def run_seo_campaign_to_completion(
    workflow_id: str | None = None,
    telegram_chat_id: str | None = None,
    telegram_report_chat_id: str | None = None,
) -> CampaignResult:
    """Сид + старт + ожидание результата (как прежний seo-os-demo)."""
    cid = seed_demo_campaign()
    client = await _connect_temporal()
    wid = workflow_id or f"seo-campaign-{cid}-{uuid.uuid4()}"
    handle = await client.start_workflow(
        SeoCampaignWorkflow.run,
        _campaign_input(str(cid), telegram_chat_id, telegram_report_chat_id=telegram_report_chat_id),
        id=wid,
        task_queue=TASK_QUEUE,
    )
    return await handle.result()
