"""Keyword phase: создать карточку Trello или использовать существующую по pipeline_id."""
from __future__ import annotations

import uuid
from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from sqlalchemy.orm import Session

from seo_os.db.repos_external import get_link_by_pipeline, upsert_trello_card
from seo_os.integrations.trello import TrelloClient, TrelloListIds


class KeywordTrelloState(TypedDict, total=False):
    campaign_id: str
    job_id: str
    pipeline_id: str
    graph_name: str
    trello_card_id: str
    trello_card_url: str | None
    handoff_note: str
    messages: list[dict[str, Any]]
    steps_log: list[dict[str, Any]]


def build_keyword_trello_graph(
    session: Session,
    campaign_uuid: uuid.UUID,
    pipeline_id: str,
    trello: TrelloClient,
    lists: TrelloListIds,
):
    def node_keyword_create(state: KeywordTrelloState) -> KeywordTrelloState:
        log = list(state.get("steps_log", []))
        msgs = list(state.get("messages", []))
        existing = get_link_by_pipeline(session, pipeline_id)
        if existing:
            cid = existing["external_id"]
            url = existing.get("external_url")
            log.append({"node": "ensure_card", "action": "reuse", "card_id": cid})
        else:
            name = f"SEO pipeline {pipeline_id[:8]}"
            desc = (
                f"pipeline_id={pipeline_id}\n"
                f"campaign_id={state.get('campaign_id')}\n"
                f"job_id={state.get('job_id')}\n"
            )
            card = trello.create_card(id_list=lists.keyword_research, name=name, desc=desc)
            cid = card["id"]
            url = card.get("shortUrl") or card.get("url")
            upsert_trello_card(
                session,
                campaign_id=campaign_uuid,
                pipeline_id=pipeline_id,
                external_id=cid,
                external_url=url,
            )
            log.append({"node": "ensure_card", "action": "create", "card_id": cid})

        trello.add_comment(
            cid,
            "Keyword agent: card ready in **Keyword Research**. (skeleton handoff)",
        )
        handoff = (
            "Handoff to Content: use stub cluster [example long tail, competitor gap] "
            "and angle «helpful overview»."
        )
        msgs.append(
            {
                "from": "keyword_agent",
                "to": "content_agent",
                "type": "handoff",
                "text": handoff,
            }
        )
        log.append({"node": "handoff", "handoff_note": handoff[:200]})
        return {
            "trello_card_id": cid,
            "trello_card_url": url,
            "handoff_note": handoff,
            "messages": msgs,
            "steps_log": log,
        }

    g = StateGraph(KeywordTrelloState)
    g.add_node("keyword_create", node_keyword_create)
    g.set_entry_point("keyword_create")
    g.add_edge("keyword_create", END)
    return g.compile()
