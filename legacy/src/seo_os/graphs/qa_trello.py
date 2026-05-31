"""QA phase: In QA → Done + финальный комментарий."""
from __future__ import annotations

import uuid
from typing import Any, TypedDict

from langgraph.graph import END, StateGraph
from sqlalchemy.orm import Session

from seo_os.db.repos_external import get_link_by_pipeline
from seo_os.integrations.trello import TrelloClient, TrelloListIds


class QaTrelloState(TypedDict, total=False):
    campaign_id: str
    job_id: str
    pipeline_id: str
    graph_name: str
    messages: list[dict[str, Any]]
    steps_log: list[dict[str, Any]]


def build_qa_trello_graph(
    session: Session,
    _campaign_uuid: uuid.UUID,
    pipeline_id: str,
    trello: TrelloClient,
    lists: TrelloListIds,
):
    def node_qa_done(state: QaTrelloState) -> QaTrelloState:
        log = list(state.get("steps_log", []))
        msgs = list(state.get("messages", []))
        link = get_link_by_pipeline(session, pipeline_id)
        if not link:
            log.append({"node": "qa", "error": "No Trello card for pipeline_id."})
            return {"steps_log": log, "messages": msgs}

        cid = link["external_id"]
        trello.move_card_to_list(cid, lists.qa)
        trello.add_comment(cid, "QA agent: card in **QA** list (skeleton checks).")
        trello.move_card_to_list(cid, lists.done)
        trello.add_comment(
            cid,
            "QA agent: pipeline finished. Card moved to **Done**.",
        )
        msgs.append(
            {
                "from": "qa_agent",
                "to": "done",
                "type": "complete",
                "text": "All skeleton steps completed.",
            }
        )
        log.append({"node": "qa_done", "card_id": cid, "final_list": "done"})
        return {"messages": msgs, "steps_log": log}

    g = StateGraph(QaTrelloState)
    g.add_node("qa_done", node_qa_done)
    g.set_entry_point("qa_done")
    g.add_edge("qa_done", END)
    return g.compile()
