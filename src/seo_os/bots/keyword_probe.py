"""Виклики ендпоінтів Keyword Adapter для перевірки з Telegram (по одному або всі разом)."""
from __future__ import annotations

import json
import os
from typing import Any

import httpx

from seo_os.integrations.keyword_search import KeywordSearchClient

_MISSING_BASE = (
    "KEYWORD_SEARCH_BASE_URL не заданий у .env процесу seo-os-telegram-bot.\n"
    "Додай, наприклад: KEYWORD_SEARCH_BASE_URL=http://127.0.0.1:8088"
)


def _fmt(obj: Any, limit: int = 3500) -> str:
    if isinstance(obj, (dict, list)):
        s = json.dumps(obj, ensure_ascii=False, indent=0)
    else:
        s = str(obj)
    if len(s) > limit:
        return s[: limit - 30] + "\n… (truncated)"
    return s


def _safe_call(label: str, fn: Any) -> str:
    try:
        out = fn()
        return f"✅ {label}\n{_fmt(out)}"
    except Exception as e:
        return f"❌ {label}\n{type(e).__name__}: {e}"


def _base_url_or_error() -> tuple[str | None, str | None]:
    base = (os.environ.get("KEYWORD_SEARCH_BASE_URL") or "").strip()
    if not base:
        return (None, _MISSING_BASE)
    return (base.rstrip("/"), None)


def probe_keyword_health() -> str:
    base, err = _base_url_or_error()
    if err:
        return err

    def health_fn() -> dict[str, Any]:
        url = f"{base}/health"
        key = (os.environ.get("KEYWORD_SEARCH_API_KEY") or "").strip()
        headers: dict[str, str] = {}
        if key:
            headers["Authorization"] = f"Bearer {key}"
        with httpx.Client(timeout=15.0) as c:
            r = c.get(url, headers=headers)
            return {"status_code": r.status_code, "body": r.text[:8000]}

    return _safe_call("GET /health", health_fn)


def _client_or_error() -> tuple[KeywordSearchClient | None, str | None]:
    _, err = _base_url_or_error()
    if err:
        return (None, err)
    try:
        return (KeywordSearchClient.from_env(), None)
    except ValueError as e:
        return (None, f"❌ KeywordSearchClient: {e}")


def probe_keyword_search(query: str) -> str:
    client, err = _client_or_error()
    if err:
        return err
    assert client is not None
    sr = _search_client(client)
    return _safe_call(
        "POST /v1/search",
        lambda: sr.search(
            {
                "query": query,
                "limit": 8,
                "mode": "cheap_live",
                # Без trends/ahrefs: швидко й по запиту; trends окремо — /kw_trends
                "sources": ["autocomplete", "youtube"],
                "correlation_id": "tg-kw-search",
            }
        ),
    )


def probe_keyword_live_autocomplete(query: str) -> str:
    client, err = _client_or_error()
    if err:
        return err
    assert client is not None
    return _safe_call(
        "POST /v1/live/autocomplete",
        lambda: client.live_autocomplete(
            {
                "query": query,
                "limit": 10,
                "hl": "en",
                "gl": "us",
                "correlation_id": "tg-kw-ac",
            }
        ),
    )


def _trends_client(base_client: KeywordSearchClient) -> KeywordSearchClient:
    # HTTP timeout має бути ≥ ніж subprocess на API (LIVE_SUBPROCESS_TIMEOUT_SEC за замовч. 600 с)
    tr_timeout = int((os.environ.get("KEYWORD_PROBE_TRENDS_TIMEOUT_SEC") or "660").strip() or "660")
    return KeywordSearchClient(
        base_client.base_url,
        base_client.api_key,
        timeout_sec=float(max(base_client.timeout_sec, tr_timeout + 30)),
    )


def _search_client(base_client: KeywordSearchClient) -> KeywordSearchClient:
    search_timeout = int((os.environ.get("KEYWORD_PROBE_SEARCH_TIMEOUT_SEC") or "180").strip() or "180")
    return KeywordSearchClient(
        base_client.base_url,
        base_client.api_key,
        timeout_sec=float(max(base_client.timeout_sec, search_timeout + 30)),
    )


def probe_keyword_live_trends(query: str) -> str:
    client, err = _client_or_error()
    if err:
        return err
    assert client is not None
    cfg = (os.environ.get("KEYWORD_TRENDS_CONFIG_PATH") or "pipeline/config.json").strip()
    tr_limit = int((os.environ.get("KEYWORD_PROBE_TRENDS_LIMIT") or "80").strip() or "80")
    tr_limit = max(1, min(200, tr_limit))
    tr = _trends_client(client)
    sub_raw = (os.environ.get("KEYWORD_PROBE_TRENDS_SUBPROCESS_SEC") or "").strip()
    body: dict[str, Any] = {
        "query": query,
        "limit": tr_limit,
        "config_path": cfg,
        "correlation_id": "tg-kw-tr",
        "filter_by_query": False,
        # Збір трендів по тексту з Telegram (seed_keywords = запит), а не лише з igaming config
        "trends_seed_mode": "only_query",
    }
    if sub_raw:
        body["subprocess_timeout_sec"] = int(sub_raw)
    return _safe_call(
        "POST /v1/live/trends",
        lambda: tr.live_trends(body),
    )


def probe_keyword_live_youtube(query: str) -> str:
    client, err = _client_or_error()
    if err:
        return err
    assert client is not None
    return _safe_call(
        "POST /v1/live/youtube",
        lambda: client.live_youtube(
            {
                "query": query,
                "limit": 8,
                "region_code": "US",
                "relevance_language": "en",
                "max_results_per_query": 10,
                "correlation_id": "tg-kw-yt",
            }
        ),
    )


def run_keyword_api_probe(query: str) -> str:
    """Усі ендпоінти послід (довгий звіт)."""
    return "\n\n---\n\n".join(
        [
            probe_keyword_health(),
            probe_keyword_search(query),
            probe_keyword_live_autocomplete(query),
            probe_keyword_live_trends(query),
            probe_keyword_live_youtube(query),
        ]
    )


def split_telegram_chunks(text: str, max_len: int = 3800) -> list[str]:
    """Розбити текст на частини для Telegram; намагається різати по рядках, не посередині JSON-поля."""
    if len(text) <= max_len:
        return [text]

    lines = text.split("\n")
    chunks: list[str] = []
    buf_parts: list[str] = []
    buf_len = 0

    def flush() -> None:
        nonlocal buf_parts, buf_len
        if buf_parts:
            chunks.append("\n".join(buf_parts))
            buf_parts = []
            buf_len = 0

    for line in lines:
        if len(line) > max_len:
            flush()
            for i in range(0, len(line), max_len):
                chunks.append(line[i : i + max_len])
            continue
        addition = len(line) + (1 if buf_parts else 0)
        if buf_len + addition > max_len:
            flush()
        buf_parts.append(line)
        buf_len += addition
    flush()

    n = len(chunks)
    if n <= 1:
        return chunks
    return [f"[{i + 1}/{n}]\n{c}" for i, c in enumerate(chunks)]
