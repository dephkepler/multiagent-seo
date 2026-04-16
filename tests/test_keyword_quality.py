from __future__ import annotations

from seo_os.keywords.quality import dedupe_hits, normalize_keyword, score_hits


def test_normalize_keyword() -> None:
    assert normalize_keyword("  Best   BONUS  App ") == "best bonus app"


def test_dedupe_hits() -> None:
    rows = [
        {"keyword": "bonus app", "topic": "bonus", "source": "autocomplete"},
        {"keyword": "Bonus App", "topic": "bonus", "source": "trends"},
        {"keyword": "bonus app no deposit", "topic": "bonus", "source": "youtube"},
    ]
    out = dedupe_hits(rows, topic_cap=2)
    assert len(out) == 2


def test_score_hits() -> None:
    rows = [{"keyword": "best bonus app", "topic": "bonus", "source": "ahrefs"}]
    out = score_hits(rows)
    assert len(out) == 1
    assert float(out[0]["score"]) >= 0.9

