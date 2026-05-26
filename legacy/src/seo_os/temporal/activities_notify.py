"""Activity: отправка сообщения в Telegram Bot API (отчёты о фазах workflow)."""
from __future__ import annotations

import os

import httpx
from temporalio import activity

from seo_os.temporal.shared_inputs import NotifyTelegramInput, NotifyTelegramOutput


@activity.defn(name="notify_telegram")
def notify_telegram(inp: NotifyTelegramInput) -> NotifyTelegramOutput:
    """Если нет chat_id и нет TELEGRAM_REPORT_CHAT_ID — пропуск."""
    token = (os.environ.get("TELEGRAM_BOT_TOKEN") or "").strip()
    chat = (inp.chat_id or "").strip() or (os.environ.get("TELEGRAM_REPORT_CHAT_ID") or "").strip()
    if not token or not chat:
        return NotifyTelegramOutput(ok=True, skipped=True, detail="no_token_or_chat")
    text = inp.build_message()
    url = f"https://api.telegram.org/bot{token}/sendMessage"
    try:
        with httpx.Client(timeout=30.0) as client:
            r = client.post(url, json={"chat_id": chat, "text": text})
            r.raise_for_status()
        return NotifyTelegramOutput(ok=True, skipped=False, detail=None)
    except Exception as e:
        activity.logger.exception("notify_telegram failed")
        return NotifyTelegramOutput(ok=False, skipped=False, detail=str(e)[:500])
