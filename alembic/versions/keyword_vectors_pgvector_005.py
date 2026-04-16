"""keyword vectors: pgvector column and ANN index.

Revision ID: keyword_vectors_pgvector_005
Revises: keyword_vectors_004
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa

revision = "keyword_vectors_pgvector_005"
down_revision = "keyword_vectors_004"
branch_labels = None
depends_on = None


def upgrade() -> None:
    bind = op.get_bind()
    available = bool(
        bind.execute(
            sa.text("SELECT EXISTS(SELECT 1 FROM pg_available_extensions WHERE name = 'vector')")
        ).scalar()
    )
    if not available:
        return
    op.execute("CREATE EXTENSION IF NOT EXISTS vector")
    op.execute("ALTER TABLE keyword_vectors ADD COLUMN IF NOT EXISTS vector_pg vector(128)")
    op.execute(
        """
        CREATE INDEX IF NOT EXISTS ix_keyword_vectors_vector_pg_ivfflat
        ON keyword_vectors
        USING ivfflat (vector_pg vector_cosine_ops)
        WITH (lists = 100)
        """
    )


def downgrade() -> None:
    op.execute("DROP INDEX IF EXISTS ix_keyword_vectors_vector_pg_ivfflat")
    op.execute("ALTER TABLE keyword_vectors DROP COLUMN IF EXISTS vector_pg")
