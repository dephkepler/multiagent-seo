from __future__ import annotations

from temporalio import activity

from seo_os.integrations.trello import TrelloClient, TrelloConfigError
from seo_os.temporal.shared_inputs import NotifyTrelloCardInput, NotifyTrelloCardOutput


def _compact_detail(detail: str, max_len: int = 3500) -> str:
    lines = (detail or "").splitlines()
    # Keep key metrics + top keyword lines only.
    keep: list[str] = []
    for ln in lines:
        if (
            ln.startswith("iterations_done=")
            or ln.startswith("aggregated_keywords=")
            or ln.startswith("best_cluster_count=")
            or ln.startswith("targets:")
            or ln.startswith("[iteration")
            or ln.startswith("keyword_count=")
            or ln.startswith("cluster_count_estimate=")
            or ln.startswith("- ")
            or "[adapter:" in ln
        ):
            keep.append(ln)
        if len(keep) >= 60:
            break
    body = "\n".join(keep) if keep else (detail or "")
    return body[:max_len]


@activity.defn(name="notify_trello_card")
def notify_trello_card(inp: NotifyTrelloCardInput) -> NotifyTrelloCardOutput:
    card_id = (inp.card_id or "").strip()
    if not card_id:
        return NotifyTrelloCardOutput(ok=True, skipped=True, detail="card_id is empty")
    try:
        client = TrelloClient.from_env()
    except TrelloConfigError as e:
        return NotifyTrelloCardOutput(ok=False, skipped=True, detail=str(e))

    msg = (
        f"SEO OS — phase: {inp.phase}\n"
        f"campaign_id={inp.campaign_id}\n"
        f"pipeline_id={inp.pipeline_id}\n\n"
        f"{_compact_detail(inp.detail)}"
    )
    try:
        client.add_comment(card_id, msg)
        return NotifyTrelloCardOutput(ok=True, skipped=False, detail=None)
    except Exception as e:
        return NotifyTrelloCardOutput(ok=False, skipped=False, detail=str(e)[:500])

