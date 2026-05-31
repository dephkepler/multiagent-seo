"""Activities: run_langgraph + БД (не импортировать из workflow-модулей тяжёлый код в sandbox)."""
from __future__ import annotations

import json
import uuid
from decimal import Decimal

from temporalio import activity

from seo_os.db.repos import (
    BudgetExceededError,
    assert_budget_allows_spend,
    insert_agent_decision,
    record_llm_cost,
)
from seo_os.db.session import session_begin
from seo_os.graphs.content_skeleton import build_content_skeleton_graph
from seo_os.graphs.content_trello import build_content_trello_graph
from seo_os.graphs.keyword_skeleton import build_keyword_skeleton_graph
from seo_os.graphs.keyword_trello import build_keyword_trello_graph
from seo_os.graphs.qa_trello import build_qa_trello_graph
from seo_os.integrations.trello import TrelloClient, TrelloConfigError, TrelloListIds
from seo_os.temporal.shared_inputs import RunLangGraphInput, RunLangGraphOutput

_ESTIMATED_LLM_USD = Decimal("0.02")

_SKELETON_GRAPHS = {
    "keyword_skeleton": build_keyword_skeleton_graph,
    "content_skeleton": build_content_skeleton_graph,
}

_TRELLO_GRAPH_BUILDERS = {
    "keyword_trello": build_keyword_trello_graph,
    "content_trello": build_content_trello_graph,
    "qa_trello": build_qa_trello_graph,
}


@activity.defn(name="run_langgraph")
def run_langgraph(inp: RunLangGraphInput) -> RunLangGraphOutput:
    campaign_uuid = uuid.UUID(inp.campaign_id)
    job_uuid = uuid.UUID(inp.job_id)

    if inp.graph_name in _TRELLO_GRAPH_BUILDERS:
        if not (inp.pipeline_id or "").strip():
            return RunLangGraphOutput(ok=False, message="pipeline_id is required for Trello graphs")
        try:
            trello = TrelloClient.from_env()
            lists = TrelloListIds.from_env()
        except TrelloConfigError as e:
            return RunLangGraphOutput(ok=False, message=str(e))

        with session_begin() as session:
            try:
                assert_budget_allows_spend(session, campaign_uuid, _ESTIMATED_LLM_USD)
            except BudgetExceededError as e:
                return RunLangGraphOutput(ok=False, message=str(e))

            builder = _TRELLO_GRAPH_BUILDERS[inp.graph_name]
            graph = builder(session, campaign_uuid, inp.pipeline_id.strip(), trello, lists)
            initial: dict = {
                "campaign_id": inp.campaign_id,
                "job_id": inp.job_id,
                "pipeline_id": inp.pipeline_id.strip(),
                "graph_name": inp.graph_name,
                "steps_log": [],
                "messages": [],
            }
            out = graph.invoke(initial)
            steps = out.get("steps_log") or []

            for step in steps:
                insert_agent_decision(
                    session,
                    job_id=job_uuid,
                    campaign_id=campaign_uuid,
                    graph_name=inp.graph_name,
                    node=str(step.get("node", "unknown")),
                    input_summary=None,
                    output_summary=json.dumps(step, ensure_ascii=False)[:8000],
                    tool_calls=None,
                    latency_ms=None,
                )

        msg = (
            f"graph={inp.graph_name} steps={len(steps)} pipeline={inp.pipeline_id[:8]}… "
            f"messages={len(out.get('messages') or [])}"
        )
        passed = out.get("passed_quality_gate")
        scores = out.get("content_scores") or {}
        agg = scores.get("aggregate") if isinstance(scores, dict) else None
        return RunLangGraphOutput(
            ok=True,
            message=msg,
            passed_quality_gate=passed if inp.graph_name == "content_trello" else None,
            content_score=float(agg) if agg is not None else None,
        )

    if inp.graph_name not in _SKELETON_GRAPHS:
        return RunLangGraphOutput(ok=False, message=f"Unknown graph: {inp.graph_name}")

    with session_begin() as session:
        try:
            assert_budget_allows_spend(session, campaign_uuid, _ESTIMATED_LLM_USD)
        except BudgetExceededError as e:
            return RunLangGraphOutput(ok=False, message=str(e))

        builder = _SKELETON_GRAPHS[inp.graph_name]
        graph = builder()
        initial: dict = {
            "campaign_id": inp.campaign_id,
            "job_id": inp.job_id,
            "graph_name": inp.graph_name,
            "steps_log": [],
        }
        out = graph.invoke(initial)
        steps = out.get("steps_log") or []

        for step in steps:
            insert_agent_decision(
                session,
                job_id=job_uuid,
                campaign_id=campaign_uuid,
                graph_name=inp.graph_name,
                node=str(step.get("node", "unknown")),
                input_summary=None,
                output_summary=json.dumps(step, ensure_ascii=False)[:8000],
                tool_calls=None,
                latency_ms=None,
            )

        idem = f"{inp.job_id}:{inp.graph_name}:skeleton"
        record_llm_cost(
            session,
            campaign_id=campaign_uuid,
            amount_usd=_ESTIMATED_LLM_USD,
            operation_type=f"langgraph:{inp.graph_name}",
            idempotency_key=idem,
        )

    passed = out.get("passed_quality_gate")
    scores = out.get("content_scores") or {}
    agg = scores.get("aggregate") if isinstance(scores, dict) else None

    msg = f"graph={inp.graph_name} steps={len(steps)}"
    if inp.graph_name == "content_skeleton":
        msg += f" quality_gate={passed} aggregate={agg}"

    return RunLangGraphOutput(
        ok=True,
        message=msg,
        passed_quality_gate=passed if inp.graph_name == "content_skeleton" else None,
        content_score=float(agg) if agg is not None else None,
    )
