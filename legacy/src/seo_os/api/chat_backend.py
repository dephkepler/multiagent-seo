"""Логика ответа в чате: OpenAI или заглушка без ключа."""
from __future__ import annotations

import os
from typing import NamedTuple

_SYSTEM = (
    "Ты ассистент проекта SEO OS (оркестрация кампаний через Temporal и LangGraph). "
    "Отвечай кратко и по делу; язык ответа — как у пользователя, если он пишет по-русски или по-украински."
)
_SYSTEM_CAMPAIGN = (
    _SYSTEM
    + " В начале сообщения пользователя может быть блок «Контекст кампании» с campaign_id и workflow_id — "
    "учитывай это при ответах (статус, уточнения, Trello и т.д.)."
)


class ChatResult(NamedTuple):
    reply: str
    note: str | None


def _normalize_openai_key(raw: str) -> str:
    """Убрать пробелы и лишние кавычки вокруг ключа из .env."""
    s = raw.strip()
    if len(s) >= 2 and ((s[0] == s[-1] == '"') or (s[0] == s[-1] == "'")):
        s = s[1:-1]
    return s.strip()


def _stub_reply(message: str, session_id: str | None) -> ChatResult:
    snippet = message.strip()[:500]
    return ChatResult(
        reply=(
            f"Сообщение принято (сессия: {session_id or '—'}). "
            f"Фрагмент: «{snippet}»"
        ),
        note=(
            "Заглушка: задайте OPENAI_API_KEY в .env и перезапустите seo-os-api или seo-os-telegram-bot."
        ),
    )


def complete_chat(
    message: str,
    session_id: str | None,
    *,
    dialogue_context: str | None = None,
) -> ChatResult:
    """dialogue_context — префикс к user-сообщению (режим кампании в Telegram, шаг D)."""
    key = _normalize_openai_key(os.environ.get("OPENAI_API_KEY") or "")
    user_raw = message.strip()
    if dialogue_context:
        user_raw = f"{dialogue_context.strip()}\n\n---\n\n{user_raw}"
    if not key:
        return _stub_reply(user_raw, session_id)

    model = (os.environ.get("OPENAI_CHAT_MODEL") or "gpt-4o-mini").strip()
    try:
        from openai import APIError, AuthenticationError, OpenAI

        client = OpenAI(api_key=key, timeout=120.0)
        user = user_raw
        if not user.strip():
            return ChatResult("Пустое сообщение.", note=None)

        system = _SYSTEM_CAMPAIGN if dialogue_context else _SYSTEM
        resp = client.chat.completions.create(
            model=model,
            messages=[
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            max_tokens=2048,
        )
        choice = resp.choices[0] if resp.choices else None
        text = (choice.message.content if choice and choice.message else None) or ""
        text = text.strip()
        if not text:
            return ChatResult(
                "(Модель вернула пустой ответ.)",
                note=f"model={model}",
            )
        return ChatResult(text, note=f"model={model}")
    except AuthenticationError:
        return ChatResult(
            "Ошибка 401: ключ OpenAI отклонён. Создайте новый ключ на "
            "https://platform.openai.com/api-keys , вставьте в .env как OPENAI_API_KEY=sk-... "
            "(без лишних кавычек и пробелов), перезапустите seo-os-telegram-bot или seo-os-api.",
            note=None,
        )
    except APIError as e:
        return ChatResult(
            f"Ошибка OpenAI API: {e}",
            note="Проверьте OPENAI_API_KEY, квоту и OPENAI_CHAT_MODEL.",
        )
    except Exception as e:
        return ChatResult(
            f"Ошибка: {e}",
            note="Неожиданная ошибка при вызове модели.",
        )
