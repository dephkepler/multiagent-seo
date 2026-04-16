"""Минимальный keyword-граф: discover → shortlist (без внешних API)."""
from __future__ import annotations

from typing import TypedDict

from langgraph.graph import END, StateGraph

from seo_os.decision_engine.content_quality import evaluate_content_quality


class KeywordState(TypedDict, total=False):
    campaign_id: str
    job_id: str
    graph_name: str
    candidates: list[str]
    steps_log: list[dict]


def _discover(state: KeywordState) -> KeywordState:
    cands = ["example long tail query one", "competitor gap stub two"]
    log = list(state.get("steps_log", []))
    log.append({"node": "discover", "out": f"{len(cands)} candidates"})
    return {"candidates": cands, "steps_log": log}


def _score_stub(state: KeywordState) -> KeywordState:
    """Используем тот же evaluate, что и для текста — заглушка."""
    text = " ".join(state.get("candidates", []))
    scores = evaluate_content_quality(text)
    log = list(state.get("steps_log", []))
    log.append({"node": "score_stub", "aggregate": scores["aggregate"]})
    return {"steps_log": log}


def build_keyword_skeleton_graph():
    g = StateGraph(KeywordState)
    g.add_node("discover", _discover)
    g.add_node("score_stub", _score_stub)
    g.set_entry_point("discover")
    g.add_edge("discover", "score_stub")
    g.add_edge("score_stub", END)
    return g.compile()
