"""Заглушка quality gate: скор по длине/наличию текста (без LLM)."""
from __future__ import annotations

QUALITY_THRESHOLD = 0.75


def evaluate_content_quality(draft: str) -> dict[str, float]:
    """Возвращает оси 0..1; итог — среднее (заглушка)."""
    t = (draft or "").strip()
    if not t:
        return {
            "intent_match": 0.0,
            "completeness": 0.0,
            "uniqueness": 0.0,
            "serp_advantage": 0.0,
            "aggregate": 0.0,
        }
    # Простая эвристика для skeleton
    completeness = min(1.0, len(t) / 400.0)
    intent_match = 0.85 if len(t) > 50 else 0.4
    uniqueness = 0.8
    serp_advantage = 0.7
    vals = [intent_match, completeness, uniqueness, serp_advantage]
    return {
        "intent_match": intent_match,
        "completeness": completeness,
        "uniqueness": uniqueness,
        "serp_advantage": serp_advantage,
        "aggregate": sum(vals) / len(vals),
    }


def passed_quality_gate(scores: dict[str, float]) -> bool:
    return scores.get("aggregate", 0.0) >= QUALITY_THRESHOLD
