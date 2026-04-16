"""Запуск Telegram-бота (long polling)."""
from __future__ import annotations

import logging
import os
import sys

from dotenv import load_dotenv
from telegram import Update

from seo_os.bots.telegram_app import build_application


def main() -> None:
    logging.basicConfig(
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        level=logging.INFO,
    )
    load_dotenv()
    token = (os.environ.get("TELEGRAM_BOT_TOKEN") or "").strip()
    if not token:
        print(
            "Задайте TELEGRAM_BOT_TOKEN в .env (токен выдаёт @BotFather). "
            "Не коммитьте токен в репозиторий.",
            file=sys.stderr,
        )
        raise SystemExit(1)

    app = build_application(token)
    print("Telegram-бот запущен (polling). Ctrl+C — остановка.")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
