"""keyword vectors: MVP semantic memory layer.

Revision ID: keyword_vectors_004
Revises: keyword_memory_003
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "keyword_vectors_004"
down_revision = "keyword_memory_003"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "keyword_vectors",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("normalized_keyword", sa.Text(), nullable=False),
        sa.Column("embedding_model", sa.Text(), nullable=False),
        sa.Column("embedding_dim", sa.Integer(), nullable=False),
        sa.Column("vector", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False),
    )
    op.create_index("uq_keyword_vectors_keyword", "keyword_vectors", ["normalized_keyword"], unique=True)
    op.create_index("ix_keyword_vectors_updated_at", "keyword_vectors", ["updated_at"])


def downgrade() -> None:
    op.drop_table("keyword_vectors")
