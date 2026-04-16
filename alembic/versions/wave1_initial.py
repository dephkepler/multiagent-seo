"""Wave 1: lifecycle enums + core tables (SEO OS).

Revision ID: wave1_initial
Revises:
Create Date: 2026-04-11

Append-only semantics (documented): workflow_events, agent_decisions, cost_tracking rows are insert-only.
Mutable: campaigns, sites, keyword_candidates (status), article_drafts, published_pages, approvals (until decided),
campaign_budgets (spent), keyword_scores new rows per scoring_version.
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "wave1_initial"
down_revision = None
branch_labels = None
depends_on = None


def _create_enums(bind) -> None:
    enums = [
        (
            "campaign_status",
            ("draft", "running", "paused", "completed", "cancelled"),
        ),
        (
            "keyword_status",
            (
                "discovered",
                "scored",
                "shortlisted",
                "approved_for_content",
                "rejected",
                "published",
                "archived",
            ),
        ),
        (
            "draft_status",
            (
                "brief_pending",
                "writing",
                "editing",
                "quality_gate",
                "human_review",
                "approved",
                "rejected",
            ),
        ),
        (
            "page_status",
            (
                "not_published",
                "live",
                "publish_failed",
                "republish_pending",
                "taken_down",
            ),
        ),
        (
            "index_status",
            ("unknown", "not_indexed", "indexed", "error", "excluded"),
        ),
        (
            "approval_status",
            ("pending", "approved", "rejected", "superseded", "expired"),
        ),
    ]
    for name, values in enums:
        e = postgresql.ENUM(*values, name=name)
        e.create(bind, checkfirst=True)


def _drop_enums() -> None:
    """Таблицы уже удалены — типы ENUM снимаем явно."""
    for name in (
        "approval_status",
        "index_status",
        "page_status",
        "draft_status",
        "keyword_status",
        "campaign_status",
    ):
        op.execute(sa.text(f'DROP TYPE IF EXISTS "{name}"'))


def upgrade() -> None:
    bind = op.get_bind()
    _create_enums(bind)

    # postgresql.ENUM + create_type=False: типы уже созданы в _create_enums (иначе Alembic дублирует CREATE TYPE)
    campaign_status = postgresql.ENUM(
        "draft",
        "running",
        "paused",
        "completed",
        "cancelled",
        name="campaign_status",
        create_type=False,
    )
    keyword_status = postgresql.ENUM(
        "discovered",
        "scored",
        "shortlisted",
        "approved_for_content",
        "rejected",
        "published",
        "archived",
        name="keyword_status",
        create_type=False,
    )
    draft_status = postgresql.ENUM(
        "brief_pending",
        "writing",
        "editing",
        "quality_gate",
        "human_review",
        "approved",
        "rejected",
        name="draft_status",
        create_type=False,
    )
    page_status = postgresql.ENUM(
        "not_published",
        "live",
        "publish_failed",
        "republish_pending",
        "taken_down",
        name="page_status",
        create_type=False,
    )
    index_status = postgresql.ENUM(
        "unknown",
        "not_indexed",
        "indexed",
        "error",
        "excluded",
        name="index_status",
        create_type=False,
    )
    approval_status = postgresql.ENUM(
        "pending",
        "approved",
        "rejected",
        "superseded",
        "expired",
        name="approval_status",
        create_type=False,
    )

    op.create_table(
        "sites",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("name", sa.Text(), nullable=False),
        sa.Column("base_url", sa.Text(), nullable=False),
        sa.Column("settings", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
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
    )

    op.create_table(
        "campaigns",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("site_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("sites.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.Text(), nullable=False),
        sa.Column("status", campaign_status, server_default="draft", nullable=False),
        sa.Column("vertical", sa.Text(), nullable=True),
        sa.Column("temporal_workflow_id", sa.Text(), nullable=True),
        sa.Column("temporal_run_id", sa.Text(), nullable=True),
        sa.Column("config", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
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
    )
    op.create_index("ix_campaigns_site_id", "campaigns", ["site_id"])
    op.create_index("ix_campaigns_status", "campaigns", ["status"])

    op.create_table(
        "keyword_candidates",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("site_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("sites.id", ondelete="CASCADE"), nullable=False),
        sa.Column("query", sa.Text(), nullable=False),
        sa.Column("normalized_query", sa.Text(), nullable=True),
        sa.Column("status", keyword_status, server_default="discovered", nullable=False),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
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
    )
    op.create_index("ix_keyword_candidates_campaign_id", "keyword_candidates", ["campaign_id"])
    op.create_index("ix_keyword_candidates_site_id", "keyword_candidates", ["site_id"])

    op.create_table(
        "keyword_scores",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "keyword_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("keyword_candidates.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("demand_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("serp_weakness_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("intent_gap_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("authority_fit_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("risk_penalty", sa.Numeric(8, 6), nullable=True),
        sa.Column("final_score", sa.Numeric(8, 6), nullable=True),
        sa.Column("scoring_version", sa.Text(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
        sa.UniqueConstraint("keyword_id", "scoring_version", name="uq_keyword_scores_keyword_version"),
    )
    op.create_index("ix_keyword_scores_keyword_id", "keyword_scores", ["keyword_id"])

    op.create_table(
        "content_briefs",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("keyword_candidate_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("keyword_candidates.id", ondelete="SET NULL"), nullable=True),
        sa.Column("title", sa.Text(), nullable=True),
        sa.Column("brief_json", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb"), nullable=False),
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
    )
    op.create_index("ix_content_briefs_campaign_id", "content_briefs", ["campaign_id"])

    op.create_table(
        "article_drafts",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("content_brief_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("content_briefs.id", ondelete="CASCADE"), nullable=False),
        sa.Column("status", draft_status, server_default="brief_pending", nullable=False),
        sa.Column("body", sa.Text(), nullable=True),
        sa.Column("generated_by", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
        sa.Column("content_scores", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
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
    )
    op.create_index("ix_article_drafts_campaign_id", "article_drafts", ["campaign_id"])
    op.create_index("ix_article_drafts_brief_id", "article_drafts", ["content_brief_id"])

    op.create_table(
        "prompt_versions",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("agent_name", sa.Text(), nullable=False),
        sa.Column("version", sa.Text(), nullable=False),
        sa.Column("content_hash", sa.Text(), nullable=False),
        sa.Column("rollout_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("git_ref", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
        sa.UniqueConstraint("agent_name", "version", name="uq_prompt_versions_agent_version"),
    )

    op.create_table(
        "published_pages",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "article_draft_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("article_drafts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("site_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("sites.id", ondelete="CASCADE"), nullable=False),
        sa.Column("url", sa.Text(), nullable=False),
        sa.Column("status", page_status, server_default="not_published", nullable=False),
        sa.Column("index_status", index_status, server_default="unknown", nullable=False),
        sa.Column("published_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
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
    )
    op.create_index("ix_published_pages_site_id", "published_pages", ["site_id"])
    op.create_index("ix_published_pages_url", "published_pages", ["url"])
    op.create_index("ix_published_pages_draft_id", "published_pages", ["article_draft_id"])

    op.create_table(
        "qa_checks",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "published_page_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("published_pages.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("check_type", sa.Text(), nullable=False),
        sa.Column("passed", sa.Boolean(), nullable=False),
        sa.Column("details", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
    )
    op.create_index("ix_qa_checks_page_created", "qa_checks", ["published_page_id", "created_at"])

    op.create_table(
        "approvals",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("subject_type", sa.Text(), nullable=False),
        sa.Column("subject_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("approval_kind", sa.Text(), server_default="default", nullable=False),
        sa.Column("status", approval_status, server_default="pending", nullable=False),
        sa.Column("actor", sa.Text(), nullable=True),
        sa.Column("decided_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("notes", sa.Text(), nullable=True),
        sa.Column("idempotency_key", sa.Text(), nullable=True),
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
        sa.UniqueConstraint("idempotency_key", name="uq_approvals_idempotency_key"),
    )
    op.create_index("ix_approvals_campaign_subject", "approvals", ["campaign_id", "subject_type", "subject_id"])

    op.create_table(
        "workflow_events",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="SET NULL"), nullable=True),
        sa.Column("temporal_workflow_id", sa.Text(), nullable=True),
        sa.Column("temporal_run_id", sa.Text(), nullable=True),
        sa.Column("event_type", sa.Text(), nullable=False),
        sa.Column("payload", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb"), nullable=False),
        sa.Column("correlation_id", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
    )
    op.create_index("ix_workflow_events_campaign_created", "workflow_events", ["campaign_id", "created_at"])

    op.create_table(
        "agent_decisions",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("job_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("graph_name", sa.Text(), nullable=False),
        sa.Column("node", sa.Text(), nullable=False),
        sa.Column("input_summary", sa.Text(), nullable=True),
        sa.Column("output_summary", sa.Text(), nullable=True),
        sa.Column("tool_calls", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'[]'::jsonb")),
        sa.Column("latency_ms", sa.Integer(), nullable=True),
        sa.Column(
            "prompt_version_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("prompt_versions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
    )
    op.create_index("ix_agent_decisions_job", "agent_decisions", ["job_id", "created_at"])
    op.create_index("ix_agent_decisions_campaign", "agent_decisions", ["campaign_id", "created_at"])

    op.create_table(
        "cost_tracking",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column("campaign_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("campaigns.id", ondelete="CASCADE"), nullable=False),
        sa.Column("keyword_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("keyword_candidates.id", ondelete="SET NULL"), nullable=True),
        sa.Column("article_draft_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("article_drafts.id", ondelete="SET NULL"), nullable=True),
        sa.Column("operation_type", sa.Text(), nullable=False),
        sa.Column("amount", sa.Numeric(18, 6), nullable=False),
        sa.Column("currency", sa.Text(), server_default="USD", nullable=False),
        sa.Column("provider", sa.Text(), nullable=True),
        sa.Column("idempotency_key", sa.Text(), nullable=True),
        sa.Column("extra", postgresql.JSONB(astext_type=sa.Text()), server_default=sa.text("'{}'::jsonb")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.text("now()"),
            nullable=False,
        ),
        sa.UniqueConstraint("idempotency_key", name="uq_cost_tracking_idempotency_key"),
    )
    op.create_index("ix_cost_tracking_campaign", "cost_tracking", ["campaign_id", "created_at"])

    op.create_table(
        "campaign_budgets",
        sa.Column(
            "campaign_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("campaigns.id", ondelete="CASCADE"),
            primary_key=True,
        ),
        sa.Column("budget_limit", sa.Numeric(18, 2), nullable=True),
        sa.Column("currency", sa.Text(), server_default="USD", nullable=False),
        sa.Column("spent", sa.Numeric(18, 6), server_default="0", nullable=False),
        sa.Column("soft_stop_threshold", sa.Numeric(18, 2), nullable=True),
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
    )


def downgrade() -> None:
    op.drop_table("campaign_budgets")
    op.drop_table("cost_tracking")
    op.drop_table("agent_decisions")
    op.drop_table("workflow_events")
    op.drop_table("approvals")
    op.drop_table("qa_checks")
    op.drop_table("published_pages")
    op.drop_table("article_drafts")
    op.drop_table("content_briefs")
    op.drop_table("keyword_scores")
    op.drop_table("keyword_candidates")
    op.drop_table("prompt_versions")
    op.drop_table("campaigns")
    op.drop_table("sites")
    _drop_enums()
