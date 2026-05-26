"""Типы входов/выходов workflow и activity (без тяжёлых зависимостей)."""
from __future__ import annotations

from dataclasses import dataclass


@dataclass
class CampaignInput:
    campaign_id: str
    """Если задан — отчёты о фазах в этот Telegram chat; иначе только TELEGRAM_REPORT_CHAT_ID в activity."""
    telegram_chat_id: str | None = None
    """Опциональный второй чат для отчётов (обычно общий TELEGRAM_REPORT_CHAT_ID)."""
    telegram_report_chat_id: str | None = None
    """Передаётся в KeywordBatchInput / Keyword Adapter (см. docs/EXTERNAL_KEYWORD_SERVICE.md)."""
    keyword_adapter_route: str = "direct_discovery"
    keyword_query: str = ""
    keyword_seeds: list[str] | None = None
    keyword_mode: str = "cached"
    keyword_limit: int = 80
    keyword_sources: list[str] | None = None
    keyword_source_profile: str = "cheap"
    keyword_source_timeouts: dict[str, float] | None = None
    keyword_source_budgets: dict[str, int] | None = None
    keyword_merge_mode: str = "topic_cap"
    keyword_topic_cap: int = 2
    keyword_focus_cluster: str | None = None
    keyword_force_depth: bool = False
    keyword_hl: str = "en"
    keyword_gl: str = "us"
    keyword_trends_config_path: str = "pipeline/config.json"
    keyword_trends_subprocess_timeout_sec: int | None = None
    keyword_trends_filter_by_query: bool = True
    keyword_trends_seed_mode: str = "merge"
    keyword_youtube_region_code: str = "US"
    keyword_youtube_relevance_language: str = "en"
    keyword_youtube_max_results: int = 15
    keyword_include_raw_ref: bool = False
    keyword_target_clusters: int = 40
    keyword_target_keywords: int = 2000
    keyword_max_iterations: int = 4
    trello_card_id: str | None = None


@dataclass
class NotifyTelegramInput:
    chat_id: str
    """Пусто — взять TELEGRAM_REPORT_CHAT_ID из env."""
    phase: str
    campaign_id: str
    pipeline_id: str
    detail: str

    def build_message(self) -> str:
        d = self.detail[:3800] if len(self.detail) > 3800 else self.detail
        return (
            f"SEO OS — фаза: {self.phase}\n"
            f"campaign_id={self.campaign_id}\n"
            f"pipeline_id={self.pipeline_id}\n\n"
            f"{d}"
        )


@dataclass
class NotifyTelegramOutput:
    ok: bool
    skipped: bool
    detail: str | None = None


@dataclass
class NotifyTrelloCardInput:
    card_id: str
    phase: str
    campaign_id: str
    pipeline_id: str
    detail: str


@dataclass
class NotifyTrelloCardOutput:
    ok: bool
    skipped: bool
    detail: str | None = None


@dataclass
class CampaignResult:
    keyword_summary: str
    content_summary: str
    qa_summary: str


@dataclass
class KeywordBatchInput:
    campaign_id: str
    job_id: str
    pipeline_id: str
    """Опційно: Keyword Adapter — див. KeywordSearchInput."""
    keyword_query: str = ""
    keyword_seeds: list[str] | None = None
    keyword_mode: str = "cached"
    keyword_limit: int = 50
    keyword_sources: list[str] | None = None
    keyword_source_profile: str = "cheap"
    keyword_source_timeouts: dict[str, float] | None = None
    keyword_source_budgets: dict[str, int] | None = None
    keyword_merge_mode: str = "topic_cap"
    keyword_topic_cap: int = 2
    """search | live_autocomplete | live_trends | live_youtube"""
    keyword_adapter_route: str = "direct_discovery"
    keyword_hl: str = "en"
    keyword_gl: str = "us"
    keyword_trends_config_path: str = "pipeline/config.json"
    keyword_trends_subprocess_timeout_sec: int | None = None
    keyword_trends_filter_by_query: bool = True
    keyword_trends_seed_mode: str = "merge"
    keyword_youtube_region_code: str = "US"
    keyword_youtube_relevance_language: str = "en"
    keyword_youtube_max_results: int = 15
    keyword_include_raw_ref: bool = False


@dataclass
class KeywordSearchInput:
    campaign_id: str
    job_id: str
    pipeline_id: str
    query: str = ""
    seeds: list[str] | None = None
    locale: str = ""
    limit: int = 50
    mode: str = "cached"
    sources: list[str] | None = None
    source_profile: str = "cheap"
    source_timeouts: dict[str, float] | None = None
    source_budgets: dict[str, int] | None = None
    merge_mode: str = "topic_cap"
    topic_cap: int = 2
    novelty_only: bool = False
    seen_file: str = ""
    """search → POST /v1/search; live_* → POST /v1/live/..."""
    adapter_route: str = "search"
    hl: str = "en"
    gl: str = "us"
    trends_config_path: str = "pipeline/config.json"
    trends_subprocess_timeout_sec: int | None = None
    trends_filter_by_query: bool = True
    trends_seed_mode: str = "merge"
    youtube_region_code: str = "US"
    youtube_relevance_language: str = "en"
    youtube_max_results: int = 15
    include_raw_ref: bool = False


@dataclass
class KeywordSearchOutput:
    ok: bool
    skipped: bool
    message: str
    detail: str | None = None
    keyword_count: int | None = None
    cluster_count_estimate: int | None = None


@dataclass
class ContentBatchInput:
    campaign_id: str
    job_id: str
    pipeline_id: str


@dataclass
class QaBatchInput:
    campaign_id: str
    job_id: str
    pipeline_id: str


@dataclass
class BatchResult:
    ok: bool
    message: str
    graph_name: str
    content_score: float | None = None
    keyword_count: int | None = None
    cluster_count_estimate: int | None = None


@dataclass
class RunLangGraphInput:
    campaign_id: str
    job_id: str
    graph_name: str
    """Имя: keyword_trello | content_trello | qa_trello | keyword_skeleton | content_skeleton"""
    pipeline_id: str = ""
    """Общий id пайплайна; обязателен для графов *_trello."""


@dataclass
class RunLangGraphOutput:
    ok: bool
    message: str
    passed_quality_gate: bool | None = None
    content_score: float | None = None


@dataclass
class KeywordBootstrapInput:
    vertical: str
    query: str
    seeds: list[str] | None
    hl: str = "en"
    gl: str = "us"
    target_keywords: int = 2000
    target_clusters: int = 50
    max_seed_initial: int = 80


@dataclass
class KeywordCluster:
    id: str
    label: str
    intent: str
    seed_hints: list[str]


@dataclass
class KeywordBootstrapOutput:
    seed_version: int
    cluster_version: int
    query: str
    seeds: list[str]
    clusters: list[KeywordCluster]
    rationale: str


@dataclass
class KeywordRefineInput:
    iteration: int
    hl: str
    gl: str
    query: str
    seeds_prev: list[str]
    clusters_prev: list[KeywordCluster]
    found_keywords_sample: list[str]
    keyword_count: int
    cluster_count_estimate: int
    target_keywords: int
    target_clusters: int
    max_new_seeds: int = 30
    max_total_seeds: int = 200
    max_new_clusters: int = 10


@dataclass
class KeywordRefineOutput:
    seed_version: int
    cluster_version: int
    query_next: str
    seeds_next: list[str]
    clusters_next: list[KeywordCluster]
    added_seeds: list[str]
    added_clusters: list[str]
    dropped_clusters: list[str]
    rationale: str
