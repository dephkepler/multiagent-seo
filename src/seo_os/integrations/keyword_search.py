"""HTTP-клієнт до зовнішнього Keyword Adapter (/v1/search та /v1/live/*)."""
from __future__ import annotations

import os
from typing import Any

import httpx


class KeywordSearchClient:
    def __init__(self, base_url: str, api_key: str, timeout_sec: float = 30.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key.strip()
        self.timeout_sec = timeout_sec

    @classmethod
    def from_env(cls) -> KeywordSearchClient:
        base = (os.environ.get("KEYWORD_SEARCH_BASE_URL") or "").strip()
        if not base:
            raise ValueError("KEYWORD_SEARCH_BASE_URL is empty")
        key = (os.environ.get("KEYWORD_SEARCH_API_KEY") or "").strip()
        timeout = float((os.environ.get("KEYWORD_SEARCH_TIMEOUT_SEC") or "30").strip() or "30")
        return cls(base, key, timeout_sec=timeout)

    def _headers(self) -> dict[str, str]:
        h: dict[str, str] = {"Content-Type": "application/json"}
        if self.api_key:
            h["Authorization"] = f"Bearer {self.api_key}"
        return h

    def post_json(self, path: str, body: dict[str, Any]) -> dict[str, Any]:
        path = path if path.startswith("/") else f"/{path}"
        url = f"{self.base_url}{path}"
        with httpx.Client(timeout=self.timeout_sec) as client:
            r = client.post(url, json=body, headers=self._headers())
            r.raise_for_status()
            return r.json()

    def search(self, body: dict[str, Any]) -> dict[str, Any]:
        return self.post_json("/v1/search", body)

    def live_autocomplete(self, body: dict[str, Any]) -> dict[str, Any]:
        return self.post_json("/v1/live/autocomplete", body)

    def live_trends(self, body: dict[str, Any]) -> dict[str, Any]:
        return self.post_json("/v1/live/trends", body)

    def live_youtube(self, body: dict[str, Any]) -> dict[str, Any]:
        return self.post_json("/v1/live/youtube", body)
