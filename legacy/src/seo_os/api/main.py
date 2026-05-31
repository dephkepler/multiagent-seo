"""HTTP-слой: health, чат (/v1/chat), старт кампании (/v1/campaign/start)."""
from __future__ import annotations

import os
from datetime import datetime
from uuid import uuid4

from dotenv import load_dotenv
from fastapi import FastAPI, Header, HTTPException
from fastapi.responses import HTMLResponse
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from sqlalchemy import text
from temporalio.client import Client, WorkflowExecutionStatus

from seo_os.api.chat_backend import complete_chat
from seo_os.db.session import get_engine
from seo_os.temporal.campaign_starter import (
    start_seo_campaign_fire_and_forget,
    start_seo_campaign_with_overrides_fire_and_forget,
)

load_dotenv()

app = FastAPI(
    title="SEO OS API",
    description="Чат (OpenAI), старт SeoCampaignWorkflow через POST /v1/campaign/start.",
    version="0.1.0",
)

_cors = os.environ.get("CORS_ORIGINS", "*").strip()
_origins = [o.strip() for o in _cors.split(",") if o.strip()] if _cors != "*" else ["*"]
app.add_middleware(
    CORSMiddleware,
    allow_origins=_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


class ChatRequest(BaseModel):
    message: str = Field(..., min_length=1, max_length=16000)
    session_id: str | None = Field(default=None, max_length=128)


class ChatResponse(BaseModel):
    reply: str
    note: str | None = None


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/v1/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    """Ответ от OpenAI (если задан OPENAI_API_KEY) или заглушка."""
    out = complete_chat(req.message, req.session_id)
    return ChatResponse(reply=out.reply, note=out.note)


class CampaignStartResponse(BaseModel):
    campaign_id: str
    workflow_id: str


class VerticalClusterView(BaseModel):
    cluster: str
    keyword_count: int
    keywords: list[str]


class VerticalStateResponse(BaseModel):
    vertical: str
    total_keywords: int
    total_clusters: int
    clusters: list[VerticalClusterView]
    generated_at: str


class ContinueVerticalResponse(BaseModel):
    ok: bool
    mode: str
    run_mode: str
    vertical: str
    campaign_id: str
    workflow_id: str
    cluster_key: str | None = None


class VerticalRunStatusResponse(BaseModel):
    ok: bool
    workflow_id: str
    campaign_id: str
    vertical: str
    run_mode: str
    cluster_key: str | None = None
    status: str
    iteration: int = 0
    keyword_count: int = 0
    new_keyword_count: int = 0
    clusters_seen: int = 0
    updated_at: str | None = None
    error: str | None = None


class SeedSuggestRequest(BaseModel):
    message: str
    current_llm_seeds: list[str] = Field(default_factory=list)
    current_my_seeds: list[str] = Field(default_factory=list)


class SeedSuggestResponse(BaseModel):
    seed_suggestions: list[str]
    warnings: list[str]
    intent_mix: dict[str, int]
    note: str | None = None


class SeedValidateRequest(BaseModel):
    llm_seeds: list[str] = Field(default_factory=list)
    my_seeds: list[str] = Field(default_factory=list)


class SeedValidateResponse(BaseModel):
    deduped_llm: list[str]
    deduped_my: list[str]
    mixed: list[str]
    duplicates: list[str]
    weak: list[str]
    breadth_estimate: str


class VerticalRunRequest(BaseModel):
    mode: str = Field(pattern="^(breadth|depth|mixed)$")
    seeds: list[str] = Field(default_factory=list)
    cluster: str | None = None
    campaign_start_key: str | None = None


@app.post("/v1/campaign/start", response_model=CampaignStartResponse)
async def campaign_start(
    x_campaign_start_key: str | None = Header(default=None, alias="X-Campaign-Start-Key"),
) -> CampaignStartResponse:
    """Сид + старт SeoCampaignWorkflow (как seo-os-demo), без ожидания завершения.

    Требует заголовок `X-Campaign-Start-Key`, совпадающий с `CAMPAIGN_START_SECRET` в .env.
    """
    secret = (os.environ.get("CAMPAIGN_START_SECRET") or "").strip()
    if not secret:
        raise HTTPException(
            status_code=503,
            detail="CAMPAIGN_START_SECRET не задан — эндпоинт отключён.",
        )
    if not x_campaign_start_key or x_campaign_start_key != secret:
        raise HTTPException(status_code=401, detail="Неверный или отсутствующий X-Campaign-Start-Key")
    if not (os.environ.get("DATABASE_URL") or "").strip():
        raise HTTPException(status_code=503, detail="DATABASE_URL не задан")
    try:
        cid, wid = await start_seo_campaign_fire_and_forget()
    except Exception as e:
        raise HTTPException(status_code=502, detail=str(e)) from e
    return CampaignStartResponse(campaign_id=str(cid), workflow_id=wid)


def _ensure_campaign_secret(x_campaign_start_key: str | None) -> None:
    secret = (os.environ.get("CAMPAIGN_START_SECRET") or "").strip()
    if not secret:
        raise HTTPException(status_code=503, detail="CAMPAIGN_START_SECRET не задан — эндпоинт отключён.")
    if not x_campaign_start_key or x_campaign_start_key != secret:
        raise HTTPException(status_code=401, detail="Неверный или отсутствующий X-Campaign-Start-Key")


def _vertical_state(vertical: str) -> VerticalStateResponse:
    v = (vertical or "").strip().lower()
    if not v:
        raise HTTPException(status_code=400, detail="vertical is required")
    engine = get_engine()
    # Practical heuristic: include clusters/keywords where cluster or keyword text contains vertical token.
    q = text(
        """
        WITH data AS (
            SELECT
                cm.cluster_key AS cluster,
                km.canonical_keyword AS keyword
            FROM cluster_memory cm
            JOIN keyword_cluster_links kcl ON kcl.cluster_memory_id = cm.id
            JOIN keyword_memory km ON km.id = kcl.keyword_memory_id
            WHERE cm.cluster_key ILIKE :pat OR km.canonical_keyword ILIKE :pat
        )
        SELECT cluster, COUNT(*) AS keyword_count
        FROM data
        GROUP BY cluster
        ORDER BY keyword_count DESC, cluster ASC
        LIMIT 200
        """
    )
    q_kw = text(
        """
        SELECT km.canonical_keyword AS keyword
        FROM cluster_memory cm
        JOIN keyword_cluster_links kcl ON kcl.cluster_memory_id = cm.id
        JOIN keyword_memory km ON km.id = kcl.keyword_memory_id
        WHERE cm.cluster_key = :cluster
        ORDER BY km.last_seen_at DESC
        LIMIT 30
        """
    )
    clusters: list[VerticalClusterView] = []
    total_keywords = 0
    with engine.begin() as conn:
        rows = conn.execute(q, {"pat": f"%{v}%"}).mappings().all()
        for row in rows:
            c = str(row["cluster"])
            k_rows = conn.execute(q_kw, {"cluster": c}).mappings().all()
            kws = [str(r["keyword"]) for r in k_rows if str(r["keyword"]).strip()]
            kc = int(row["keyword_count"] or 0)
            total_keywords += kc
            clusters.append(VerticalClusterView(cluster=c, keyword_count=kc, keywords=kws))
    return VerticalStateResponse(
        vertical=v,
        total_keywords=total_keywords,
        total_clusters=len(clusters),
        clusters=clusters,
        generated_at=datetime.utcnow().isoformat() + "Z",
    )


async def _workflow_status(workflow_id: str) -> tuple[str, str | None]:
    try:
        client = await Client.connect((os.environ.get("TEMPORAL_ADDRESS") or "localhost:7233").strip())
        desc = await client.get_workflow_handle(workflow_id).describe()
        st = desc.status
        mapping = {
            WorkflowExecutionStatus.RUNNING: "running",
            WorkflowExecutionStatus.COMPLETED: "done",
            WorkflowExecutionStatus.FAILED: "error",
            WorkflowExecutionStatus.TERMINATED: "error",
            WorkflowExecutionStatus.CANCELED: "error",
            WorkflowExecutionStatus.TIMED_OUT: "error",
            WorkflowExecutionStatus.CONTINUED_AS_NEW: "running",
        }
        return mapping.get(st, str(st).lower()), None
    except Exception as e:
        # If Temporal is temporarily unavailable, UI still receives snapshot-based progress.
        return "queued", str(e)[:240]


def _run_snapshot(campaign_id: str) -> dict[str, object]:
    engine = get_engine()
    q = text(
        """
        SELECT payload, created_at
        FROM keyword_run_snapshots
        WHERE campaign_id = :cid
        ORDER BY created_at DESC
        LIMIT 1
        """
    )
    q_cnt = text(
        """
        SELECT COUNT(*) AS cnt
        FROM keyword_run_snapshots
        WHERE campaign_id = :cid
        """
    )
    with engine.begin() as conn:
        row = conn.execute(q, {"cid": campaign_id}).mappings().first()
        cnt = conn.execute(q_cnt, {"cid": campaign_id}).mappings().first()
    payload = dict(row["payload"]) if row and row.get("payload") else {}
    return {
        "iteration": int((cnt or {}).get("cnt") or 0),
        "keyword_count": int(payload.get("keyword_count") or 0),
        "new_keyword_count": int(payload.get("new_keyword_count") or 0),
        "clusters_seen": int((payload.get("stats") or {}).get("cluster_keywords", {}) and len((payload.get("stats") or {}).get("cluster_keywords", {})) or 0),
        "updated_at": (row.get("created_at").isoformat() + "Z") if row and row.get("created_at") else None,
    }


@app.get("/v1/verticals/{vertical}/state", response_model=VerticalStateResponse)
def vertical_state(vertical: str) -> VerticalStateResponse:
    return _vertical_state(vertical)


async def _start_vertical_mode(
    *,
    vertical: str,
    mode: str,
    x_campaign_start_key: str | None,
    seeds: list[str] | None = None,
    cluster: str | None = None,
) -> ContinueVerticalResponse:
    _ensure_campaign_secret(x_campaign_start_key)
    v = (vertical or "").strip().lower()
    if not v:
        raise HTTPException(status_code=400, detail="vertical is required")
    base_overrides: dict[str, object] = {
        "keyword_adapter_route": "direct_discovery",
        "keyword_query": f"{v} keywords",
        "keyword_seeds": list(seeds) if seeds else None,
        "keyword_sources": ["autocomplete", "trends", "youtube"],
        "keyword_mode": "cheap_live",
        "keyword_hl": "en",
        "keyword_gl": "us",
        "keyword_limit": 120,
        "keyword_max_iterations": 4,
    }
    run_mode = "breadth"
    cluster_key = None
    if mode == "breadth":
        base_overrides.update(
            {
                "keyword_target_clusters": 400,
                "keyword_target_keywords": 1200,
                "keyword_merge_mode": "topic_cap",
                "keyword_topic_cap": 3,
            }
        )
    elif mode == "depth":
        depth_cluster = (cluster or "").strip()
        depth_query = depth_cluster or f"{v} keywords"
        cluster_key = depth_cluster or None
        run_mode = "depth_cluster" if depth_cluster else "depth"
        base_overrides.update(
            {
                "keyword_query": depth_query,
                "keyword_target_clusters": 120 if not depth_cluster else 1,
                "keyword_target_keywords": 3000,
                "keyword_merge_mode": "topic_cap",
                "keyword_topic_cap": 30,
                "keyword_limit": 180,
                "keyword_force_depth": True,
                "keyword_focus_cluster": depth_cluster or None,
                "keyword_seeds": list(seeds) if seeds else ([depth_cluster] if depth_cluster else None),
            }
        )
    else:
        raise HTTPException(status_code=400, detail="mode must be breadth or depth")

    try:
        cid, wid = await start_seo_campaign_with_overrides_fire_and_forget(
            overrides=base_overrides,
            workflow_id=f"seo-{mode}-{v}-{uuid4()}",
        )
    except Exception as e:
        raise HTTPException(status_code=502, detail=str(e)) from e
    return ContinueVerticalResponse(
        ok=True,
        mode=mode,
        run_mode=run_mode,
        vertical=v,
        campaign_id=str(cid),
        workflow_id=wid,
        cluster_key=cluster_key,
    )


@app.post("/v1/verticals/{vertical}/continue-breadth", response_model=ContinueVerticalResponse)
async def continue_breadth(
    vertical: str,
    x_campaign_start_key: str | None = Header(default=None, alias="X-Campaign-Start-Key"),
) -> ContinueVerticalResponse:
    return await _start_vertical_mode(vertical=vertical, mode="breadth", x_campaign_start_key=x_campaign_start_key)


@app.post("/v1/verticals/{vertical}/continue-depth", response_model=ContinueVerticalResponse)
async def continue_depth(
    vertical: str,
    x_campaign_start_key: str | None = Header(default=None, alias="X-Campaign-Start-Key"),
) -> ContinueVerticalResponse:
    return await _start_vertical_mode(vertical=vertical, mode="depth", x_campaign_start_key=x_campaign_start_key)


def _norm_seed(s: str) -> str:
    return " ".join((s or "").strip().lower().split())


def _dedupe_seeds(items: list[str]) -> tuple[list[str], list[str]]:
    out: list[str] = []
    dups: list[str] = []
    seen: set[str] = set()
    for raw in items:
        s = (raw or "").strip()
        key = _norm_seed(s)
        if not key:
            continue
        if key in seen:
            dups.append(s)
            continue
        seen.add(key)
        out.append(s)
    return out, dups


@app.post("/v1/verticals/{vertical}/seeds/chat-suggest", response_model=SeedSuggestResponse)
def seeds_chat_suggest(vertical: str, req: SeedSuggestRequest) -> SeedSuggestResponse:
    v = (vertical or "").strip().lower() or "general"
    msg = (req.message or "").strip()
    if not msg:
        raise HTTPException(status_code=400, detail="message is required")
    prompt = (
        f"Вертикаль: {v}. "
        f"Пользователь попросил: {msg}. "
        "Сгенерируй 20 seed-фраз для keyword research. "
        "Формат ответа: одна seed-фраза на строку, без нумерации."
    )
    out = complete_chat(prompt, session_id=f"seed-suggest-{v}")
    raw_lines = [x.strip(" -\t") for x in out.reply.splitlines()]
    suggestions, _ = _dedupe_seeds(raw_lines)
    if not suggestions:
        suggestions = [
            f"{v} game providers",
            f"{v} casino platforms",
            f"{v} casino brands",
            f"{v} affiliate programs",
            f"best {v} bonuses",
        ]
    warnings: list[str] = []
    weak = [s for s in suggestions if len(s.split()) < 2]
    if weak:
        warnings.append(f"Есть слишком общие seeds: {len(weak)}")
    mix = {
        "informational": len([s for s in suggestions if "how" in s.lower() or "what" in s.lower()]),
        "commercial": len([s for s in suggestions if "best" in s.lower() or "top" in s.lower()]),
        "brand_or_entity": len(
            [s for s in suggestions if any(k in s.lower() for k in ("provider", "platform", "brand", "casino"))]
        ),
    }
    return SeedSuggestResponse(
        seed_suggestions=suggestions[:40],
        warnings=warnings,
        intent_mix=mix,
        note=out.note,
    )


@app.post("/v1/verticals/{vertical}/seeds/validate", response_model=SeedValidateResponse)
def seeds_validate(vertical: str, req: SeedValidateRequest) -> SeedValidateResponse:
    llm, llm_dups = _dedupe_seeds(req.llm_seeds)
    mine, my_dups = _dedupe_seeds(req.my_seeds)
    mixed, cross_dups = _dedupe_seeds(llm + mine)
    weak = [s for s in mixed if len(s.split()) < 2 or len(s) < 8]
    duplicates = llm_dups + my_dups + cross_dups
    breadth = "low"
    if len(mixed) >= 25:
        breadth = "high"
    elif len(mixed) >= 12:
        breadth = "medium"
    return SeedValidateResponse(
        deduped_llm=llm,
        deduped_my=mine,
        mixed=mixed,
        duplicates=duplicates,
        weak=weak,
        breadth_estimate=breadth,
    )


@app.post("/v1/verticals/{vertical}/run", response_model=ContinueVerticalResponse)
async def vertical_run(
    vertical: str,
    req: VerticalRunRequest,
    x_campaign_start_key: str | None = Header(default=None, alias="X-Campaign-Start-Key"),
) -> ContinueVerticalResponse:
    auth_key = (req.campaign_start_key or "").strip() or x_campaign_start_key
    return await _start_vertical_mode(
        vertical=vertical,
        mode=req.mode,
        x_campaign_start_key=auth_key,
        seeds=req.seeds,
        cluster=req.cluster,
    )


@app.get("/v1/verticals/{vertical}/run-status", response_model=VerticalRunStatusResponse)
async def vertical_run_status(
    vertical: str,
    campaign_id: str,
    workflow_id: str,
    run_mode: str = "breadth",
    cluster_key: str | None = None,
) -> VerticalRunStatusResponse:
    v = (vertical or "").strip().lower() or "general"
    status, wf_error = await _workflow_status(workflow_id)
    snap = _run_snapshot(campaign_id)
    return VerticalRunStatusResponse(
        ok=True,
        workflow_id=workflow_id,
        campaign_id=campaign_id,
        vertical=v,
        run_mode=run_mode,
        cluster_key=(cluster_key or "").strip() or None,
        status=status,
        iteration=int(snap.get("iteration") or 0),
        keyword_count=int(snap.get("keyword_count") or 0),
        new_keyword_count=int(snap.get("new_keyword_count") or 0),
        clusters_seen=int(snap.get("clusters_seen") or 0),
        updated_at=(snap.get("updated_at") or None),
        error=wf_error,
    )


@app.get("/verticals/{vertical}", response_class=HTMLResponse)
def vertical_dashboard_page(vertical: str) -> str:
    v = (vertical or "").strip().lower() or "general"
    return f"""
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>SEO OS Vertical Dashboard — {v}</title>
  <style>
    body {{ font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif; margin: 0; background: #0d1117; color: #e6edf3; }}
    .wrap {{ max-width: 1200px; margin: 0 auto; padding: 20px; }}
    .row {{ display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 12px; margin-bottom: 14px; }}
    .card {{ background: #161b22; border: 1px solid #30363d; border-radius: 12px; padding: 14px; }}
    .kpi {{ font-size: 30px; font-weight: 700; }}
    .muted {{ color: #8b949e; font-size: 12px; }}
    button {{ border: 1px solid #30363d; background: #21262d; color: #e6edf3; border-radius: 10px; padding: 10px 14px; cursor: pointer; }}
    button:hover {{ background: #30363d; }}
    input {{ border: 1px solid #30363d; background: #0d1117; color: #e6edf3; border-radius: 8px; padding: 8px; width: 100%; }}
    .grid {{ display: grid; grid-template-columns: 380px 1fr; gap: 12px; }}
    .list {{ max-height: 70vh; overflow: auto; }}
    .cluster {{ padding: 8px; border-radius: 8px; margin-bottom: 6px; cursor: pointer; border: 1px solid transparent; }}
    .cluster:hover {{ background: #1f2630; border-color: #30363d; }}
    .cluster.active {{ background: #1f2630; border-color: #58a6ff; }}
    .kw {{ padding: 6px 0; border-bottom: 1px dashed #30363d; }}
    .tabs {{ display: flex; gap: 8px; margin-bottom: 12px; }}
    .tab-btn {{ padding: 8px 12px; }}
    .tab-btn.active {{ border-color: #58a6ff; background: #1f2630; }}
    .tab-content {{ display: none; }}
    .tab-content.active {{ display: block; }}
    .status-box {{ border: 1px solid #30363d; border-radius: 10px; padding: 10px; }}
    .status-ok {{ border-color: #238636; color: #7ee787; }}
    .status-err {{ border-color: #f85149; color: #ffb3ad; }}
  </style>
  <script>
    let state = null;
    let selected = null;
    let activeRun = null;
    let statusPollTimer = null;

    async function loadState() {{
      const res = await fetch('/v1/verticals/{v}/state');
      if (!res.ok) {{
        document.getElementById('status').textContent = 'Ошибка загрузки state: ' + res.status;
        return;
      }}
      state = await res.json();
      document.getElementById('totalKeywords').textContent = state.total_keywords;
      document.getElementById('totalClusters').textContent = state.total_clusters;
      document.getElementById('generatedAt').textContent = state.generated_at;
      renderClusters();
      if (state.clusters.length > 0) selectCluster(state.clusters[0].cluster);
    }}

    function loadSavedSecret() {{
      try {{
        const saved = localStorage.getItem('seoos_campaign_start_secret') || '';
        if (saved) {{
          document.getElementById('startKey').value = saved;
        }}
      }} catch (e) {{
        // ignore storage issues
      }}
    }}

    function saveSecret() {{
      try {{
        const v = document.getElementById('startKey').value.trim();
        if (v) {{
          localStorage.setItem('seoos_campaign_start_secret', v);
        }}
      }} catch (e) {{
        // ignore storage issues
      }}
    }}

    function clearSecret() {{
      try {{
        localStorage.removeItem('seoos_campaign_start_secret');
      }} catch (e) {{
        // ignore storage issues
      }}
      document.getElementById('startKey').value = '';
      document.getElementById('status').textContent = 'CAMPAIGN_START_SECRET очищен в браузере.';
    }}

    function ensureSecretOrShowInlineHelp() {{
      const key = document.getElementById('startKey').value.trim();
      if (key) return true;
      document.getElementById('status').textContent =
        'Нужна однократная настройка: вставь CAMPAIGN_START_SECRET в форму ниже и нажми Сохранить.';
      document.getElementById('secretSetup').style.display = 'block';
      document.getElementById('status').classList.add('status-err');
      return false;
    }}

    function setStatus(message, isOk = false) {{
      const el = document.getElementById('status');
      el.textContent = message;
      el.classList.remove('status-ok', 'status-err');
      el.classList.add(isOk ? 'status-ok' : 'status-err');
    }}

    function renderRunTelemetry(run) {{
      const box = document.getElementById('runTelemetry');
      if (!run) {{
        box.textContent = 'Нет активного запуска.';
        return;
      }}
      box.textContent =
        `mode=${{run.run_mode}} | status=${{run.status}} | iteration=${{run.iteration}} | ` +
        `keywords=${{run.keyword_count}} | new=${{run.new_keyword_count}} | clusters_seen=${{run.clusters_seen}}` +
        (run.updated_at ? ` | updated_at=${{run.updated_at}}` : '') +
        (run.error ? `\\nerror=${{run.error}}` : '');
    }}

    function stopRunPolling() {{
      if (statusPollTimer) {{
        clearTimeout(statusPollTimer);
        statusPollTimer = null;
      }}
    }}

    async function pollRunStatus() {{
      if (!activeRun) return;
      const p = new URLSearchParams({{
        campaign_id: activeRun.campaign_id,
        workflow_id: activeRun.workflow_id,
        run_mode: activeRun.run_mode || activeRun.mode || 'breadth',
      }});
      if (activeRun.cluster_key) p.set('cluster_key', activeRun.cluster_key);
      const res = await fetch(`/v1/verticals/{v}/run-status?` + p.toString());
      const data = await res.json();
      if (!res.ok) {{
        setStatus('Ошибка polling run-status: ' + JSON.stringify(data), false);
        stopRunPolling();
        return;
      }}
      renderRunTelemetry(data);
      const st = (data.status || '').toLowerCase();
      if (st === 'done' || st === 'error') {{
        setStatus(`Run завершен: status=${{data.status}}`, st === 'done');
        stopRunPolling();
        await loadState();
        return;
      }}
      statusPollTimer = setTimeout(pollRunStatus, 2500);
    }}

    function startRunPolling(runInfo) {{
      activeRun = runInfo;
      stopRunPolling();
      renderRunTelemetry({{
        run_mode: runInfo.run_mode || runInfo.mode || 'breadth',
        status: 'queued',
        iteration: 0,
        keyword_count: 0,
        new_keyword_count: 0,
        clusters_seen: 0,
      }});
      statusPollTimer = setTimeout(pollRunStatus, 1000);
    }}

    function switchTab(tabName) {{
      const tabs = ['seedsTab', 'clustersTab'];
      for (const t of tabs) {{
        document.getElementById(t).classList.remove('active');
      }}
      document.getElementById(tabName).classList.add('active');
      const btns = ['tabSeedsBtn', 'tabClustersBtn'];
      for (const b of btns) {{
        document.getElementById(b).classList.remove('active');
      }}
      document.getElementById(tabName === 'seedsTab' ? 'tabSeedsBtn' : 'tabClustersBtn').classList.add('active');
    }}

    function renderClusters() {{
      const root = document.getElementById('clusters');
      root.innerHTML = '';
      for (const c of state.clusters) {{
        const div = document.createElement('div');
        div.className = 'cluster' + (selected === c.cluster ? ' active' : '');
        div.onclick = () => selectCluster(c.cluster);
        div.innerHTML = `<div><b>${{c.cluster}}</b></div><div class="muted">keywords=${{c.keyword_count}}</div>`;
        root.appendChild(div);
      }}
    }}

    function selectCluster(name) {{
      selected = name;
      renderClusters();
      const c = state.clusters.find(x => x.cluster === name);
      const root = document.getElementById('keywords');
      root.innerHTML = '';
      document.getElementById('selectedCluster').textContent = name;
      if (!c) return;
      for (const k of c.keywords) {{
        const div = document.createElement('div');
        div.className = 'kw';
        div.textContent = k;
        root.appendChild(div);
      }}
    }}

    function _splitSeeds(text) {{
      return text.split(/[\\n,]/).map(x => x.trim()).filter(Boolean);
    }}

    async function chatSuggest() {{
      const prompt = document.getElementById('chatPrompt').value.trim();
      const body = {{
        message: prompt || 'предложи seeds по game providers, casino platforms, casino brands',
        current_llm_seeds: _splitSeeds(document.getElementById('llmSeeds').value),
        current_my_seeds: _splitSeeds(document.getElementById('mySeeds').value),
      }};
      const res = await fetch('/v1/verticals/{v}/seeds/chat-suggest', {{
        method: 'POST',
        headers: {{ 'Content-Type': 'application/json' }},
        body: JSON.stringify(body),
      }});
      const data = await res.json();
      if (!res.ok) {{
        document.getElementById('seedWarnings').textContent = JSON.stringify(data);
        return;
      }}
      document.getElementById('llmSeeds').value = (data.seed_suggestions || []).join('\\n');
      document.getElementById('seedWarnings').textContent = (data.warnings || []).join('\\n');
    }}

    async function validateSeeds() {{
      const body = {{
        llm_seeds: _splitSeeds(document.getElementById('llmSeeds').value),
        my_seeds: _splitSeeds(document.getElementById('mySeeds').value),
      }};
      const res = await fetch('/v1/verticals/{v}/seeds/validate', {{
        method: 'POST',
        headers: {{ 'Content-Type': 'application/json' }},
        body: JSON.stringify(body),
      }});
      const data = await res.json();
      if (!res.ok) {{
        document.getElementById('seedWarnings').textContent = JSON.stringify(data);
        return;
      }}
      document.getElementById('llmSeeds').value = (data.deduped_llm || []).join('\\n');
      document.getElementById('mySeeds').value = (data.deduped_my || []).join('\\n');
      document.getElementById('seedWarnings').textContent =
        `duplicates=${{(data.duplicates || []).length}}, weak=${{(data.weak || []).length}}, breadth=${{data.breadth_estimate}}`;
      window.__mixedSeeds = data.mixed || [];
    }}

    function useMixed() {{
      const mixed = window.__mixedSeeds || [];
      if (!mixed.length) {{
        document.getElementById('seedWarnings').textContent = 'Сначала нажми Validate & dedupe';
        return;
      }}
      document.getElementById('llmSeeds').value = mixed.join('\\n');
    }}

    async function runMode(mode, seeds = null, cluster = null) {{
      const key = document.getElementById('startKey').value.trim();
      if (!ensureSecretOrShowInlineHelp()) {{
        return;
      }}
      saveSecret();
      const payload = {{
        mode,
        seeds: seeds || _splitSeeds(document.getElementById('llmSeeds').value),
        cluster: cluster || undefined,
        campaign_start_key: key,
      }};
      const res = await fetch('/v1/verticals/{v}/run', {{
        method: 'POST',
        headers: {{
          'Content-Type': 'application/json',
        }},
        body: JSON.stringify(payload),
      }});
      const txt = await res.text();
      console.log('runMode response', res.status, txt);
      if (res.ok) {{
        const data = JSON.parse(txt);
        setStatus(
          `Запуск создан: mode=${{data.run_mode || data.mode}}, workflow_id=${{data.workflow_id}}, campaign_id=${{data.campaign_id}}`,
          true
        );
        startRunPolling(data);
      }} else if (res.status === 401 || res.status === 403) {{
        document.getElementById('secretSetup').style.display = 'block';
        setStatus(txt, false);
      }} else {{
        setStatus(txt, false);
      }}
    }}

    function runBreadthFromSeeds() {{
      runMode('breadth');
      switchTab('clustersTab');
    }}

    function runDepthForSelected() {{
      if (!selected) {{
        alert('Выбери кластер');
        return;
      }}
      const c = state?.clusters?.find(x => x.cluster === selected);
      const seeds = c ? c.keywords.slice(0, 20) : null;
      runMode('depth', seeds, selected);
      switchTab('clustersTab');
    }}

    window.onload = () => {{
      switchTab('seedsTab');
      loadSavedSecret();
      if (document.getElementById('startKey').value.trim()) {{
        document.getElementById('secretSetup').style.display = 'none';
        setStatus('Ключ найден в браузере. Можно запускать поиск.', true);
      }} else {{
        setStatus('Для запуска поиска нужен CAMPAIGN_START_SECRET.', false);
      }}
      loadState();
    }};
  </script>
</head>
<body>
  <div class="wrap">
    <h2>Vertical Dashboard: {v}</h2>
    <div class="row">
      <div class="card"><div class="muted">Total Keywords</div><div id="totalKeywords" class="kpi">-</div></div>
      <div class="card"><div class="muted">Total Clusters</div><div id="totalClusters" class="kpi">-</div></div>
      <div class="card"><div class="muted">Generated At</div><div id="generatedAt">-</div></div>
    </div>
    <div id="secretSetup" class="card" style="margin-bottom: 12px; display:none;">
      <div class="muted" style="margin-bottom: 6px;">Однократная настройка: CAMPAIGN_START_SECRET</div>
      <input id="startKey" placeholder="Вставь CAMPAIGN_START_SECRET сюда (один раз)" />
      <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap;">
        <button onclick="saveSecret(); document.getElementById('secretSetup').style.display='none'; setStatus('Ключ сохранен. Теперь можно запускать поиск одной кнопкой.', true);">Сохранить и скрыть</button>
        <button onclick="clearSecret()">Очистить ключ</button>
      </div>
      <div class="muted" style="margin-top:6px;">
        Ключ сохраняется локально в этом браузере. В обычной работе этот блок можно не показывать.
      </div>
    </div>
    <div id="status" class="muted status-box" style="margin-top: 10px; margin-bottom: 12px; white-space: pre-wrap;"></div>
    <div id="runTelemetry" class="muted status-box" style="margin-top: 10px; margin-bottom: 12px; white-space: pre-wrap;">Нет активного запуска.</div>
    <div class="tabs">
      <button id="tabSeedsBtn" class="tab-btn" onclick="switchTab('seedsTab')">Seeds Studio</button>
      <button id="tabClustersBtn" class="tab-btn" onclick="switchTab('clustersTab')">Clusters & Keywords</button>
    </div>

    <div id="seedsTab" class="tab-content">
      <div class="grid" style="grid-template-columns: 1fr 1fr; margin-bottom: 12px;">
        <div class="card">
          <div class="muted">Seeds Studio — LLM предложил seeds</div>
          <textarea id="llmSeeds" style="width:100%;height:140px;background:#0d1117;color:#e6edf3;border:1px solid #30363d;border-radius:8px;margin-top:8px;"></textarea>
          <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap;">
            <button onclick="chatSuggest()">Regenerate seeds</button>
            <button onclick="validateSeeds()">Validate & dedupe</button>
            <button onclick="useMixed()">Use mixed (LLM + mine)</button>
            <button onclick="runBreadthFromSeeds()">Continue Breadth</button>
            <button onclick="switchTab('clustersTab')">Перейти к кластерам</button>
          </div>
          <div id="seedWarnings" class="muted" style="margin-top:8px;white-space:pre-wrap;"></div>
        </div>
        <div class="card">
          <div class="muted">Мои seeds</div>
          <textarea id="mySeeds" style="width:100%;height:140px;background:#0d1117;color:#e6edf3;border:1px solid #30363d;border-radius:8px;margin-top:8px;"></textarea>
          <div class="muted" style="margin-top:8px;">Chat Copilot prompt</div>
          <input id="chatPrompt" placeholder="предложи seeds по game providers / casino platforms / casino brands" style="margin-top:6px;" />
        </div>
      </div>
    </div>

    <div id="clustersTab" class="tab-content">
      <div class="grid">
        <div class="card list">
          <div class="muted" style="margin-bottom: 8px;">Clusters</div>
          <div id="clusters"></div>
        </div>
        <div class="card list">
          <div class="muted" style="margin-bottom: 8px;">Keywords in Cluster: <span id="selectedCluster">-</span></div>
          <div style="margin-bottom:8px;">
            <button onclick="runDepthForSelected()">Continue Depth for Selected Cluster</button>
          </div>
          <div id="keywords"></div>
        </div>
      </div>
    </div>
  </div>
</body>
</html>
    """
