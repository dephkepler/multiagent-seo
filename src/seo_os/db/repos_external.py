"""Связь pipeline_id с внешними системами (Trello card id)."""
from __future__ import annotations

import uuid
from typing import Any

from sqlalchemy import text
from sqlalchemy.orm import Session


def get_link_by_pipeline(
    session: Session, pipeline_id: str, system: str = "trello"
) -> dict[str, Any] | None:
    row = session.execute(
        text(
            """
            SELECT id, campaign_id, pipeline_id, system, external_id, external_url
            FROM external_task_links
            WHERE pipeline_id = :pid AND system = :sys
            """
        ),
        {"pid": pipeline_id, "sys": system},
    ).mappings().first()
    return dict(row) if row else None


def upsert_trello_card(
    session: Session,
    *,
    campaign_id: uuid.UUID,
    pipeline_id: str,
    external_id: str,
    external_url: str | None,
) -> None:
    session.execute(
        text(
            """
            INSERT INTO external_task_links (
                id, campaign_id, pipeline_id, system, external_id, external_url
            ) VALUES (
                gen_random_uuid(), :campaign_id, :pipeline_id, 'trello', :external_id, :external_url
            )
            ON CONFLICT (pipeline_id) DO UPDATE SET
                external_id = EXCLUDED.external_id,
                external_url = EXCLUDED.external_url,
                updated_at = now()
            """
        ),
        {
            "campaign_id": campaign_id,
            "pipeline_id": pipeline_id,
            "external_id": external_id,
            "external_url": external_url,
        },
    )


__all__ = ["get_link_by_pipeline", "upsert_trello_card"]
