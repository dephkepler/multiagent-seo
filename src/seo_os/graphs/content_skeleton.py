"""Черновик → quality gate (decision_engine)."""
from __future__ import annotations

from typing import TypedDict

from langgraph.graph import END, StateGraph

from seo_os.decision_engine.content_quality import (
    evaluate_content_quality,
    passed_quality_gate,
)


class ContentState(TypedDict, total=False):
    campaign_id: str
    job_id: str
    graph_name: str
    draft: str
    content_scores: dict[str, float]
    passed_quality_gate: bool
    steps_log: list[dict]


def _draft(state: ContentState) -> ContentState:
    # Достаточная длина для заглушки evaluate_content_quality (aggregate >= 0.75)
    body = (
        "This is a skeleton draft for SEO content in the SEO OS pipeline. "
        "It demonstrates LangGraph nodes, Temporal activities, and the decision_engine "
        "quality gate before publishing. The paragraph is intentionally verbose so that "
        "the completeness heuristic reaches a passing score without calling an LLM."
    ) * 3
    log = list(state.get("steps_log", []))
    log.append({"node": "draft", "out_preview": body[:80]})
    return {"draft": body, "steps_log": log}


def _quality(state: ContentState) -> ContentState:
    scores = evaluate_content_quality(state.get("draft", ""))
    ok = passed_quality_gate(scores)
    log = list(state.get("steps_log", []))
    log.append({"node": "quality", "aggregate": scores["aggregate"], "passed": ok})
    return {
        "content_scores": scores,
        "passed_quality_gate": ok,
        "steps_log": log,
    }


def build_content_skeleton_graph():
    g = StateGraph(ContentState)
    g.add_node("draft", _draft)
    g.add_node("quality", _quality)
    g.set_entry_point("draft")
    g.add_edge("draft", "quality")
    g.add_edge("quality", END)
    return g.compile()
