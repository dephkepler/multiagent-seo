"""Temporal worker: parent + child workflows + run_langgraph activity."""
from __future__ import annotations

import asyncio
import concurrent.futures
import os

from dotenv import load_dotenv
from temporalio.client import Client
from temporalio.worker import Worker

from seo_os.temporal.activities_impl import run_langgraph
from seo_os.temporal.activities_keyword_search import keyword_search
from seo_os.temporal.activities_keyword_strategy import keyword_llm_bootstrap, keyword_llm_refine
from seo_os.temporal.activities_notify import notify_telegram
from seo_os.temporal.activities_trello_card import notify_trello_card
from seo_os.temporal.workflows import (
    ContentBatchWorkflow,
    KeywordBatchWorkflow,
    QaBatchWorkflow,
    SeoCampaignWorkflow,
)

TASK_QUEUE = os.environ.get("TEMPORAL_TASK_QUEUE", "seo-os-local")


async def _run() -> None:
    host = os.environ.get("TEMPORAL_ADDRESS", "localhost:7233")
    client = await Client.connect(host)
    # Синхронные activities (run_langgraph) требуют executor в temporalio 1.7+
    with concurrent.futures.ThreadPoolExecutor(max_workers=32) as executor:
        worker = Worker(
            client,
            task_queue=TASK_QUEUE,
            workflows=[
                SeoCampaignWorkflow,
                KeywordBatchWorkflow,
                ContentBatchWorkflow,
                QaBatchWorkflow,
            ],
            activities=[
                run_langgraph,
                keyword_search,
                keyword_llm_bootstrap,
                keyword_llm_refine,
                notify_telegram,
                notify_trello_card,
            ],
            activity_executor=executor,
        )
        await worker.run()


def main() -> None:
    load_dotenv()
    asyncio.run(_run())


if __name__ == "__main__":
    main()
