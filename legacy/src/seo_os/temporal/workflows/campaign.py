from __future__ import annotations

from datetime import timedelta
import re

from temporalio import workflow

from seo_os.temporal.shared_inputs import (
    BatchResult,
    CampaignInput,
    CampaignResult,
    ContentBatchInput,
    KeywordBatchInput,
    KeywordBootstrapInput,
    KeywordBootstrapOutput,
    KeywordCluster,
    KeywordRefineInput,
    KeywordRefineOutput,
    NotifyTrelloCardInput,
    NotifyTrelloCardOutput,
    NotifyTelegramInput,
    NotifyTelegramOutput,
    QaBatchInput,
)
from seo_os.temporal.workflows.content_batch import ContentBatchWorkflow
from seo_os.temporal.workflows.keyword_batch import KeywordBatchWorkflow
from seo_os.temporal.workflows.qa_batch import QaBatchWorkflow


def _batch_detail(br: BatchResult) -> str:
    extra = ""
    if br.keyword_count is not None:
        extra += f"\nkeyword_count={br.keyword_count}"
    if br.cluster_count_estimate is not None:
        extra += f"\ncluster_count_estimate={br.cluster_count_estimate}"
    return f"ok={br.ok}\ngraph={br.graph_name}{extra}\n{br.message}"


_KW_LINE_RE = re.compile(r"^- (.+?) \[[^\]]+\]")
_SRC_LINE_RE = re.compile(r"^- ([a-z0-9_]+): keywords=(\d+), clusters=(\d+)$", re.IGNORECASE)
_CLUSTER_LINE_RE = re.compile(r"^- (.+?):\s*(.*)$")


def _extract_keywords_from_message(message: str, *, limit: int = 80) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for line in (message or "").splitlines():
        m = _KW_LINE_RE.match(line.strip())
        if not m:
            continue
        kw = m.group(1).strip()
        key = kw.lower()
        if not kw or key in seen:
            continue
        seen.add(key)
        out.append(kw)
        if len(out) >= limit:
            break
    return out


def _parse_keyword_stats(message: str) -> tuple[dict[str, tuple[int, int]], dict[str, list[str]]]:
    source_stats: dict[str, tuple[int, int]] = {}
    cluster_stats: dict[str, list[str]] = {}
    in_stats = False
    in_source = False
    in_cluster = False
    for raw in (message or "").splitlines():
        line = raw.strip()
        if line == "--- keyword stats ---":
            in_stats = True
            in_source = False
            in_cluster = False
            continue
        if not in_stats:
            continue
        if line.lower() == "source_effectiveness:":
            in_source = True
            in_cluster = False
            continue
        if line.lower() == "clusters_with_keywords:":
            in_source = False
            in_cluster = True
            continue
        if not line:
            continue
        if in_source:
            m = _SRC_LINE_RE.match(line)
            if not m:
                continue
            src = m.group(1).lower()
            source_stats[src] = (int(m.group(2)), int(m.group(3)))
        elif in_cluster:
            m = _CLUSTER_LINE_RE.match(line)
            if not m:
                continue
            cluster = m.group(1).strip()
            kws = [x.strip() for x in m.group(2).split(",") if x.strip()]
            if cluster and kws:
                cluster_stats[cluster] = kws
    return source_stats, cluster_stats


def _build_keyword_analytics(iter_messages: list[str]) -> str:
    src_keywords_total: dict[str, int] = {}
    src_clusters_max: dict[str, int] = {}
    cluster_examples: dict[str, list[str]] = {}
    cluster_hits: dict[str, int] = {}
    for msg in iter_messages:
        src_stats, cl_stats = _parse_keyword_stats(msg)
        for src, (kw, cl) in src_stats.items():
            src_keywords_total[src] = src_keywords_total.get(src, 0) + kw
            src_clusters_max[src] = max(src_clusters_max.get(src, 0), cl)
        for cluster, kws in cl_stats.items():
            cluster_hits[cluster] = cluster_hits.get(cluster, 0) + 1
            if cluster not in cluster_examples:
                cluster_examples[cluster] = kws[:5]
    lines: list[str] = []
    lines.append("keyword_analytics:")
    lines.append("source_effectiveness_total:")
    for src, kw_total in sorted(src_keywords_total.items(), key=lambda kv: kv[1], reverse=True):
        lines.append(f"- {src}: keywords_total={kw_total}, clusters_max={src_clusters_max.get(src, 0)}")
    lines.append("")
    lines.append("strategy_modes:")
    for msg in iter_messages:
        cl = 0
        for line in msg.splitlines():
            if line.startswith("cluster_count_estimate="):
                try:
                    cl = int(line.split("=", 1)[1].strip())
                except Exception:
                    cl = 0
                break
        mode = "exploration"
        if cl > 0:
            mode = "exploitation" if cl >= 20 else "exploration"
        lines.append(f"- iter_cluster_count={cl}: mode={mode}")
    lines.append("")
    lines.append("top_clusters_by_iteration_presence:")
    for cluster, hits in sorted(cluster_hits.items(), key=lambda kv: kv[1], reverse=True)[:20]:
        examples = ", ".join(cluster_examples.get(cluster, [])[:5])
        lines.append(f"- {cluster}: seen_in_iterations={hits}; keywords={examples}")
    if len(lines) <= 4:
        return "keyword_analytics:\n(no stats collected)"
    return "\n".join(lines)


@workflow.defn
class SeoCampaignWorkflow:
    @workflow.run
    async def run(self, inp: CampaignInput) -> CampaignResult:
        pipeline_id = str(workflow.uuid4())
        ct_job = str(workflow.uuid4())
        qa_job = str(workflow.uuid4())
        info = workflow.info()
        tq = info.task_queue
        primary_chat = (inp.telegram_chat_id or "").strip()
        secondary_chat = (inp.telegram_report_chat_id or "").strip()

        def _notify_chats() -> list[str]:
            out: list[str] = []
            for c in (primary_chat, secondary_chat):
                cc = (c or "").strip()
                if cc and cc not in out:
                    out.append(cc)
            return out

        async def _notify(phase: str, detail: str) -> None:
            chats = _notify_chats()
            if not chats:
                # keep backward compatibility: let activity fallback to env TELEGRAM_REPORT_CHAT_ID
                chats = [""]
            for chat in chats:
                await workflow.execute_activity(
                    "notify_telegram",
                    NotifyTelegramInput(
                        chat_id=chat,
                        phase=phase,
                        campaign_id=inp.campaign_id,
                        pipeline_id=pipeline_id,
                        detail=detail,
                    ),
                    start_to_close_timeout=timedelta(seconds=90),
                    result_type=NotifyTelegramOutput,
                )

        async def _notify_trello_card(phase: str, detail: str) -> None:
            card_id = (inp.trello_card_id or "").strip()
            if not card_id:
                return
            await workflow.execute_activity(
                "notify_trello_card",
                NotifyTrelloCardInput(
                    card_id=card_id,
                    phase=phase,
                    campaign_id=inp.campaign_id,
                    pipeline_id=pipeline_id,
                    detail=detail,
                ),
                start_to_close_timeout=timedelta(seconds=90),
                result_type=NotifyTrelloCardOutput,
            )

        max_iterations = max(1, min(inp.keyword_max_iterations, 50))
        force_depth = bool(inp.keyword_force_depth)
        breadth_iters = 0 if force_depth else max(1, max_iterations // 2)
        target_keywords = max(1, inp.keyword_target_keywords)
        target_clusters = max(1, inp.keyword_target_clusters)
        dynamic_query = (inp.keyword_query or "").strip()
        dynamic_seeds = [x.strip() for x in (inp.keyword_seeds or []) if x and x.strip()]
        dynamic_clusters: list[KeywordCluster] = []
        focus_cluster = (inp.keyword_focus_cluster or "").strip()
        seed_version = 0
        cluster_version = 0
        vertical_guess = (dynamic_query.replace("keywords", "").strip() if dynamic_query else "general") or "general"

        if (not dynamic_query or not dynamic_seeds) and not force_depth:
            boot: KeywordBootstrapOutput = await workflow.execute_activity(
                "keyword_llm_bootstrap",
                KeywordBootstrapInput(
                    vertical=vertical_guess,
                    query=dynamic_query,
                    seeds=dynamic_seeds,
                    hl=inp.keyword_hl,
                    gl=inp.keyword_gl,
                    target_keywords=target_keywords,
                    target_clusters=target_clusters,
                    max_seed_initial=max(20, min(120, inp.keyword_limit * 2)),
                ),
                start_to_close_timeout=timedelta(seconds=90),
                result_type=KeywordBootstrapOutput,
            )
            dynamic_query = boot.query
            dynamic_seeds = boot.seeds
            dynamic_clusters = boot.clusters
            seed_version = boot.seed_version
            cluster_version = boot.cluster_version
            bootstrap_detail = (
                f"query={dynamic_query}\n"
                f"seed_version={seed_version}\n"
                f"cluster_version={cluster_version}\n"
                f"seeds={len(dynamic_seeds)}\n"
                f"clusters={len(dynamic_clusters)}\n"
                f"rationale={boot.rationale}"
            )
            await _notify("keyword_bootstrap", bootstrap_detail)
            await _notify_trello_card("keyword_bootstrap", bootstrap_detail)
        aggregated_keywords = 0
        best_clusters = 0
        stagnation_hits = 0
        prev_kw = 0
        prev_cl = 0
        keyword_msgs: list[str] = []
        last_kr: BatchResult | None = None

        for i in range(1, max_iterations + 1):
            iter_route = inp.keyword_adapter_route
            iter_mode = inp.keyword_mode
            iter_limit = max(20, inp.keyword_limit)
            iter_trends_seed_mode = inp.keyword_trends_seed_mode
            iter_trends_filter = inp.keyword_trends_filter_by_query
            iter_topic_cap = max(1, int(inp.keyword_topic_cap or 2))

            if force_depth:
                # Cluster-focused depth mode: keep route and query stable, avoid breadth reroute.
                iter_route = inp.keyword_adapter_route or "direct_discovery"
                iter_mode = inp.keyword_mode or "cheap_live"
                iter_limit = max(80, inp.keyword_limit)
                iter_topic_cap = max(20, iter_topic_cap)
                if focus_cluster:
                    dynamic_query = focus_cluster
            elif i <= breadth_iters:
                # Breadth: find new cluster surfaces faster via autocomplete.
                if (inp.keyword_adapter_route or "").strip().lower().startswith("direct_"):
                    iter_route = "direct_discovery"
                else:
                    iter_route = "live_autocomplete"
                iter_mode = "cached"
                iter_limit = max(40, min(200, inp.keyword_limit * 2))
            else:
                # Depth: mine more long-tail inside discovered areas.
                iter_route = inp.keyword_adapter_route or "search"
                iter_mode = inp.keyword_mode or "cheap_live"
                iter_limit = max(40, inp.keyword_limit)
                iter_topic_cap = max(10, iter_topic_cap)
                if iter_route == "live_trends":
                    iter_trends_seed_mode = "only_query"
                    iter_trends_filter = True

            iter_job = str(workflow.uuid4())
            kr: BatchResult = await workflow.execute_child_workflow(
                KeywordBatchWorkflow.run,
                KeywordBatchInput(
                    campaign_id=inp.campaign_id,
                    job_id=iter_job,
                    pipeline_id=pipeline_id,
                    keyword_query=dynamic_query,
                    keyword_seeds=dynamic_seeds,
                    keyword_mode=iter_mode,
                    keyword_limit=iter_limit,
                    keyword_sources=inp.keyword_sources,
                    keyword_source_profile=inp.keyword_source_profile,
                    keyword_source_timeouts=inp.keyword_source_timeouts,
                    keyword_source_budgets=inp.keyword_source_budgets,
                    keyword_merge_mode=inp.keyword_merge_mode,
                    keyword_topic_cap=iter_topic_cap,
                    keyword_adapter_route=iter_route,
                    keyword_hl=inp.keyword_hl,
                    keyword_gl=inp.keyword_gl,
                    keyword_trends_config_path=inp.keyword_trends_config_path,
                    keyword_trends_subprocess_timeout_sec=inp.keyword_trends_subprocess_timeout_sec,
                    keyword_trends_filter_by_query=iter_trends_filter,
                    keyword_trends_seed_mode=iter_trends_seed_mode,
                    keyword_youtube_region_code=inp.keyword_youtube_region_code,
                    keyword_youtube_relevance_language=inp.keyword_youtube_relevance_language,
                    keyword_youtube_max_results=inp.keyword_youtube_max_results,
                    keyword_include_raw_ref=inp.keyword_include_raw_ref,
                ),
                id=f"keyword-{inp.campaign_id}-{iter_job}",
                task_queue=tq,
                result_type=BatchResult,
            )
            last_kr = kr
            await _notify("keyword", f"iteration={i}/{max_iterations}\n{_batch_detail(kr)}")
            await _notify_trello_card("keyword_iteration", f"iteration={i}/{max_iterations}\n{_batch_detail(kr)}")
            keyword_msgs.append(f"[iteration {i}] {_batch_detail(kr)}")

            cur_kw = max(0, kr.keyword_count or 0)
            cur_cl = max(0, kr.cluster_count_estimate or 0)
            aggregated_keywords += cur_kw
            best_clusters = max(best_clusters, cur_cl)

            prev_kw, prev_cl = cur_kw, cur_cl

            found_sample = _extract_keywords_from_message(kr.message, limit=80)
            refine: KeywordRefineOutput = await workflow.execute_activity(
                "keyword_llm_refine",
                KeywordRefineInput(
                    iteration=i + 1,
                    hl=inp.keyword_hl,
                    gl=inp.keyword_gl,
                    query=dynamic_query,
                    seeds_prev=dynamic_seeds,
                    clusters_prev=dynamic_clusters,
                    found_keywords_sample=found_sample,
                    keyword_count=cur_kw,
                    cluster_count_estimate=cur_cl,
                    target_keywords=target_keywords,
                    target_clusters=target_clusters,
                    max_new_seeds=max(10, min(50, inp.keyword_limit // 2)),
                    max_total_seeds=max(100, inp.keyword_limit * 3),
                    max_new_clusters=max(5, min(20, target_clusters // 5 or 5)),
                ),
                start_to_close_timeout=timedelta(seconds=90),
                result_type=KeywordRefineOutput,
            )
            dynamic_query = refine.query_next or dynamic_query
            dynamic_seeds = refine.seeds_next or dynamic_seeds
            # Keep cluster history cumulative: grow breadth, do not reset map on each refine.
            prev_by_id = {c.id: c for c in dynamic_clusters}
            for c in refine.clusters_next or []:
                prev_by_id[c.id] = c
            dynamic_clusters = list(prev_by_id.values())
            seed_version = refine.seed_version
            cluster_version = refine.cluster_version
            added_seeds_count = len(refine.added_seeds)
            refine_detail = (
                f"iteration={i}/{max_iterations}\n"
                f"seed_version={seed_version}\n"
                f"cluster_version={cluster_version}\n"
                f"added_seeds={added_seeds_count}\n"
                f"added_clusters={len(refine.added_clusters)}\n"
                f"dropped_clusters=0\n"
                f"rationale={refine.rationale}"
            )
            await _notify("keyword_refine", refine_detail)
            await _notify_trello_card("keyword_refine", refine_detail)

            kw_growth_ok = prev_kw > 0 and cur_kw <= int(prev_kw * 1.03)
            cl_growth_ok = prev_cl > 0 and cur_cl <= int(prev_cl * 1.02)
            if kw_growth_ok and cl_growth_ok and added_seeds_count < 3:
                stagnation_hits += 1
            else:
                stagnation_hits = 0

            if aggregated_keywords >= target_keywords and best_clusters >= target_clusters:
                break
            if stagnation_hits >= 4:
                break

        if last_kr is None:
            last_kr = BatchResult(ok=False, message="keyword stage failed to start", graph_name="keyword_trello")
        kr = BatchResult(
            ok=last_kr.ok,
            message=(
                f"iterations_done={len(keyword_msgs)}\n"
                f"aggregated_keywords={aggregated_keywords}\n"
                f"best_cluster_count={best_clusters}\n"
                f"targets: keywords>={target_keywords}, clusters>={target_clusters}\n\n"
                + "\n\n".join(keyword_msgs)
                + "\n\n"
                + _build_keyword_analytics(keyword_msgs)
            ),
            graph_name="keyword_trello",
            content_score=last_kr.content_score,
            keyword_count=aggregated_keywords,
            cluster_count_estimate=best_clusters,
        )
        await _notify_trello_card("keyword", kr.message)
        cr: BatchResult = await workflow.execute_child_workflow(
            ContentBatchWorkflow.run,
            ContentBatchInput(
                campaign_id=inp.campaign_id,
                job_id=ct_job,
                pipeline_id=pipeline_id,
            ),
            id=f"content-{inp.campaign_id}-{ct_job}",
            task_queue=tq,
            result_type=BatchResult,
        )
        await _notify("content", _batch_detail(cr))
        qr: BatchResult = await workflow.execute_child_workflow(
            QaBatchWorkflow.run,
            QaBatchInput(
                campaign_id=inp.campaign_id,
                job_id=qa_job,
                pipeline_id=pipeline_id,
            ),
            id=f"qa-{inp.campaign_id}-{qa_job}",
            task_queue=tq,
            result_type=BatchResult,
        )
        await _notify("qa", _batch_detail(qr))
        out = CampaignResult(
            keyword_summary=kr.message,
            content_summary=cr.message,
            qa_summary=qr.message,
        )
        await _notify(
            "done",
            "Пайплайн завершён.\n\n"
            f"[keyword]\n{kr.message}\n\n"
            f"[content]\n{cr.message}\n\n"
            f"[qa]\n{qr.message}",
        )
        await _notify_trello_card(
            "done",
            "Пайплайн завершён.\n\n"
            f"[keyword]\n{kr.message}\n\n"
            f"[content]\n{cr.message}\n\n"
            f"[qa]\n{qr.message}",
        )
        return out
