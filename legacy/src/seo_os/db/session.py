"""Синхронный engine для activities (Temporal worker)."""
from __future__ import annotations

import os
from contextlib import contextmanager
from functools import lru_cache

from sqlalchemy import create_engine
from sqlalchemy.engine import Engine
from sqlalchemy.orm import Session, sessionmaker


@lru_cache
def get_engine() -> Engine:
    url = os.environ.get("DATABASE_URL")
    if not url:
        raise RuntimeError("Set DATABASE_URL (e.g. postgresql+psycopg://user:pass@localhost:5432/seo_os)")
    return create_engine(url, pool_pre_ping=True)


@contextmanager
def session_begin():
    factory = sessionmaker(bind=get_engine(), expire_on_commit=False)
    session = factory()
    try:
        with session.begin():
            yield session
    finally:
        session.close()


__all__ = ["get_engine", "session_begin"]
