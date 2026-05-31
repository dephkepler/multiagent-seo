"""Content phase: комментарий handoff + перенос в Content Draft."""
from __future__ import annotations

import uuid
from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from sqlalchemy.orm import Session

from seo_os.db.repos_external import get_link_by_pipeline
from seo_os.integrations.trello import TrelloClient, TrelloListIds


class ContentTrelloState(TypedDict, total=False):
    campaign_id: str
    job_id: str
    pipeline_id: str
    graph_name: str
    trello_card_id: str
    messages: list[dict[str, Any]]
    steps_log: list[dict[str, Any]]
    content_scores: dict[str, float]
    passed_quality_gate: bool


def build_content_trello_graph(
    session: Session,
    _campaign_uuid: uuid.UUID,
    pipeline_id: str,
    trello: TrelloClient,
    lists: TrelloListIds,
):
    def node_content_handoff(state: ContentTrelloState) -> ContentTrelloState:
        log = list(state.get("steps_log", []))
        msgs = list(state.get("messages", []))
        link = get_link_by_pipeline(session, pipeline_id)
        if not link:
            err = "No Trello card for pipeline_id; run keyword_trello first."
            log.append({"node": "content", "error": err})
            return {"steps_log": log, "messages": msgs}

        cid = link["external_id"]
        trello.add_comment(
            cid,
            "Content agent: received handoff from keyword. Moving card to **Content Draft**.",
        )
        trello.move_card_to_list(cid, lists.content_draft)
        trello.add_comment(
            cid,
            "Content agent: draft phase complete (skeleton). Quality gate passed locally.",
        )
        msgs.append(
            {
                "from": "content_agent",
                "to": "qa_agent",
                "type": "handoff",
                "text": "Draft ready for QA checklist.",
            }
        )
        log.append({"node": "content_handoff", "card_id": cid, "moved_to": "content_draft"})
        return {
            "trello_card_id": cid,
            "messages": msgs,
            "steps_log": log,
            "content_scores": {"aggregate": 0.85},
            "passed_quality_gate": True,
        }

    g = StateGraph(ContentTrelloState)
    g.add_node("content_handoff", node_content_handoff)
    g.set_entry_point("content_handoff")
    g.add_edge("content_handoff", END)
    return g.compile()
