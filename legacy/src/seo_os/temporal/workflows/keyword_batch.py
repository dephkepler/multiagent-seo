from __future__ import annotations

from datetime import timedelta
import json

from temporalio import workflow

from seo_os.temporal.shared_inputs import (
    BatchResult,
    KeywordBatchInput,
    KeywordSearchInput,
    KeywordSearchOutput,
    RunLangGraphInput,
    RunLangGraphOutput,
)


@workflow.defn
class KeywordBatchWorkflow:
    @workflow.run
    async def run(self, inp: KeywordBatchInput) -> BatchResult:
        r: RunLangGraphOutput = await workflow.execute_activity(
            "run_langgraph",
            RunLangGraphInput(
                campaign_id=inp.campaign_id,
                job_id=inp.job_id,
                graph_name="keyword_trello",
                pipeline_id=inp.pipeline_id,
            ),
            start_to_close_timeout=timedelta(minutes=5),
            result_type=RunLangGraphOutput,
        )
        ks: KeywordSearchOutput = await workflow.execute_activity(
            "keyword_search",
            KeywordSearchInput(
                campaign_id=inp.campaign_id,
                job_id=inp.job_id,
                pipeline_id=inp.pipeline_id,
                query=inp.keyword_query,
                seeds=inp.keyword_seeds,
                limit=inp.keyword_limit,
                mode=inp.keyword_mode,
                sources=inp.keyword_sources,
                source_profile=inp.keyword_source_profile,
                source_timeouts=inp.keyword_source_timeouts,
                source_budgets=inp.keyword_source_budgets,
                merge_mode=inp.keyword_merge_mode,
                topic_cap=max(1, int(inp.keyword_topic_cap or 2)),
                adapter_route=inp.keyword_adapter_route,
                hl=inp.keyword_hl,
                gl=inp.keyword_gl,
                trends_config_path=inp.keyword_trends_config_path,
                trends_subprocess_timeout_sec=inp.keyword_trends_subprocess_timeout_sec,
                trends_filter_by_query=inp.keyword_trends_filter_by_query,
                trends_seed_mode=inp.keyword_trends_seed_mode,
                youtube_region_code=inp.keyword_youtube_region_code,
                youtube_relevance_language=inp.keyword_youtube_relevance_language,
                youtube_max_results=inp.keyword_youtube_max_results,
                include_raw_ref=inp.keyword_include_raw_ref,
            ),
            start_to_close_timeout=timedelta(
                minutes=8 if (inp.keyword_adapter_route or "").strip().lower() == "live_trends" else 5
            ),
            result_type=KeywordSearchOutput,
        )
        msg = r.message
        if ks.ok and not ks.skipped and (ks.message or "").strip():
            msg = f"{msg}\n\n--- keyword adapter ---\n{ks.message.strip()}"
            if (ks.detail or "").strip():
                detail_text = ks.detail.strip()
                try:
                    parsed = json.loads(detail_text)
                    stats = parsed.get("stats") if isinstance(parsed, dict) else None
                    if isinstance(stats, dict):
                        source_summary = stats.get("source_summary") or {}
                        cluster_keywords = stats.get("cluster_keywords") or {}
                        memory = parsed.get("memory") if isinstance(parsed, dict) else None
                        lines: list[str] = []
                        lines.append("source_effectiveness:")
                        for src, val in source_summary.items():
                            if isinstance(val, dict):
                                lines.append(
                                    f"- {src}: keywords={val.get('keywords', 0)}, clusters={val.get('clusters', 0)}"
                                )
                        lines.append("")
                        lines.append("clusters_with_keywords:")
                        shown = 0
                        for cluster, kws in cluster_keywords.items():
                            sample = ", ".join(list(kws or [])[:5])
                            lines.append(f"- {cluster}: {sample}")
                            shown += 1
                            if shown >= 12:
                                break
                        if isinstance(memory, dict):
                            lines.append("")
                            lines.append(
                                "memory_persistence: "
                                f"ok={memory.get('ok')}, "
                                f"keywords_saved={memory.get('keywords_saved', 0)}, "
                                f"clusters_saved={memory.get('clusters_saved', 0)}, "
                                f"vectors_saved={memory.get('vectors_saved', 0)}"
                            )
                            if memory.get("error"):
                                lines.append(f"memory_error={memory.get('error')}")
                        detail_text = "\n".join(lines).strip()
                except Exception:
                    pass
                msg += f"\n\n--- keyword stats ---\n{detail_text}"
        elif not ks.ok and ks.detail:
            msg = f"{msg}\n\n--- keyword adapter error ---\n{ks.detail}"
        return BatchResult(
            ok=r.ok,
            message=msg,
            graph_name="keyword_trello",
            content_score=r.content_score,
            keyword_count=ks.keyword_count,
            cluster_count_estimate=ks.cluster_count_estimate,
        )
