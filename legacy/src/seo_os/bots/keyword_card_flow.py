from __future__ import annotations

import asyncio
import os
import re
import uuid
from typing import Any

from seo_os.integrations.trello import TrelloClient, TrelloConfigError, TrelloListIds
from seo_os.temporal.campaign_starter import start_seo_campaign_with_overrides_fire_and_forget

_LINE_RE = re.compile(r"^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$")


def default_keyword_params() -> dict[str, str]:
    return {
        "KEYWORD_ADAPTER_ROUTE": "direct_discovery",
        "KEYWORD_QUERY": "anti age products",
        "KEYWORD_MODE": "cheap_live",
        "KEYWORD_LIMIT": "80",
        "KEYWORD_SEEDS": "anti age,anti aging products,skin care",
        "KEYWORD_SOURCES": "autocomplete,trends,youtube",
        "KEYWORD_TARGET_CLUSTERS": "40",
        "KEYWORD_TARGET_KEYWORDS": "2000",
        "KEYWORD_MAX_ITERATIONS": "4",
        "KEYWORD_HL": "en",
        "KEYWORD_GL": "us",
        "KEYWORD_TRENDS_CONFIG_PATH": "pipeline/config.json",
        "KEYWORD_TRENDS_SUBPROCESS_TIMEOUT_SEC": "180",
        "KEYWORD_TRENDS_FILTER_BY_QUERY": "true",
        "KEYWORD_TRENDS_SEED_MODE": "merge",
        "KEYWORD_YOUTUBE_REGION_CODE": "US",
        "KEYWORD_YOUTUBE_RELEVANCE_LANGUAGE": "en",
        "KEYWORD_YOUTUBE_MAX_RESULTS": "15",
        "KEYWORD_SOURCE_PROFILE": "cheap",
        "KEYWORD_MERGE_MODE": "topic_cap",
    }


def enrich_keyword_params(params: dict[str, str], *, vertical: str) -> dict[str, str]:
    out = dict(params)
    v = (vertical or "general").strip()
    q = (out.get("KEYWORD_QUERY") or "").strip()
    seeds = (out.get("KEYWORD_SEEDS") or "").strip()

    if not q or q.lower() in {"-", "skip", "auto"}:
        out["KEYWORD_QUERY"] = f"{v} keywords"
    if not seeds or seeds.lower() in {"-", "skip", "auto"}:
        base = out["KEYWORD_QUERY"]
        out["KEYWORD_SEEDS"] = f"{v},{base},best {v} tips"
    return out


def keyword_card_description(
    params: dict[str, str],
    *,
    vertical: str,
    created_by: str,
    telegram_chat_id: str | None = None,
) -> str:
    lines = [
        f"vertical={vertical}",
        f"created_by={created_by}",
        f"telegram_chat_id={(telegram_chat_id or '').strip()}",
        "",
        "# keyword-defaults",
    ]
    for k in sorted(params.keys()):
        lines.append(f"{k}={params[k]}")
    lines.append("")
    lines.append("move_to=ToDo queue to start")
    return "\n".join(lines)


def parse_keyword_params_from_desc(desc: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in (desc or "").splitlines():
        m = _LINE_RE.match(line)
        if not m:
            continue
        key, value = m.group(1), m.group(2)
        if key.startswith("KEYWORD_"):
            out[key] = value
    return out


def keyword_overrides_from_params(params: dict[str, str]) -> dict[str, object]:
    def _int(name: str, default: int) -> int:
        raw = (params.get(name) or "").strip()
        try:
            return int(raw) if raw else default
        except ValueError:
            return default

    def _bool(name: str, default: bool) -> bool:
        raw = (params.get(name) or "").strip().lower()
        if not raw:
            return default
        return raw in {"1", "true", "yes", "on"}

    def _csv(name: str) -> list[str] | None:
        raw = (params.get(name) or "").strip()
        if not raw:
            return None
        vals = [x.strip() for x in raw.split(",") if x.strip()]
        return vals or None

    def _kv_float(name: str) -> dict[str, float] | None:
        raw = (params.get(name) or "").strip()
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

    def _kv_int(name: str) -> dict[str, int] | None:
        raw = (params.get(name) or "").strip()
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

    profile = (params.get("KEYWORD_SOURCE_PROFILE") or "cheap").strip().lower() or "cheap"
    route = (params.get("KEYWORD_ADAPTER_ROUTE") or "direct_discovery").strip() or "direct_discovery"
    sources = _csv("KEYWORD_SOURCES")
    if not sources:
        if profile == "cheap":
            sources = ["autocomplete", "trends", "youtube"]
        elif profile == "balanced":
            sources = ["autocomplete", "trends", "youtube"]
        elif profile == "ahrefs_validate":
            sources = ["autocomplete", "trends", "youtube", "ahrefs"]
    if profile == "ahrefs_validate" and route == "direct_discovery":
        route = "direct_discovery"
    return {
        "keyword_adapter_route": route,
        "keyword_query": (params.get("KEYWORD_QUERY") or "").strip(),
        "keyword_seeds": _csv("KEYWORD_SEEDS"),
        "keyword_mode": (params.get("KEYWORD_MODE") or "cheap_live").strip() or "cheap_live",
        "keyword_limit": max(1, _int("KEYWORD_LIMIT", 80)),
        "keyword_sources": sources,
        "keyword_source_profile": profile,
        "keyword_source_timeouts": _kv_float("KEYWORD_SOURCE_TIMEOUTS"),
        "keyword_source_budgets": _kv_int("KEYWORD_SOURCE_BUDGETS"),
        "keyword_merge_mode": (params.get("KEYWORD_MERGE_MODE") or "topic_cap").strip() or "topic_cap",
        "keyword_hl": (params.get("KEYWORD_HL") or "en").strip() or "en",
        "keyword_gl": (params.get("KEYWORD_GL") or "us").strip() or "us",
        "keyword_trends_config_path": (
            params.get("KEYWORD_TRENDS_CONFIG_PATH") or "pipeline/config.json"
        ).strip()
        or "pipeline/config.json",
        "keyword_trends_subprocess_timeout_sec": _int("KEYWORD_TRENDS_SUBPROCESS_TIMEOUT_SEC", 180),
        "keyword_trends_filter_by_query": _bool("KEYWORD_TRENDS_FILTER_BY_QUERY", True),
        "keyword_trends_seed_mode": (params.get("KEYWORD_TRENDS_SEED_MODE") or "merge").strip() or "merge",
        "keyword_youtube_region_code": (params.get("KEYWORD_YOUTUBE_REGION_CODE") or "US").strip() or "US",
        "keyword_youtube_relevance_language": (
            params.get("KEYWORD_YOUTUBE_RELEVANCE_LANGUAGE") or "en"
        ).strip()
        or "en",
        "keyword_youtube_max_results": max(1, _int("KEYWORD_YOUTUBE_MAX_RESULTS", 15)),
        "keyword_target_clusters": max(1, _int("KEYWORD_TARGET_CLUSTERS", 40)),
        "keyword_target_keywords": max(1, _int("KEYWORD_TARGET_KEYWORDS", 2000)),
        "keyword_max_iterations": max(1, _int("KEYWORD_MAX_ITERATIONS", 4)),
    }


async def run_trello_todo_once(*, telegram_chat_id: str | None) -> str:
    report_chat = (os.environ.get("TELEGRAM_REPORT_CHAT_ID") or "").strip() or None
    """Process one card from ToDo queue -> InProgress -> start workflow."""
    try:
        trello = TrelloClient.from_env()
        lists = TrelloListIds.from_env()
    except TrelloConfigError as e:
        return f"trello config skipped: {e}"
    todo = lists.keyword_todo
    inprog = lists.keyword_inprogress
    if not todo or not inprog:
        return "trello todo skipped: set TRELLO_LIST_KEYWORD_TODO and TRELLO_LIST_KEYWORD_INPROGRESS"

    cards = trello.cards_for_list(todo, limit=20)
    if not cards:
        return "trello todo: no cards"
    card = cards[0]
    card_id = str(card.get("id") or "")
    if not card_id:
        return "trello todo: invalid card payload"
    params = default_keyword_params()
    params.update(parse_keyword_params_from_desc(str(card.get("desc") or "")))
    vmatch = re.search(r"(?im)^\s*vertical\s*=\s*(.*?)\s*$", str(card.get("desc") or ""))
    vertical = (vmatch.group(1).strip() if vmatch else "general") or "general"
    cmatch = re.search(r"(?im)^\s*telegram_chat_id\s*=\s*(.*?)\s*$", str(card.get("desc") or ""))
    card_chat_id = (cmatch.group(1).strip() if cmatch else "") or None
    params = enrich_keyword_params(params, vertical=vertical)
    overrides = keyword_overrides_from_params(params)
    overrides["trello_card_id"] = card_id
    query = str(overrides.get("keyword_query") or "").strip()
    if not query:
        return f"trello todo card {card_id}: missing KEYWORD_QUERY"

    trello.move_card_to_list(card_id, inprog)
    trello.add_comment(card_id, "SEO OS: taken from ToDo, moved to InProgress. Starting workflow.")
    cid, wid = await start_seo_campaign_with_overrides_fire_and_forget(
        overrides=overrides,
        workflow_id=f"seo-card-{card_id}-{uuid.uuid4()}",
        telegram_chat_id=card_chat_id or telegram_chat_id,
        telegram_report_chat_id=report_chat,
    )
    trello.add_comment(
        card_id,
        f"SEO OS: workflow started\ncampaign_id={cid}\nworkflow_id={wid}\nquery={query}",
    )
    return f"trello todo started: card={card_id} workflow={wid}"


async def run_trello_todo_poll_forever(*, telegram_chat_id: str | None, interval_sec: int) -> None:
    while True:
        try:
            await run_trello_todo_once(telegram_chat_id=telegram_chat_id)
        except Exception:
            # keep poller alive; detailed logs are handled by caller
            pass
        await asyncio.sleep(max(15, interval_sec))

