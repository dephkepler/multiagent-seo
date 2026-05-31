"""Запуск uvicorn для FastAPI (чат API)."""
from __future__ import annotations

import argparse
import os

from dotenv import load_dotenv


def main() -> None:
    load_dotenv()
    import uvicorn

    p = argparse.ArgumentParser(description="SEO OS: HTTP API (FastAPI + uvicorn).")
    p.add_argument("--host", default=os.environ.get("CHAT_API_HOST", "127.0.0.1"))
    p.add_argument("--port", type=int, default=int(os.environ.get("CHAT_API_PORT", "8000")))
    p.add_argument("--reload", action="store_true", help="Автоперезагрузка при изменении кода (dev).")
    args = p.parse_args()
    uvicorn.run(
        "seo_os.api.main:app",
        host=args.host,
        port=args.port,
        reload=args.reload,
    )


if __name__ == "__main__":
    main()
