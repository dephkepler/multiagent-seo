"""Python Enums зеркалят PostgreSQL ENUM из wave1_initial (Alembic)."""
from __future__ import annotations

from enum import Enum


class CampaignStatus(str, Enum):
    DRAFT = "draft"
    RUNNING = "running"
    PAUSED = "paused"
    COMPLETED = "completed"
    CANCELLED = "cancelled"


class KeywordStatus(str, Enum):
    DISCOVERED = "discovered"
    SCORED = "scored"
    SHORTLISTED = "shortlisted"
    APPROVED_FOR_CONTENT = "approved_for_content"
    REJECTED = "rejected"
    PUBLISHED = "published"
    ARCHIVED = "archived"


class DraftStatus(str, Enum):
    BRIEF_PENDING = "brief_pending"
    WRITING = "writing"
    EDITING = "editing"
    QUALITY_GATE = "quality_gate"
    HUMAN_REVIEW = "human_review"
    APPROVED = "approved"
    REJECTED = "rejected"


class PageStatus(str, Enum):
    NOT_PUBLISHED = "not_published"
    LIVE = "live"
    PUBLISH_FAILED = "publish_failed"
    REPUBLISH_PENDING = "republish_pending"
    TAKEN_DOWN = "taken_down"


class IndexStatus(str, Enum):
    UNKNOWN = "unknown"
    NOT_INDEXED = "not_indexed"
    INDEXED = "indexed"
    ERROR = "error"
    EXCLUDED = "excluded"


class ApprovalStatus(str, Enum):
    PENDING = "pending"
    APPROVED = "approved"
    REJECTED = "rejected"
    SUPERSEDED = "superseded"
    EXPIRED = "expired"
