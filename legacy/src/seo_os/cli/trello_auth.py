"""Печать URL разового ручного получения TRELLO_TOKEN (не при каждом запросе к API)."""
from __future__ import annotations

import argparse
import os
from urllib.parse import urlencode

from dotenv import load_dotenv

AUTH_BASE = "https://trello.com/1/authorize"


def build_authorize_url(api_key: str, *, app_name: str = "SEO OS") -> str:
    q = urlencode(
        {
            "expiration": "never",
            "scope": "read,write",
            "response_type": "token",
            "name": app_name,
            "key": api_key,
        }
    )
    return f"{AUTH_BASE}?{q}"


def main() -> None:
    load_dotenv()
    p = argparse.ArgumentParser(
        description="Сформировать ссылку авторизации Trello и вывести инструкцию.",
    )
    p.add_argument(
        "--key",
        default=None,
        help="Trello API key (иначе из env TRELLO_API_KEY)",
    )
    p.add_argument(
        "--name",
        default="SEO OS",
        help="Имя приложения на экране Trello (параметр name)",
    )
    args = p.parse_args()
    key = (args.key or os.environ.get("TRELLO_API_KEY") or "").strip()
    if not key:
        raise SystemExit(
            "Укажите API key: --key=... или положите TRELLO_API_KEY в .env "
            "(ключ с https://trello.com/power-ups/admin)."
        )
    url = build_authorize_url(key, app_name=args.name)
    print("Один раз откройте ссылку в браузере (будучи залогиненным в Trello):")
    print()
    print(url)
    print()
    print(
        "После подтверждения Trello покажет **токен** на странице — скопируйте его в .env как TRELLO_TOKEN.\n"
        "С параметром expiration=never токен не протухает, пока вы его не отзовёте; "
        "worker при каждом запросе **переиспользует** этот тот же токен из .env, "
        "а не проходит авторизацию заново."
    )


if __name__ == "__main__":
    main()
