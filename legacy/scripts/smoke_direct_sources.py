from __future__ import annotations

import json
import os

from seo_os.keywords.quality import dedupe_hits, score_hits, split_novelty


def main() -> None:
    sample = [
        {"keyword": "Gambling Keywords", "source": "autocomplete", "topic": "gambling"},
        {"keyword": "gambling keywords", "source": "trends", "topic": "gambling"},
        {"keyword": "best gambling tips", "source": "youtube", "topic": "gambling tips"},
    ]
    ded = dedupe_hits(sample, topic_cap=2)
    scored = score_hits(ded)
    seen_file = os.environ.get("KEYWORD_SEEN_FILE") or "runtime/keyword_seen_keywords.txt"
    new_rows, resurfaced = split_novelty(scored, seen_file=seen_file)
    print(
        json.dumps(
            {
                "deduped": len(ded),
                "scored": len(scored),
                "new": len(new_rows),
                "resurfaced": len(resurfaced),
                "seen_file": seen_file,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()

