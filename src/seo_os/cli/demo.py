"""Сид БД + старт SeoCampaignWorkflow (нужны worker и Temporal)."""
from __future__ import annotations

import argparse
import asyncio
import os

from dotenv import load_dotenv

from seo_os.temporal.campaign_starter import run_seo_campaign_to_completion


async def _async_main(workflow_id: str | None) -> None:
    load_dotenv()
    if not os.environ.get("DATABASE_URL"):
        raise SystemExit("Set DATABASE_URL")

    result = await run_seo_campaign_to_completion(workflow_id=workflow_id)
    print("Result:", result)
    print("keyword_summary:", result.keyword_summary)
    print("content_summary:", result.content_summary)
    print("qa_summary:", result.qa_summary)


def main() -> None:
    p = argparse.ArgumentParser(description="Seed DB and run SeoCampaignWorkflow")
    p.add_argument("--workflow-id", default=None, help="Temporal workflow id (unique)")
    args = p.parse_args()
    asyncio.run(_async_main(args.workflow_id))


if __name__ == "__main__":
    main()
