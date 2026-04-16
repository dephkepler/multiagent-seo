"""external_task_links: связь pipeline_id с внешними задачами (Trello).

Revision ID: external_task_links_002
Revises: wave1_initial
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "external_task_links_002"
down_revision = "wave1_initial"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "external_task_links",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "campaign_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("campaigns.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("pipeline_id", sa.Text(), nullable=False),
        sa.Column("system", sa.Text(), server_default="trello", nullable=False),
        sa.Column("external_id", sa.Text(), nullable=False),
        sa.Column("external_url", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
        sa.UniqueConstraint("pipeline_id", name="uq_external_task_links_pipeline_id"),
    )
    op.create_index("ix_external_task_links_campaign_id", "external_task_links", ["campaign_id"])


def downgrade() -> None:
    op.drop_table("external_task_links")
