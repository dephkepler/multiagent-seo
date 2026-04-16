"""keyword memory: persistent keyword/cluster snapshots.

Revision ID: keyword_memory_003
Revises: external_task_links_002
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "keyword_memory_003"
down_revision = "external_task_links_002"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "keyword_memory",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("normalized_keyword", sa.Text(), nullable=False),
        sa.Column("canonical_keyword", sa.Text(), nullable=False),
        sa.Column("first_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("seen_count", sa.Integer(), server_default="1", nullable=False),
        sa.Column("new_count", sa.Integer(), server_default="1", nullable=False),
        sa.Column("resurfaced_count", sa.Integer(), server_default="0", nullable=False),
        sa.Column("avg_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("best_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("last_source", sa.Text(), nullable=True),
        sa.Column("last_campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="SET NULL")),
        sa.Column("last_pipeline_id", sa.Text(), nullable=True),
        sa.Column("extra", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb"), nullable=False),
    )
    op.create_index("uq_keyword_memory_normalized", "keyword_memory", ["normalized_keyword"], unique=True)
    op.create_index("ix_keyword_memory_last_seen_at", "keyword_memory", ["last_seen_at"])

    op.create_table(
        "cluster_memory",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("cluster_key", sa.Text(), nullable=False),
        sa.Column("label", sa.Text(), nullable=False),
        sa.Column("first_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("seen_count", sa.Integer(), server_default="1", nullable=False),
        sa.Column("keyword_count", sa.Integer(), server_default="0", nullable=False),
        sa.Column("expected_size", sa.Integer(), server_default="50", nullable=False),
        sa.Column("saturation_score", sa.Numeric(8, 6), server_default="0", nullable=False),
        sa.Column("status", sa.Text(), server_default="active", nullable=False),
        sa.Column("alias_of", sa.Text(), nullable=True),
        sa.Column("last_campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="SET NULL")),
        sa.Column("last_pipeline_id", sa.Text(), nullable=True),
        sa.Column("extra", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb"), nullable=False),
    )
    op.create_index("uq_cluster_memory_key", "cluster_memory", ["cluster_key"], unique=True)
    op.create_index("ix_cluster_memory_last_seen_at", "cluster_memory", ["last_seen_at"])

    op.create_table(
        "keyword_cluster_links",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "keyword_memory_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("keyword_memory.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "cluster_memory_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("cluster_memory.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("first_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("seen_count", sa.Integer(), server_default="1", nullable=False),
        sa.Column("last_campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="SET NULL")),
        sa.Column("last_pipeline_id", sa.Text(), nullable=True),
    )
    op.create_index(
        "uq_keyword_cluster_links_pair",
        "keyword_cluster_links",
        ["keyword_memory_id", "cluster_memory_id"],
        unique=True,
    )

    op.create_table(
        "keyword_run_snapshots",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("pipeline_id", sa.Text(), nullable=False),
        sa.Column("job_id", sa.Text(), nullable=False),
        sa.Column("payload", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb"), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
    )
    op.create_index(
        "uq_keyword_run_snapshots_pipeline_job",
        "keyword_run_snapshots",
        ["pipeline_id", "job_id"],
        unique=True,
    )
    op.create_index("ix_keyword_run_snapshots_campaign", "keyword_run_snapshots", ["campaign_id", "created_at"])


def downgrade() -> None:
    op.drop_table("keyword_run_snapshots")
    op.drop_table("keyword_cluster_links")
    op.drop_table("cluster_memory")
    op.drop_table("keyword_memory")
