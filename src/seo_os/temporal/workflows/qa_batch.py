from __future__ import annotations

from datetime import timedelta

from temporalio import workflow

from seo_os.temporal.shared_inputs import (
    BatchResult,
    QaBatchInput,
    RunLangGraphInput,
    RunLangGraphOutput,
)


@workflow.defn
class QaBatchWorkflow:
    @workflow.run
    async def run(self, inp: QaBatchInput) -> BatchResult:
        r: RunLangGraphOutput = await workflow.execute_activity(
            "run_langgraph",
            RunLangGraphInput(
                campaign_id=inp.campaign_id,
                job_id=inp.job_id,
                graph_name="qa_trello",
                pipeline_id=inp.pipeline_id,
            ),
            start_to_close_timeout=timedelta(minutes=5),
            result_type=RunLangGraphOutput,
        )
        return BatchResult(
            ok=r.ok,
            message=r.message,
            graph_name="qa_trello",
            content_score=r.content_score,
        )
