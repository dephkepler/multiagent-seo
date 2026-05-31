"""Telegram-бот: сообщения → complete_chat (тот же OpenAI, что и /v1/chat)."""
from __future__ import annotations

import asyncio
import logging
import os
from typing import FrozenSet

from telegram import Update
from telegram.error import TelegramError
from telegram.ext import Application, CommandHandler, ContextTypes, MessageHandler, filters

from seo_os.api.chat_backend import complete_chat
from seo_os.bots import campaign_context
from seo_os.bots.campaign_context import UserCampaignContext
from seo_os.bots.keyword_card_flow import (
    default_keyword_params,
    enrich_keyword_params,
    keyword_card_description,
    run_trello_todo_once,
)
from seo_os.bots.keyword_probe import (
    probe_keyword_health,
    probe_keyword_live_autocomplete,
    probe_keyword_live_trends,
    probe_keyword_live_youtube,
    probe_keyword_search,
    run_keyword_api_probe,
    split_telegram_chunks,
)
from seo_os.integrations.trello import TrelloClient, TrelloConfigError, TrelloListIds
from seo_os.temporal.campaign_starter import start_seo_campaign_fire_and_forget

logger = logging.getLogger(__name__)

TG_MAX_MESSAGE = 4096
KW_NEW_STATE_KEY = "kw_new_state"
KW_NEW_QUESTIONS: list[tuple[str, str]] = [
    ("vertical", "Вкажи vertical (приклад: gambling або health):"),
    ("KEYWORD_QUERY", "Вкажи KEYWORD_QUERY (приклад: anti age products). Якщо не знаєш — напиши '-' :"),
    (
        "KEYWORD_SEEDS",
        "Вкажи KEYWORD_SEEDS через кому (приклад: anti age,anti aging products,skin care). Якщо не знаєш — '-' :",
    ),
    ("KEYWORD_SOURCE_PROFILE", "Вкажи source profile: cheap | balanced | ahrefs_validate (enter для cheap):"),
    ("KEYWORD_TARGET_CLUSTERS", "Вкажи KEYWORD_TARGET_CLUSTERS (число):"),
    ("KEYWORD_TARGET_KEYWORDS", "Вкажи KEYWORD_TARGET_KEYWORDS (число):"),
    ("KEYWORD_MAX_ITERATIONS", "Вкажи KEYWORD_MAX_ITERATIONS (число):"),
]


def _allowed_ids_from_env() -> FrozenSet[int] | None:
    raw = (os.environ.get("TELEGRAM_ALLOWED_USER_IDS") or "").strip()
    if not raw:
        return None
    out: set[int] = set()
    for part in raw.split(","):
        p = part.strip()
        if p:
            out.add(int(p))
    return frozenset(out)


def _truncate_for_telegram(text: str) -> str:
    if len(text) <= TG_MAX_MESSAGE:
        return text
    return text[: TG_MAX_MESSAGE - 40] + "\n\n… (сообщение обрезано)"


def _tg_safe(text: str) -> str:
    return text.replace("\x00", "")


async def _denied_sensitive_commands(update: Update, context: ContextTypes.DEFAULT_TYPE) -> bool:
    """True, якщо доступ заборонено (і вже відповіли в чат). Для /campaign і Keyword Adapter-команд."""
    if not update.message:
        return True
    allowed = context.bot_data.get("allowed_user_ids")
    user = update.effective_user
    if allowed is None or len(allowed) == 0:
        await update.message.reply_text(
            "Команда відключена з міркувань безпеки: задайте в .env TELEGRAM_ALLOWED_USER_IDS "
            "(числовий user id з Telegram), перезапустіть бота."
        )
        return True
    if user is None or user.id not in allowed:
        await update.message.reply_text(
            "Команда доступна лише користувачам з TELEGRAM_ALLOWED_USER_IDS."
        )
        return True
    return False


async def cmd_ping(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Проверка только связи с Telegram (без OpenAI)."""
    if not update.message:
        return
    await update.message.reply_text(
        "OK: бот получил команду от Telegram. Связь с Telegram работает.\n"
        "Ошибки OpenAI к этому не относятся — смотри /status."
    )


async def cmd_campaign(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Старт SeoCampaignWorkflow (как seo-os-demo), без ожидания завершения."""
    if not update.message:
        return
    if await _denied_sensitive_commands(update, context):
        return
    user = update.effective_user
    if not (os.environ.get("DATABASE_URL") or "").strip():
        await update.message.reply_text("Нет DATABASE_URL — задайте в .env (Postgres).")
        return
    await update.message.reply_text(
        "Запускаю SeoCampaignWorkflow… Нужны запущенные Temporal и seo-os-worker."
    )
    chat = update.effective_chat
    tg_chat = str(chat.id) if chat else ""
    try:
        cid, wid = await start_seo_campaign_fire_and_forget(telegram_chat_id=tg_chat or None)
    except Exception as e:
        logger.exception("start_seo_campaign_fire_and_forget")
        await update.message.reply_text(f"Ошибка старта workflow: {e}")
        return
    if user:
        campaign_context.bind(user.id, str(cid), wid)
    board = (os.environ.get("TRELLO_BOARD_SHORT_LINK") or "").strip() or "—"
    await update.message.reply_text(
        f"Кампания запущена.\n"
        f"campaign_id={cid}\n"
        f"workflow_id={wid}\n"
        f"Trello board short: {board}\n"
        "Отчёты о фазах придут в этот чат (если задан TELEGRAM_BOT_TOKEN и worker).\n"
        "Диалог в контексте кампании включён — /context, сброс: /context_off."
    )


async def cmd_status(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Показать, задан ли ключ OpenAI (без раскрытия значения)."""
    if not update.message:
        return
    has_oai = bool((os.environ.get("OPENAI_API_KEY") or "").strip())
    oai_line = "OPENAI_API_KEY в .env: задан (ответы модели возможны)." if has_oai else (
        "OPENAI_API_KEY в .env: не задан — будет заглушка или ошибка API."
    )
    await update.message.reply_text(
        "Telegram: процесс бота запущен и видит команды.\n"
        f"{oai_line}\n\n"
        "Внешние сервисы (OpenClaw и др.) не имеют доступа к твоему боту, "
        "если в BotFather выдан новый TELEGRAM_BOT_TOKEN и старый отозван."
    )


async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    await update.message.reply_text(
        "Привет! Я бот SEO OS. Напиши обычным текстом — отвечу через OpenAI "
        "(нужен OPENAI_API_KEY на машине с ботом).\n\n"
        "Надёжнее писать в личные сообщения боту (не в группу). "
        "В группах бот часто не видит обычные сообщения: BotFather → Bot Settings → "
        "Group Privacy → Turn off, либо упомяни бота в сообщении.\n\n"
        "Принимаю только текст (не голос и не фото без подписи).\n\n"
        "Проверка: /ping — Telegram; /status — OpenAI.\n"
        "Кампания SEO OS: /campaign (нужны TELEGRAM_ALLOWED_USER_IDS, Temporal, worker, Trello в .env).\n"
        "Keyword Adapter: /kw_help — список команд; /kw_probe [запит] — усі ендпоінти; "
        "окремо: /kw_health, /kw_search, /kw_autocomplete, /kw_trends, /kw_youtube "
        "(потрібні TELEGRAM_ALLOWED_USER_IDS і KEYWORD_SEARCH_BASE_URL у .env бота).\n"
        "Trello flow: /kw_new — створити картку в Backlog (InBox) з KEYWORD_*; "
        "/kw_cancel — скасувати опитування; /kw_poll_once — вручну обробити одну картку з ToDo.\n"
        "После /campaign — вопросы по кампании в том же чате; /context — показать привязку, /context_off — выключить."
    )


def _kw_new_prompt(step_idx: int) -> str:
    _, q = KW_NEW_QUESTIONS[step_idx]
    return f"[{step_idx + 1}/{len(KW_NEW_QUESTIONS)}] {q}"


async def cmd_kw_cancel(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    context.user_data.pop(KW_NEW_STATE_KEY, None)
    await update.message.reply_text("Опрос /kw_new отменён.")


async def cmd_kw_new(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    if await _denied_sensitive_commands(update, context):
        return
    defaults = default_keyword_params()
    vertical = (_kw_query_from_args(context) or "general").strip()
    context.user_data[KW_NEW_STATE_KEY] = {
        "step": 0,
        "vertical": vertical,
        "params": defaults,
    }
    await update.message.reply_text(
        "Створюю задачу для Backlog. Відповідай одним повідомленням на кожен крок.\n"
        "Щоб скасувати: /kw_cancel\n\n"
        + _kw_new_prompt(0)
    )


async def cmd_kw_poll_once(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Один ручний цикл ToDo -> InProgress -> start workflow."""
    if not update.message:
        return
    if await _denied_sensitive_commands(update, context):
        return
    await update.message.reply_text("Запускаю один цикл poller…")
    chat = update.effective_chat
    tg_chat = str(chat.id) if chat else None
    try:
        result = await run_trello_todo_once(telegram_chat_id=tg_chat)
    except Exception as e:
        logger.exception("kw_poll_once failed")
        await update.message.reply_text(f"Помилка /kw_poll_once: {e}")
        return
    await update.message.reply_text(result)


def _create_trello_keyword_card(*, title: str, desc: str) -> dict:
    trello = TrelloClient.from_env()
    lists = TrelloListIds.from_env()
    return trello.create_card(id_list=lists.inbox, name=title, desc=desc)


async def _finish_kw_new(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    st = context.user_data.get(KW_NEW_STATE_KEY)
    if not isinstance(st, dict):
        return
    vertical = str(st.get("vertical") or "general")
    params = dict(st.get("params") or {})
    params = enrich_keyword_params(params, vertical=vertical)
    user = update.effective_user
    chat = update.effective_chat
    tg_chat_id = str(chat.id) if chat else None
    created_by = str(user.id) if user else "unknown"
    desc = keyword_card_description(
        params,
        vertical=vertical,
        created_by=created_by,
        telegram_chat_id=tg_chat_id,
    )
    title = f"KW {vertical}: {params.get('KEYWORD_QUERY', 'query')}"
    try:
        card = await asyncio.to_thread(_create_trello_keyword_card, title=title[:120], desc=desc)
    except TrelloConfigError as e:
        await update.message.reply_text(f"Trello не сконфігурований: {e}")
        return
    except Exception as e:
        logger.exception("kw_new create trello card")
        await update.message.reply_text(f"Не вдалося створити картку: {e}")
        return
    finally:
        context.user_data.pop(KW_NEW_STATE_KEY, None)
    url = str(card.get("shortUrl") or card.get("url") or "")
    await update.message.reply_text(
        "Картку створено в Backlog (TRELLO_LIST_INBOX).\n"
        f"card_id={card.get('id')}\n"
        f"url={url or '—'}\n"
        "Перетягни її в ToDo queue — poller забере у роботу."
    )


def _kw_query_from_args(context: ContextTypes.DEFAULT_TYPE) -> str:
    return ((" ".join(context.args).strip()) if context.args else "").strip()


async def _kw_require_query(
    update: Update,
    context: ContextTypes.DEFAULT_TYPE,
    *,
    command: str,
) -> str | None:
    query = _kw_query_from_args(context)
    if query:
        return query
    if update.message:
        await update.message.reply_text(
            f"Вкажи текст запиту після команди. Приклад: /{command} best crash games"
        )
    return None


async def _reply_keyword_report(
    update: Update,
    context: ContextTypes.DEFAULT_TYPE,
    status_line: str,
    report_fn,
) -> None:
    if not update.message:
        return
    if await _denied_sensitive_commands(update, context):
        return
    await update.message.reply_text(status_line)
    try:
        report = await asyncio.to_thread(report_fn)
    except Exception as e:
        logger.exception("keyword probe report")
        await update.message.reply_text(f"Помилка: {e}")
        return
    for chunk in split_telegram_chunks(_tg_safe(report)):
        try:
            await update.message.reply_text(chunk)
        except TelegramError:
            logger.exception("keyword report chunk")


async def cmd_kw_help(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Короткий список команд Keyword Adapter."""
    if not update.message:
        return
    if await _denied_sensitive_commands(update, context):
        return
    await update.message.reply_text(
        "Keyword Adapter (сирі JSON-відповіді в чат):\n"
        "/kw_health — GET /health\n"
        "/kw_search [запит] — POST /v1/search (cheap_live: autocomplete + youtube)\n"
        "/kw_autocomplete або /kw_ac [запит] — POST /v1/live/autocomplete\n"
        "/kw_trends [запит] — POST /v1/live/trends, тренди по твоєму тексту (trends_seed_mode=only_query; може бути довго)\n"
        "/kw_youtube або /kw_yt [запит] — POST /v1/live/youtube\n"
        "/kw_probe [запит] — усі ендпоінти послід\n"
        "Для /kw_search, /kw_autocomplete, /kw_trends, /kw_youtube, /kw_probe запит обов'язковий."
    )


async def cmd_kw_health(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    await _reply_keyword_report(
        update,
        context,
        "GET /health…",
        probe_keyword_health,
    )


async def cmd_kw_search(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    q = await _kw_require_query(update, context, command="kw_search")
    if not q:
        return
    await _reply_keyword_report(
        update,
        context,
        f"POST /v1/search, query={q!r}…",
        lambda: probe_keyword_search(q),
    )


async def cmd_kw_autocomplete(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    q = await _kw_require_query(update, context, command="kw_autocomplete")
    if not q:
        return
    await _reply_keyword_report(
        update,
        context,
        f"POST /v1/live/autocomplete, query={q!r}…",
        lambda: probe_keyword_live_autocomplete(q),
    )


async def cmd_kw_trends(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    q = await _kw_require_query(update, context, command="kw_trends")
    if not q:
        return
    await _reply_keyword_report(
        update,
        context,
        f"POST /v1/live/trends, query={q!r} (може тривати хвилини)…",
        lambda: probe_keyword_live_trends(q),
    )


async def cmd_kw_youtube(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    q = await _kw_require_query(update, context, command="kw_youtube")
    if not q:
        return
    await _reply_keyword_report(
        update,
        context,
        f"POST /v1/live/youtube, query={q!r}…",
        lambda: probe_keyword_live_youtube(q),
    )


async def cmd_kw_probe(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Усі ендпоінти Keyword Adapter послід."""
    query = await _kw_require_query(update, context, command="kw_probe")
    if not query:
        return
    await _reply_keyword_report(
        update,
        context,
        "Усі ендпоінти: GET /health, POST /v1/search, /v1/live/autocomplete, "
        "/v1/live/trends, /v1/live/youtube. Останні два можуть тривати хвилини…",
        lambda: run_keyword_api_probe(query),
    )


def _dialogue_prefix(ctx: UserCampaignContext) -> str:
    return (
        "[Контекст кампании SEO OS]\n"
        f"campaign_id={ctx.campaign_id}\n"
        f"workflow_id={ctx.workflow_id}\n"
        "Сообщение пользователя ниже относится к этой кампании."
    )


async def cmd_context(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    user = update.effective_user
    if user is None:
        return
    ctx = campaign_context.get_ctx(user.id)
    if ctx is None:
        await update.message.reply_text(
            "Контекст кампании не задан. Запусти /campaign или сбрось /context_off и снова /campaign."
        )
        return
    await update.message.reply_text(
        f"Активный контекст:\n"
        f"campaign_id={ctx.campaign_id}\n"
        f"workflow_id={ctx.workflow_id}\n"
        "Обычные сообщения идут в LLM с этим префиксом. /context_off — отключить."
    )


async def cmd_context_off(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    user = update.effective_user
    if user is None:
        return
    campaign_context.unbind(user.id)
    await update.message.reply_text("Контекст кампании сброшен. Дальше — обычный чат без привязки к workflow.")


def _message_body(m) -> str:
    """Текст сообщения или подпись к фото/документу."""
    if not m:
        return ""
    return (m.text or m.caption or "").strip()


async def on_text(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    body = _message_body(update.message)
    if not body:
        return
    state = context.user_data.get(KW_NEW_STATE_KEY)
    if isinstance(state, dict):
        step = int(state.get("step") or 0)
        key, _ = KW_NEW_QUESTIONS[step]
        if key == "vertical":
            state["vertical"] = body.strip() or "general"
        else:
            params = dict(state.get("params") or {})
            params[key] = body.strip()
            state["params"] = params
        step += 1
        if step >= len(KW_NEW_QUESTIONS):
            await _finish_kw_new(update, context)
            return
        state["step"] = step
        context.user_data[KW_NEW_STATE_KEY] = state
        await update.message.reply_text(_kw_new_prompt(step))
        return
    allowed = context.bot_data.get("allowed_user_ids")
    user = update.effective_user
    chat = update.effective_chat
    logger.info(
        "Входящее текстовое сообщение: user_id=%s chat_id=%s type=%s",
        user.id if user else None,
        chat.id if chat else None,
        chat.type if chat else None,
    )
    if allowed is not None and user is not None and user.id not in allowed:
        await update.message.reply_text("Доступ к этому боту ограничен.")
        return

    session_id = str(user.id) if user else None
    dctx = campaign_context.get_ctx(user.id) if user else None
    prefix = _dialogue_prefix(dctx) if dctx else None
    try:
        result = await asyncio.to_thread(
            complete_chat,
            body,
            session_id,
            dialogue_context=prefix,
        )
    except Exception:
        logger.exception("complete_chat failed")
        await update.message.reply_text("Внутренняя ошибка при обработке сообщения.")
        return

    text = _tg_safe(_truncate_for_telegram(result.reply))
    if not text.strip():
        text = "(Пустой ответ модели.)"
    try:
        await update.message.reply_text(text)
        logger.info("Ответ отправлен в Telegram, длина=%s", len(text))
    except TelegramError:
        logger.exception("Не удалось отправить ответ в Telegram")


async def on_non_text(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Голос, фото и т.д. — подсказка, чтобы не было «тишины»."""
    if not update.message:
        return
    await update.message.reply_text(
        "Пока принимаю только обычный текст. Если пишешь в группе — см. /start "
        "(privacy бота или упоминание)."
    )


async def job_trello_todo_poller(context: ContextTypes.DEFAULT_TYPE) -> None:
    chat_id = (os.environ.get("TELEGRAM_REPORT_CHAT_ID") or "").strip() or None
    result = await run_trello_todo_once(telegram_chat_id=chat_id)
    logger.info("trello todo poller: %s", result)


def build_application(token: str) -> Application:
    allowed = _allowed_ids_from_env()
    app = Application.builder().token(token).build()
    app.bot_data["allowed_user_ids"] = allowed
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("ping", cmd_ping))
    app.add_handler(CommandHandler("status", cmd_status))
    app.add_handler(CommandHandler("campaign", cmd_campaign))
    app.add_handler(CommandHandler("context", cmd_context))
    app.add_handler(CommandHandler("context_off", cmd_context_off))
    app.add_handler(CommandHandler("kw_help", cmd_kw_help))
    app.add_handler(CommandHandler("kw_new", cmd_kw_new))
    app.add_handler(CommandHandler("kw_cancel", cmd_kw_cancel))
    app.add_handler(CommandHandler("kw_poll_once", cmd_kw_poll_once))
    app.add_handler(CommandHandler("kw_health", cmd_kw_health))
    app.add_handler(CommandHandler("kw_search", cmd_kw_search))
    app.add_handler(CommandHandler(["kw_autocomplete", "kw_ac"], cmd_kw_autocomplete))
    app.add_handler(CommandHandler("kw_trends", cmd_kw_trends))
    app.add_handler(CommandHandler(["kw_youtube", "kw_yt"], cmd_kw_youtube))
    app.add_handler(CommandHandler("kw_probe", cmd_kw_probe))
    poller_enabled = (os.environ.get("TRELLO_TODO_POLLER_ENABLED") or "").strip().lower() in {
        "1",
        "true",
        "yes",
        "on",
    }
    if poller_enabled:
        every = int((os.environ.get("TRELLO_TODO_POLLER_INTERVAL_SEC") or "45").strip() or "45")
        if app.job_queue is None:
            logger.warning(
                "TRELLO_TODO_POLLER_ENABLED=true, but JobQueue is unavailable. "
                "Install with: pip install 'python-telegram-bot[job-queue]'"
            )
        else:
            app.job_queue.run_repeating(job_trello_todo_poller, interval=max(15, every), first=5)
    text_or_caption = filters.TEXT | filters.CAPTION
    app.add_handler(MessageHandler(text_or_caption & ~filters.COMMAND, on_text))
    # Всё остальное (стикер, голос, фото без подписи) — короткая подсказка
    app.add_handler(MessageHandler(~text_or_caption & ~filters.COMMAND, on_non_text))
    return app
