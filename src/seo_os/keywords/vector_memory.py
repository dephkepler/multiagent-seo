"""MVP vector memory utilities (OpenAI embeddings + deterministic fallback)."""
from __future__ import annotations

import hashlib
import math
import os
import re

from seo_os.api.chat_backend import _normalize_openai_key


def _tokenize(text: str) -> list[str]:
    t = re.sub(r"\s+", " ", (text or "").strip().lower())
    return [x for x in re.split(r"[^a-z0-9]+", t) if x]


def _normalize(vec: list[float]) -> list[float]:
    norm = math.sqrt(sum(v * v for v in vec)) or 1.0
    return [v / norm for v in vec]


def _hash_embed(text: str, *, dim: int) -> list[float]:
    vec = [0.0] * dim
    for tok in _tokenize(text):
        h = int(hashlib.sha256(tok.encode("utf-8")).hexdigest(), 16)
        idx = h % dim
        sign = -1.0 if ((h >> 8) & 1) else 1.0
        vec[idx] += sign
    return _normalize(vec)


def embed_texts(texts: list[str], *, dim: int) -> tuple[list[list[float]], str]:
    key = _normalize_openai_key(os.environ.get("OPENAI_API_KEY") or "")
    model = (os.environ.get("OPENAI_EMBEDDING_MODEL") or "text-embedding-3-small").strip()
    if not key:
        return ([_hash_embed(t, dim=dim) for t in texts], f"hash-{dim}")
    try:
        from openai import OpenAI

        client = OpenAI(api_key=key, timeout=120.0)
        resp = client.embeddings.create(model=model, input=texts, dimensions=dim)
        vectors: list[list[float]] = []
        for item in resp.data:
            vectors.append(_normalize([float(x) for x in item.embedding]))
        if len(vectors) != len(texts):
            raise RuntimeError("embeddings size mismatch")
        return vectors, model
    except Exception:
        return ([_hash_embed(t, dim=dim) for t in texts], f"hash-{dim}")


def cosine_similarity(a: list[float], b: list[float]) -> float:
    n = min(len(a), len(b))
    if n == 0:
        return 0.0
    return float(sum(a[i] * b[i] for i in range(n)))
