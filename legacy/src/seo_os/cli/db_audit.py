"""Печать последних строк external_task_links и agent_decisions."""
from __future__ import annotations

import argparse
import os
import sys

from dotenv import load_dotenv
from sqlalchemy import text

from seo_os.db.session import get_engine


def main() -> None:
    load_dotenv()
    if not (os.environ.get("DATABASE_URL") or "").strip():
        print("Задайте DATABASE_URL в .env", file=sys.stderr)
        raise SystemExit(1)

    p = argparse.ArgumentParser(description="Последние записи в external_task_links и agent_decisions")
    p.add_argument("--limit", type=int, default=20, help="Строк на таблицу (по умолчанию 20)")
    args = p.parse_args()
    lim = max(1, min(args.limit, 500))

    engine = get_engine()
    with engine.connect() as conn:
        print("=== external_task_links (последние по времени) ===\n")
        rows = conn.execute(
            text(
                """
                SELECT pipeline_id, system, external_id,
                       LEFT(COALESCE(external_url, ''), 60) AS url_preview,
                       created_at
                FROM external_task_links
                ORDER BY created_at DESC
                LIMIT :lim
                """
            ),
            {"lim": lim},
        ).fetchall()
        if not rows:
            print("(пусто)\n")
        else:
            for r in rows:
                print(f"  pipeline_id={r[0]}")
                print(f"    system={r[1]}  external_id={r[2]}")
                print(f"    url={r[3]}")
                print(f"    created_at={r[4]}")
                print()

        print("=== agent_decisions (последние по времени) ===\n")
        rows2 = conn.execute(
            text(
                """
                SELECT graph_name, node,
                       LEFT(COALESCE(output_summary, ''), 120) AS out_preview,
                       created_at
                FROM agent_decisions
                ORDER BY created_at DESC
                LIMIT :lim
                """
            ),
            {"lim": lim},
        ).fetchall()
        if not rows2:
            print("(пусто)\n")
        else:
            for r in rows2:
                print(f"  {r[0]} / {r[1]}")
                print(f"    {r[2]}")
                print(f"    created_at={r[3]}")
                print()


if __name__ == "__main__":
    main()
