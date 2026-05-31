"""Клиент Trello REST API v1 (карточки, списки, комментарии).

Документация: https://developer.atlassian.com/cloud/trello/rest/
"""
from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any

import httpx

BASE = "https://api.trello.com/1"


class TrelloConfigError(RuntimeError):
    """Не заданы TRELLO_API_KEY / TRELLO_TOKEN или списки."""


@dataclass(frozen=True)
class TrelloListIds:
    inbox: str
    keyword_research: str
    content_draft: str
    qa: str
    done: str
    blocked: str
    keyword_todo: str | None = None
    keyword_inprogress: str | None = None

    @classmethod
    def from_env(cls) -> TrelloListIds:
        def req(name: str) -> str:
            v = os.environ.get(name)
            if not v or not v.strip():
                raise TrelloConfigError(f"Missing env {name}")
            return v.strip()

        return cls(
            inbox=req("TRELLO_LIST_INBOX"),
            keyword_research=req("TRELLO_LIST_KEYWORD_RESEARCH"),
            content_draft=req("TRELLO_LIST_CONTENT_DRAFT"),
            qa=req("TRELLO_LIST_QA"),
            done=req("TRELLO_LIST_DONE"),
            blocked=req("TRELLO_LIST_BLOCKED"),
            keyword_todo=(os.environ.get("TRELLO_LIST_KEYWORD_TODO") or "").strip() or None,
            keyword_inprogress=(os.environ.get("TRELLO_LIST_KEYWORD_INPROGRESS") or "").strip() or None,
        )


class TrelloClient:
    def __init__(self, api_key: str, token: str, timeout_s: float = 30.0) -> None:
        self._api_key = api_key
        self._token = token
        self._timeout = timeout_s

    @classmethod
    def from_env(cls) -> TrelloClient:
        key = os.environ.get("TRELLO_API_KEY", "").strip()
        token = os.environ.get("TRELLO_TOKEN", "").strip()
        if not key or not token:
            raise TrelloConfigError("Set TRELLO_API_KEY and TRELLO_TOKEN")
        return cls(key, token)

    def _params(self, extra: dict[str, Any] | None = None) -> dict[str, Any]:
        p: dict[str, Any] = {"key": self._api_key, "token": self._token}
        if extra:
            p.update(extra)
        return p

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        json: Any | None = None,
    ) -> Any:
        url = f"{BASE}{path}" if path.startswith("/") else f"{BASE}/{path}"
        with httpx.Client(timeout=self._timeout) as client:
            kw: dict[str, Any] = {"params": self._params(params)}
            if json is not None:
                kw["json"] = json
            r = client.request(method, url, **kw)
            r.raise_for_status()
            if r.content:
                return r.json()
            return None

    def get_board(self, board_id_or_shortlink: str) -> dict[str, Any]:
        """Разрешить shortLink (напр. qPDyVJen) в объект доски."""
        return self._request("GET", f"/boards/{board_id_or_shortlink}")

    def lists_for_board(self, board_id_or_shortlink: str) -> list[dict[str, Any]]:
        """Все открытые списки доски (id, name, pos, …)."""
        return self._request(
            "GET",
            f"/boards/{board_id_or_shortlink}/lists",
            params={"filter": "open"},
        )

    def create_list(
        self,
        *,
        id_board: str,
        name: str,
        pos: str | float = "bottom",
    ) -> dict[str, Any]:
        """Создать колонку на доске (pos: top, bottom или число)."""
        return self._request(
            "POST",
            "/lists",
            params={"name": name, "idBoard": id_board, "pos": pos},
        )

    def create_card(
        self,
        *,
        id_list: str,
        name: str,
        desc: str,
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            "/cards",
            params={"idList": id_list, "name": name, "desc": desc},
        )

    def move_card_to_list(self, card_id: str, id_list: str) -> dict[str, Any]:
        return self._request("PUT", f"/cards/{card_id}", params={"idList": id_list})

    def add_comment(self, card_id: str, text: str) -> dict[str, Any]:
        return self._request(
            "POST",
            f"/cards/{card_id}/actions/comments",
            params={"text": text},
        )

    def cards_for_list(self, list_id: str, *, limit: int = 50) -> list[dict[str, Any]]:
        return self._request(
            "GET",
            f"/lists/{list_id}/cards",
            params={"fields": "id,name,desc,idList,shortUrl,dateLastActivity", "limit": max(1, min(limit, 200))},
        )


def card_url_short(card_id: str, short_link_board: str | None = None) -> str:
    """Короткая ссылка на карточку (для логов). Полный URL в ответе API часто в shortUrl."""
    base = os.environ.get("TRELLO_BOARD_SHORT_LINK", "qPDyVJen").strip()
    if short_link_board:
        base = short_link_board
    return f"https://trello.com/c/{card_id}/{base}"


__all__ = [
    "TrelloClient",
    "TrelloConfigError",
    "TrelloListIds",
    "card_url_short",
]
