"""In-memory привязка Telegram user → последняя запущенная кампания (шаг D)."""
from __future__ import annotations

import time
from dataclasses import dataclass, field


@dataclass
class UserCampaignContext:
    campaign_id: str
    workflow_id: str
    created_at: float = field(default_factory=time.time)


_sessions: dict[int, UserCampaignContext] = {}


def bind(user_id: int, campaign_id: str, workflow_id: str) -> None:
    _sessions[user_id] = UserCampaignContext(campaign_id=campaign_id, workflow_id=workflow_id)


def get_ctx(user_id: int) -> UserCampaignContext | None:
    return _sessions.get(user_id)


def unbind(user_id: int) -> None:
    _sessions.pop(user_id, None)
