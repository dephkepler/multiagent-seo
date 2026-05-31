"""Показать id колонок доски Trello и (опционально) создать стандартные шесть списков."""
from __future__ import annotations

import argparse
import os
from typing import Any

from dotenv import load_dotenv

from seo_os.integrations.trello import TrelloClient, TrelloConfigError

# Имена колонок в пайплайне (слева направо при ручном создании / после reorder в UI).
PIPELINE_LISTS: list[tuple[str, str]] = [
    ("TRELLO_LIST_INBOX", "Inbox"),
    ("TRELLO_LIST_KEYWORD_RESEARCH", "Keyword Research"),
    ("TRELLO_LIST_CONTENT_DRAFT", "Content Draft"),
    ("TRELLO_LIST_QA", "QA"),
    ("TRELLO_LIST_DONE", "Done"),
    ("TRELLO_LIST_BLOCKED", "Blocked"),
]


def _norm(s: str) -> str:
    return " ".join(s.strip().lower().split())


def _match_list_id(
    lists: list[dict[str, Any]], canonical_name: str
) -> str | None:
    """Найти id списка по точному имени или без учёта регистра/лишних пробелов."""
    want = _norm(canonical_name)
    for row in lists:
        name = row.get("name") or ""
        if name.strip() == canonical_name.strip():
            return str(row["id"])
    for row in lists:
        name = row.get("name") or ""
        if _norm(name) == want:
            return str(row["id"])
    return None


def main() -> None:
    load_dotenv()
    p = argparse.ArgumentParser(
        description="Список колонок доски Trello + блок для .env; "
        "--bootstrap создаёт недостающие стандартные колонки.",
    )
    p.add_argument(
        "--board",
        default=None,
        help="Short link доски (иначе TRELLO_BOARD_SHORT_LINK, по умолчанию qPDyVJen).",
    )
    p.add_argument(
        "--bootstrap",
        action="store_true",
        help="Создать на доске колонки Inbox … Blocked, если их ещё нет.",
    )
    args = p.parse_args()

    board = (args.board or os.environ.get("TRELLO_BOARD_SHORT_LINK") or "qPDyVJen").strip()
    try:
        client = TrelloClient.from_env()
    except TrelloConfigError as e:
        raise SystemExit(str(e)) from e

    board_obj = client.get_board(board)
    board_id = str(board_obj["id"])
    lists = client.lists_for_board(board)

    if args.bootstrap:
        for _env_name, display_name in PIPELINE_LISTS:
            if _match_list_id(lists, display_name) is None:
                client.create_list(id_board=board_id, name=display_name, pos="bottom")
        lists = client.lists_for_board(board)

    print(f"Доска: {board_obj.get('name', '?')}  (shortLink={board}, id={board_id})\n")
    print("Колонки на доске (скопируйте id в нужную переменную .env по смыслу):")
    print(f"{'id':<26}  name")
    print("-" * 60)
    for row in sorted(lists, key=lambda x: float(x.get("pos") or 0)):
        lid = str(row.get("id", ""))
        name = str(row.get("name", ""))
        print(f"{lid:<26}  {name}")

    print("\n# --- вставка в .env (сопоставьте колонки с этапами пайплайна) ---\n")
    for env_name, display_name in PIPELINE_LISTS:
        mid = _match_list_id(lists, display_name)
        val = mid if mid else ""
        print(f"{env_name}={val}")
    missing = [dn for _e, dn in PIPELINE_LISTS if not _match_list_id(lists, dn)]
    if missing:
        print(
            f"\n# Не найдены колонки с именами: {', '.join(missing)}.\n"
            "# Переименуйте колонки на доске, задайте id вручную из таблицы выше, "
            "или запустите снова с --bootstrap.",
            end="",
        )
        if not args.bootstrap:
            print("  Пример: seo-os-trello-lists --bootstrap")
        else:
            print()
    else:
        print("\n# Все шесть стандартных колонок сопоставлены по имени.")


if __name__ == "__main__":
    main()
