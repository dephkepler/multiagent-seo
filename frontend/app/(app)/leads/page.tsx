'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { LineChart } from '@/components/charts/line-chart'
import { Badge, type Variant as BadgeVariant } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { SectionHeader } from '@/components/ui/section-header'
import { StatTile } from '@/components/ui/stat-tile'
import { CHART_PRIMARY, CHART_SECONDARY, STATUS_CRITICAL, STATUS_GOOD, STATUS_SERIOUS } from '@/lib/chart-colors'
import { cx } from '@/lib/cx'


interface LeadStats {
  range: { from: string; to: string; group_by: string }
  totals: {
    leads: number
    clients: number
    consultations: number
    revenue_booked: number
    revenue_earned: number
    revenue_lost: number
    avg_ticket: number
    cases_in_progress: number
    cases_completed: number
    case_fee_contracted: number
    case_paid: number
    case_owed: number
    site_sessions: number
    organic_sessions: number
  }
  trend: {
    bucket: string
    leads: number
    consultations: number
    revenue_earned: number
    site_sessions: number
    organic_sessions: number
  }[]
  by_source: {
    key: string
    leads: number
    consulted_ever: number
    cased_ever: number
    revenue_earned: number
    case_paid: number
  }[]
  by_creator: { key: string; bookings: number; revenue_earned: number }[]
  by_status: { key: string; count: number }[]
  by_category: { key: string; cases: number; contracted: number; paid: number }[]
  by_advocate: { key: string; cases: number; contracted: number; paid: number }[]
  funnel: {
    cohort_leads: number
    consulted_ever: number
    cased_ever: number
    avg_days_to_consult: number
    avg_days_consult_to_case: number
    first_consult_outcome: { completed: number; cancelled: number; no_show: number; scheduled: number }
  }
  by_weekday: { key: string; count: number }[]
  by_lead_practice_area: { key: string; count: number }[]
  audience: {
    by_age: { key: string; count: number }[]
    by_gender: { key: string; count: number }[]
    by_city: { key: string; count: number }[]
  }
}

const STATUS_META: Record<string, { label: string; bar: string; badge: BadgeVariant }> = {
  scheduled: { label: 'Запланирована', bar: '#a8a6a0', badge: 'neutral' },
  completed: { label: 'Провёл', bar: STATUS_GOOD, badge: 'success' },
  cancelled: { label: 'Отменил', bar: STATUS_CRITICAL, badge: 'danger' },
  no_show: { label: 'Не пришёл', bar: STATUS_SERIOUS, badge: 'warning' },
}

// GA4 marks a dimension it couldn't determine as "(not set)" (age/city) or
// "unknown" (gender) — both read as the same thing to a human, so both map
// to one Russian label instead of leaking GA4's raw wording onto the page.
const AUDIENCE_UNKNOWN: Record<string, string> = { '(not set)': 'Не определено', unknown: 'Не определено' }
const GENDER_LABEL: Record<string, string> = { male: 'Мужчины', female: 'Женщины', ...AUDIENCE_UNKNOWN }
function audienceLabel(key: string): string {
  return AUDIENCE_UNKNOWN[key] ?? key
}

function toISODate(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function daysAgo(n: number): Date {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return d
}

const PRESETS: { label: string; from: () => Date }[] = [
  { label: '7 дней', from: () => daysAgo(7) },
  { label: '30 дней', from: () => daysAgo(30) },
  { label: '90 дней', from: () => daysAgo(90) },
  { label: 'Этот месяц', from: () => new Date(new Date().getFullYear(), new Date().getMonth(), 1) },
  { label: 'Весь период', from: () => new Date('2000-01-01') },
]

function fmtDate(iso: string): string {
  const [y, m, d] = iso.split('-')
  return `${d}.${m}.${y}`
}

function fmtMoney(n: number): string {
  return Math.round(n).toLocaleString('ru-RU') + ' ₴'
}

export default function LeadsPage() {
  const [from, setFrom] = useState(() => toISODate(daysAgo(90)))
  const [to, setTo] = useState(() => toISODate(new Date()))
  const [activePreset, setActivePreset] = useState('90 дней')
  const [pickerOpen, setPickerOpen] = useState(false)
  const pickerRef = useRef<HTMLDivElement>(null)
  const pickerTriggerRef = useRef<HTMLButtonElement>(null)

  // Keyboard users opened the picker from the trigger button — closing it
  // (via Escape, a preset, or "Готово") without sending focus back there
  // would drop focus to <body>, silently resetting their place on the page.
  const closePicker = useCallback(() => {
    setPickerOpen(false)
    pickerTriggerRef.current?.focus()
  }, [])

  const spanDays = useMemo(() => {
    const a = new Date(from).getTime()
    const b = new Date(to).getTime()
    return Math.max(1, Math.round((b - a) / 86400000))
  }, [from, to])
  const groupBy = spanDays <= 45 ? 'day' : 'month'

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['lead-stats', from, to, groupBy],
    queryFn: () => api<LeadStats>(`/leads/stats?from=${from}&to=${to}&group_by=${groupBy}`),
  })

  function applyPreset(p: (typeof PRESETS)[number]) {
    setFrom(toISODate(p.from()))
    setTo(toISODate(new Date()))
    setActivePreset(p.label)
    closePicker()
  }

  // Click outside the picker (button + panel) closes it — the panel itself
  // is positioned inside pickerRef, so any click landing outside that whole
  // subtree means "done".
  useEffect(() => {
    if (!pickerOpen) return
    function onClick(e: MouseEvent) {
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) setPickerOpen(false)
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') closePicker()
    }
    document.addEventListener('mousedown', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [pickerOpen, closePicker])

  return (
    <div className='max-w-6xl space-y-6'>
      <div>
        <SectionHeader title='Заявки и консультации' as='h1' />
        <p className='-mt-3 text-sm text-gray-500'>
          Считается напрямую из данных бота — включает историю до его запуска и растёт с каждой новой заявкой.
        </p>
      </div>

      <div ref={pickerRef} className='relative flex flex-wrap items-center gap-3'>
        <button
          ref={pickerTriggerRef}
          type='button'
          onClick={() => setPickerOpen((v) => !v)}
          aria-haspopup='dialog'
          aria-expanded={pickerOpen}
          className={cx(
            'flex items-center gap-2 rounded-lg border bg-white px-3 py-2 text-sm shadow-sm',
            pickerOpen ? 'border-emerald-400 ring-2 ring-emerald-100' : 'border-gray-200 hover:border-gray-300'
          )}
        >
          <span aria-hidden>📅</span>
          <span className='font-medium'>{activePreset || `${fmtDate(from)} – ${fmtDate(to)}`}</span>
          {activePreset && <span className='hidden text-gray-400 sm:inline'>{fmtDate(from)} – {fmtDate(to)}</span>}
          <span className={cx('text-gray-400 transition-transform', pickerOpen && 'rotate-180')}>▾</span>
        </button>
        <span className='text-xs text-gray-400'>группировка: {groupBy === 'day' ? 'по дням' : 'по месяцам'}</span>

        {pickerOpen && (
          <div className='absolute top-full left-0 z-20 mt-2 flex w-[min(420px,calc(100vw-2.5rem))] flex-col gap-4 rounded-lg border border-gray-200 bg-white p-4 shadow-lg sm:flex-row'>
            <div className='flex flex-col gap-1 sm:w-36 sm:shrink-0'>
              {PRESETS.map((p) => (
                <button
                  key={p.label}
                  type='button'
                  onClick={() => applyPreset(p)}
                  className={cx(
                    'rounded-md px-2 py-1.5 text-left text-sm',
                    activePreset === p.label ? 'bg-emerald-100 font-medium text-emerald-800' : 'text-gray-700 hover:bg-gray-100'
                  )}
                >
                  {p.label}
                </button>
              ))}
            </div>
            <div className='h-px bg-gray-100 sm:h-auto sm:w-px' />
            <div className='flex flex-1 flex-col gap-3'>
              <div className='text-xs font-medium text-gray-500'>Свой период</div>
              <label className='flex flex-col gap-1 text-xs text-gray-500'>
                с
                <input
                  type='date'
                  value={from}
                  onChange={(e) => {
                    setFrom(e.target.value)
                    setActivePreset('')
                  }}
                  className='rounded border border-gray-200 px-2 py-1.5 text-sm text-gray-800'
                />
              </label>
              <label className='flex flex-col gap-1 text-xs text-gray-500'>
                по
                <input
                  type='date'
                  value={to}
                  onChange={(e) => {
                    setTo(e.target.value)
                    setActivePreset('')
                  }}
                  className='rounded border border-gray-200 px-2 py-1.5 text-sm text-gray-800'
                />
              </label>
              <button
                type='button'
                onClick={closePicker}
                className='mt-auto self-end rounded-md bg-emerald-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-600'
              >
                Готово
              </button>
            </div>
          </div>
        )}
      </div>

      {isError && (
        <div role='alert'>
          <Card className='border-red-200 bg-red-50 text-sm text-red-700'>
            Не удалось загрузить статистику{error instanceof Error ? `: ${error.message}` : ''}.
          </Card>
        </div>
      )}

      {isLoading && (
        <div role='status' aria-live='polite'>
          <Card className='text-sm text-gray-500'>Загрузка…</Card>
        </div>
      )}

      {data && (
        <>
          <GroupHeading
            title='Заявки за период'
            subtitle='Сырые числа — включая повторные обращения уже существующих клиентов, не только новых (воронка ниже — только новые)'
          />
          <div className='grid grid-cols-2 gap-4 sm:w-1/2'>
            <StatTile label='Всего заявок' value={data.totals.leads.toLocaleString('ru-RU')} />
            <StatTile label='Уникальных обратившихся' value={data.totals.clients.toLocaleString('ru-RU')} />
          </div>

          <GroupHeading
            title='Воронка'
            subtitle={`Клиенты, первый раз обратившиеся ${fmtDate(data.range.from)} – ${fmtDate(data.range.to)} — что с ними стало с тех пор, неважно когда`}
          />
          <FunnelRow
            leads={data.funnel.cohort_leads}
            consultations={data.funnel.consulted_ever}
            cases={data.funnel.cased_ever}
            daysToConsult={data.funnel.avg_days_to_consult}
            daysToCase={data.funnel.avg_days_consult_to_case}
          />
          <FirstConsultOutcomeBar outcome={data.funnel.first_consult_outcome} />

          <GroupHeading title='Деньги' />
          <div className='space-y-4'>
            <div>
              <div className='mb-2 text-xs text-gray-400'>
                Консультации — сумма самих консультаций (обычно 500–800 ₴ за штуку), отдельно от денег по делам
              </div>
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                <StatTile label='Забронировано (весь потенциал)' value={fmtMoney(data.totals.revenue_booked)} />
                <StatTile
                  label='Заработано (провёл)'
                  value={fmtMoney(data.totals.revenue_earned)}
                  hint={
                    data.totals.avg_ticket > 0
                      ? `средний чек ${fmtMoney(data.totals.avg_ticket)} — цена состоявшейся консультации, не дела`
                      : undefined
                  }
                  accent='good'
                />
                <StatTile label='Потеряно (отмены и неявки)' value={fmtMoney(data.totals.revenue_lost)} accent='bad' />
              </div>
            </div>

            <div>
              <div className='mb-2 text-xs text-gray-400'>
                Дела (клопотання/позов/супровід) — вот тут реальные деньги бизнеса. За выбранный период — сколько
                открыто/законтрактовано/оплачено.
              </div>
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                <StatTile label='Завершено дел' value={data.totals.cases_completed.toLocaleString('ru-RU')} />
                <StatTile label='Законтрактовано' value={fmtMoney(data.totals.case_fee_contracted)} />
                <StatTile
                  label='Получено оплат'
                  value={fmtMoney(data.totals.case_paid)}
                  hint='Растёт по мере частичной оплаты — это уже поступившие деньги, не сумма договора'
                  accent='good'
                />
              </div>

              <div className='mt-4 mb-2 text-xs text-gray-400'>Сейчас, по всем делам — не зависит от периода выше</div>
              <div className='grid grid-cols-2 gap-4'>
                <StatTile label='В работе' value={data.totals.cases_in_progress.toLocaleString('ru-RU')} />
                <StatTile label='Долг клиентов' value={fmtMoney(data.totals.case_owed)} accent={data.totals.case_owed > 0 ? 'bad' : undefined} />
              </div>
            </div>
          </div>

          <GroupHeading title='Качество работы' subtitle='Не только сколько записей, а что с ними стало и кто реально закрывает деньги' />
          <div className='space-y-4'>
            {data.totals.consultations > 0 &&
              (() => {
                const cancelled = data.by_status.find((s) => s.key === 'cancelled')?.count ?? 0
                const noShow = data.by_status.find((s) => s.key === 'no_show')?.count ?? 0
                const lost = cancelled + noShow
                const pct = (lost / data.totals.consultations) * 100
                return (
                  <StatTile
                    label='Отмены и неявки — доля от всех забронированных консультаций'
                    value={`${pct.toFixed(0)}%`}
                    hint={`${lost} из ${data.totals.consultations} (${cancelled} отмен, ${noShow} неявок)`}
                    accent={pct >= 30 ? 'bad' : undefined}
                    emphasize
                  />
                )
              })()}
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <Card className='p-4'>
                <SectionHeader title='Бронирования — кто записывает' />
                <p className='mb-3 text-xs text-gray-400'>
                  Сортировка по заработанному, не по числу записей — можно бронировать много и терять много. По
                  старым записям видно имя сотрудника, по новым пока только Telegram ID — имена ещё не подключены.
                </p>
                <CreatorList rows={data.by_creator} />
              </Card>
              <Card className='p-4'>
                <SectionHeader title='Дела — кто реально закрывает деньги' />
                <p className='mb-3 text-xs text-gray-400'>
                  Дело — на порядок больше денег, чем консультация, и не всегда его ведёт тот же человек, что
                  бронировал слева.
                </p>
                <CategoryList rows={data.by_advocate} emptyLabel='(без адвоката)' />
              </Card>
            </div>
          </div>

          {data.totals.site_sessions > 0 && (
            <>
              <GroupHeading title='Привлечение' subtitle='Сайт, не заявки — воронка выше начинается уже после этого шага' />
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                <StatTile label='Визитов на сайт' value={data.totals.site_sessions.toLocaleString('ru-RU')} />
                <StatTile
                  label='Из поиска (Сео)'
                  value={data.totals.organic_sessions.toLocaleString('ru-RU')}
                  hint={`${Math.round((data.totals.organic_sessions / data.totals.site_sessions) * 100)}% от всего трафика`}
                />
                <StatTile
                  label='Заявка на визит'
                  value={`${((data.totals.leads / data.totals.site_sessions) * 100).toFixed(1)}%`}
                  hint={`${fmtLeads(data.totals.leads)} из ${data.totals.site_sessions} визитов`}
                />
              </div>
            </>
          )}

          {/* Cifры выше открыты всегда; графики свёрнуты по умолчанию — каждый
              открывается своей кнопкой независимо от остальных и остаётся
              открытым, пока не нажать ту же кнопку ещё раз. */}
          <div className='space-y-2'>
            <CollapsibleChart icon='🎯' title='Какое направление приносит больше денег'>
              <p className='mb-4 text-xs text-gray-400'>
                По сумме договора и отдельно — сколько из этого реально оплачено. Направление сотрудник указывает при
                заведении дела в боте.
              </p>
              <CategoryList rows={data.by_category} />
            </CollapsibleChart>

            <CollapsibleChart icon='📨' title='О чём спрашивают на этапе заявки'>
              <p className='mb-4 text-xs text-gray-400'>
                Каждая заявка, а не только те, что дошли до дела (в отличие от графика выше). Направление сотрудник
                ставит кнопкой прямо под уведомлением о заявке в Telegram — «(без направления)» значит ещё не
                отметили.
              </p>
              <HBarList
                rows={data.by_lead_practice_area.map((r) => ({ key: r.key, count: r.count }))}
                emptyLabel='(без направления)'
              />
            </CollapsibleChart>

            <CollapsibleChart icon='📈' title='Обращения по периоду' subtitle={groupBy === 'day' ? 'по дням' : 'по месяцам'}>
              <p className='mb-4 text-xs text-gray-400'>Заявки и забронированные консультации, {groupBy === 'day' ? 'по дням' : 'по месяцам'}</p>
              <TrendChart trend={data.trend} groupBy={data.range.group_by} />
            </CollapsibleChart>

            <CollapsibleChart icon='🗓️' title='В какие дни недели приходят заявки'>
              <p className='mb-4 text-xs text-gray-400'>
                Помогает понять, когда реально нужно, чтобы кто-то отвечал. Час дня сюда специально не добавлен —
                для большей части истории он не сохранился (в старой таблице была только дата), а живых заявок с
                момента бота пока слишком мало для честного вывода.
              </p>
              <WeekdayChart rows={data.by_weekday} />
            </CollapsibleChart>

            <CollapsibleChart icon='💰' title='Выручка по периоду'>
              <p className='mb-4 text-xs text-gray-400'>
                Только проведённые консультации, в деньгах — отдельный график, чтобы не путать суммы со штуками.
              </p>
              <RevenueTrendChart trend={data.trend} groupBy={data.range.group_by} />
            </CollapsibleChart>

            {data.totals.site_sessions > 0 && (
              <CollapsibleChart icon='🌐' title='Трафик сайта по периоду'>
                <p className='mb-4 text-xs text-gray-400'>
                  Все визиты на сайт и отдельно — сколько из поиска. Цифры на порядок больше, чем заявок, поэтому
                  график свой.
                </p>
                <TrafficTrendChart trend={data.trend} groupBy={data.range.group_by} />
              </CollapsibleChart>
            )}

            {(data.audience.by_age.length > 0 || data.audience.by_gender.length > 0 || data.audience.by_city.length > 0) && (
              <CollapsibleChart icon='👥' title='Аудитория сайта — кто заходит'>
                <p className='mb-4 text-xs text-gray-400'>
                  Оценка Google Analytics по всем визитам за период — не привязана к конкретным заявкам (в самой
                  заявке возраст/пол/город пока не фиксируются, это срез по сайту в целом). Возраст и пол — модель
                  Google по поведению визита, не то, что вводит посетитель.
                </p>
                <div className='grid grid-cols-1 gap-6 md:grid-cols-3'>
                  <div>
                    <div className='mb-2 text-xs font-medium text-gray-500'>Возраст</div>
                    <HBarList
                      rows={data.audience.by_age.map((r) => ({ key: audienceLabel(r.key), count: r.count }))}
                      emptyLabel='Не определено'
                    />
                  </div>
                  <div>
                    <div className='mb-2 text-xs font-medium text-gray-500'>Пол</div>
                    <HBarList
                      rows={data.audience.by_gender.map((r) => ({ key: GENDER_LABEL[r.key] ?? r.key, count: r.count }))}
                      emptyLabel='Не определено'
                    />
                  </div>
                  <div>
                    <div className='mb-2 text-xs font-medium text-gray-500'>Город (топ)</div>
                    <HBarList
                      rows={data.audience.by_city.map((r) => ({ key: audienceLabel(r.key), count: r.count }))}
                      emptyLabel='Не определено'
                    />
                  </div>
                </div>
              </CollapsibleChart>
            )}

            <CollapsibleChart icon='🌐' title='Источники — какой реально приносит деньги'>
              <p className='mb-4 text-xs text-gray-400'>
                Не просто число заявок, а что из них вышло — сколько дошло до консультации, до дела, и сколько денег
                это принесло. «Без источника» — письма без формы с сайта, так приходит почти всё: форма на сайте пока
                почти не используется.
              </p>
              <SourceList rows={data.by_source} />
            </CollapsibleChart>

            <CollapsibleChart icon='📋' title='Что происходит с забронированными консультациями'>
              <p className='mb-4 text-xs text-gray-400'>
                Статус ставит сотрудник кнопками под сообщением о брони в Telegram — не через админку.
              </p>
              <HBarList
                rows={data.by_status.map((s) => {
                  const meta = STATUS_META[s.key]
                  return { key: meta?.label ?? s.key, count: s.count, barColor: meta?.bar, variant: meta?.badge }
                })}
                emptyLabel='(без статуса)'
              />
            </CollapsibleChart>
          </div>
        </>
      )}
    </div>
  )
}

// GroupHeading marks the start of one of the 4 always-visible groups
// (Воронка / Деньги / Качество работы / Привлечение) — each is a logical
// continuation of the one above it, so the heading says why, not just what.
function GroupHeading({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div>
      <SectionHeader title={title} />
      {subtitle && <p className='-mt-3 text-xs text-gray-400'>{subtitle}</p>}
    </div>
  )
}

// FunnelRow is a cohort conversion, not "consultations this period / leads
// this period" — see the LeadStatsFunnel doc in openapi.yaml for why that
// distinction matters. Консультация → дело is normally the worst step by
// far (a handful of percent), so it's flagged red past a threshold the
// same way the cancellation-rate card already does elsewhere on this page.
// Stage width (row layout) / height (mobile column layout) grows with the
// stage's own share of the first stage — the funnel's whole point is showing
// the drop-off as a shape, not just as three same-size boxes with a number
// in each. A floor keeps the smallest stage from shrinking to unreadable.
const FUNNEL_SHARE_FLOOR = 18

function FunnelRow({
  leads,
  consultations,
  cases,
  daysToConsult,
  daysToCase,
}: {
  leads: number
  consultations: number
  cases: number
  daysToConsult: number
  daysToCase: number
}) {
  const leadToConsult = leads > 0 ? (consultations / leads) * 100 : 0
  const consultToCase = consultations > 0 ? (cases / consultations) * 100 : 0
  const caseStageIsBad = consultToCase < 10
  return (
    <div className='flex flex-col items-stretch overflow-hidden rounded-lg sm:flex-row sm:gap-1'>
      <FunnelStage label='Клиентов' value={leads} share={Math.max(FUNNEL_SHARE_FLOOR, 100)} />
      <FunnelArrow pct={leadToConsult} days={daysToConsult} />
      <FunnelStage label='Дошли до консультации' value={consultations} share={Math.max(FUNNEL_SHARE_FLOOR, leadToConsult)} />
      <FunnelArrow pct={consultToCase} days={daysToCase} bad={caseStageIsBad} />
      <FunnelStage
        label='Дошли до дела'
        value={cases}
        share={Math.max(FUNNEL_SHARE_FLOOR, leads > 0 ? (cases / leads) * 100 : 0)}
        accent={caseStageIsBad ? 'bad' : undefined}
      />
    </div>
  )
}

function FunnelStage({
  label,
  value,
  share,
  accent,
}: {
  label: string
  value: number
  share: number
  accent?: 'bad'
}) {
  return (
    <div
      className='flex basis-0 flex-col justify-center rounded-md p-4 text-white sm:min-w-[6rem]'
      style={{ flexGrow: share, backgroundColor: accent === 'bad' ? STATUS_CRITICAL : CHART_PRIMARY }}
    >
      <div className='text-xs text-white/80'>{label}</div>
      <div className='mt-1 text-2xl font-semibold tabular-nums'>{value.toLocaleString('ru-RU')}</div>
    </div>
  )
}

function FunnelArrow({ pct, days, bad }: { pct: number; days: number; bad?: boolean }) {
  return (
    <div
      className={cx(
        'flex shrink-0 flex-col items-center justify-center px-3 py-2 text-center whitespace-nowrap sm:py-0',
        bad ? '' : 'text-gray-400'
      )}
      style={bad ? { color: STATUS_CRITICAL } : undefined}
    >
      <span className='text-sm font-medium'>
        {pct.toFixed(1)}% <span className='inline-block rotate-90 sm:rotate-0'>→</span>
      </span>
      {days > 0 && <span className='text-[11px] text-gray-400'>в среднем {fmtDays(days)}</span>}
    </div>
  )
}

// FirstConsultOutcomeBar is the funnel's drop-off detail — of the cohort's
// "дошли до консультации" stage above, what happened to each client's FIRST
// consultation specifically (not "ever", which FunnelRow's own numbers
// already cover). Reuses STATUS_META's colors/labels so a segment here
// means the same thing it does everywhere else consultation status shows up.
const FIRST_CONSULT_OUTCOME_ORDER = ['completed', 'cancelled', 'no_show', 'scheduled'] as const

function FirstConsultOutcomeBar({
  outcome,
}: {
  outcome: { completed: number; cancelled: number; no_show: number; scheduled: number }
}) {
  const total = outcome.completed + outcome.cancelled + outcome.no_show + outcome.scheduled
  if (total === 0) return null

  return (
    <div className='space-y-2'>
      <div className='text-xs text-gray-400'>Дошли до консультации — что случилось на первой:</div>
      <div className='flex h-4 overflow-hidden rounded bg-gray-100'>
        {FIRST_CONSULT_OUTCOME_ORDER.filter((key) => outcome[key] > 0).map((key) => (
          <div
            key={key}
            style={{ width: `${(outcome[key] / total) * 100}%`, backgroundColor: STATUS_META[key].bar }}
            title={`${STATUS_META[key].label}: ${outcome[key]}`}
          />
        ))}
      </div>
      <div className='flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500'>
        {FIRST_CONSULT_OUTCOME_ORDER.map((key) => (
          <span key={key} className='inline-flex items-center gap-1.5'>
            <span className='inline-block h-2 w-2 rounded-full' style={{ backgroundColor: STATUS_META[key].bar }} />
            {STATUS_META[key].label}: {outcome[key]} ({Math.round((outcome[key] / total) * 100)}%)
          </span>
        ))}
      </div>
    </div>
  )
}

// Russian day-count pluralisation — "1 день", "2 дня", "5 дней" — the kind
// of thing that reads as broken/machine-made if left as a flat "n дней".
// Russian noun pluralisation — "1 день/дело/заявка", "2 дня/дела/заявки",
// "5 дней/дел/заявок" all follow the same one/few/many split, just with
// different words. One shared rule instead of copy-pasting the same
// mod10/mod100 branching under a new name for every noun on this page.
function pluralizeRu(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod100 >= 11 && mod100 <= 14) return many
  if (mod10 === 1) return one
  if (mod10 >= 2 && mod10 <= 4) return few
  return many
}

function fmtDays(n: number): string {
  if (n < 1) return 'меньше дня' // "0.1 дня" reads as broken next to "в среднем"
  const rounded = Math.round(n)
  return `${rounded} ${pluralizeRu(rounded, 'день', 'дня', 'дней')}`
}

function fmtCases(n: number): string {
  return `${n} ${pluralizeRu(n, 'дело', 'дела', 'дел')}`
}

function fmtClients(n: number): string {
  return `${n.toLocaleString('ru-RU')} ${pluralizeRu(n, 'клиент', 'клиента', 'клиентов')}`
}

function fmtLeads(n: number): string {
  return `${n} ${pluralizeRu(n, 'заявка', 'заявки', 'заявок')}`
}

// CollapsibleChart is a chart/list section that starts closed — the header
// (icon + title) doubles as its own toggle button, so a stack of these reads
// as a compact menu until you open the one you actually want. Each one is
// independent: opening one doesn't close the others, and it stays open
// until the same button is clicked again.
function CollapsibleChart({
  icon,
  title,
  subtitle,
  children,
}: {
  icon: string
  title: string
  subtitle?: string
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <Card className='overflow-hidden p-0'>
      <button
        type='button'
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className={cx('flex w-full items-center gap-3 p-4 text-left hover:bg-gray-50 active:bg-gray-100', open && 'bg-gray-50')}
      >
        <span className='text-xl' aria-hidden>
          {icon}
        </span>
        <span className='min-w-0 flex-1'>
          <span className='block truncate text-sm font-medium'>{title}</span>
          {subtitle && <span className='block truncate text-xs text-gray-400'>{subtitle}</span>}
        </span>
        <span className={cx('shrink-0 text-gray-400 transition-transform', open && 'rotate-180')} aria-hidden>
          ▾
        </span>
      </button>
      {open && <div className='border-t border-gray-100 p-4 pt-4'>{children}</div>}
    </Card>
  )
}

// Every bar-list on this page caps itself at the top 8 rows for readability
// (a 30-source list would be unreadable). Silently dropping the rest reads as
// a bug the moment there are actually 9+ rows, so every list that slices to 8
// says so.
const LIST_ROW_LIMIT = 8

function TruncationNote({ shown, total }: { shown: number; total: number }) {
  if (total <= shown) return null
  return <div className='pt-1 text-xs text-gray-400'>Показаны топ-{shown} из {total}</div>
}

function HBarList({
  rows,
  emptyLabel,
}: {
  rows: { key: string; count: number; barColor?: string; variant?: BadgeVariant }[]
  emptyLabel: string
}) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const total = rows.reduce((s, r) => s + r.count, 0)
  const max = Math.max(...rows.map((r) => r.count))
  return (
    <div className='space-y-2'>
      {rows.slice(0, LIST_ROW_LIMIT).map((r) => {
        const label = r.key === '' ? emptyLabel : r.key
        const pct = total ? ((r.count / total) * 100).toFixed(1) : '0.0'
        return (
          <div key={label} className='flex flex-col gap-1 text-sm sm:flex-row sm:items-center sm:gap-3'>
            <div className='min-w-0 sm:w-40 sm:shrink-0 sm:text-right'>
              {r.variant ? (
                <Badge variant={r.variant}>{label}</Badge>
              ) : (
                <span className='block truncate text-gray-600' title={label}>
                  {label}
                </span>
              )}
            </div>
            <div className='h-4 rounded bg-gray-100 sm:flex-1'>
              <div
                className='h-4 rounded'
                style={{ width: `${Math.max(2, (r.count / max) * 100)}%`, backgroundColor: r.barColor ?? CHART_PRIMARY }}
              />
            </div>
            <div className='tabular-nums text-gray-700 sm:w-24 sm:shrink-0'>
              {r.count} · {pct}%
            </div>
          </div>
        )
      })}
      <TruncationNote shown={LIST_ROW_LIMIT} total={rows.length} />
    </div>
  )
}

function bucketLabel(bucket: string, groupBy: string): string {
  if (groupBy === 'month') {
    const [y, m] = bucket.split('-')
    const names = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек']
    return `${names[parseInt(m, 10) - 1]}'${y.slice(2)}`
  }
  const [, m, d] = bucket.split('-')
  return `${d}.${m}`
}

const WEEKDAY_LABEL: Record<string, string> = {
  '1': 'Пн',
  '2': 'Вт',
  '3': 'Ср',
  '4': 'Чт',
  '5': 'Пт',
  '6': 'Сб',
  '7': 'Вс',
}
const WEEKDAY_ORDER = ['1', '2', '3', '4', '5', '6', '7']

// Fixed Mon–Sun order, not sorted by count like the other bar lists here —
// this is a calendar, not a ranking, so re-sorting it would make the shape
// unreadable (that's the whole point of asking "which day").
// ChartTooltip replaces the native `title` attribute on a bar/column —
// `title` only appears after a slow, motionless hover and is easy to miss
// entirely (that's what "hovering shows nothing" turned out to be).
// CSS `:hover` alone (group-hover/tip below) covers mouse/trackpad, but
// touch has no hover state at all — nothing ever showed on a phone. `open`/
// `onToggle` are owned by the chart above so tapping one bar closes
// whichever other one was open, instead of every tapped tooltip stacking up.
// A real <button> (not a div+onClick) so the tooltip is reachable and
// triggerable by keyboard too — Tab lands on it, Enter/Space fires onToggle
// for free, and group-focus/tip below reveals the tooltip on focus the same
// way group-hover/tip does on mouse hover.
function ChartTooltip({
  content,
  className,
  open,
  onToggle,
  children,
}: {
  content: React.ReactNode
  className?: string
  open: boolean
  onToggle: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type='button'
      className={cx(
        'group/tip relative rounded border-0 bg-transparent p-0 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-300',
        className
      )}
      onClick={onToggle}
      aria-pressed={open}
    >
      {children}
      <div
        className={cx(
          'pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 -translate-x-1/2 rounded-md bg-gray-900 px-2 py-1 text-xs whitespace-nowrap text-white opacity-0 shadow-lg transition-opacity group-hover/tip:opacity-100 group-focus/tip:opacity-100',
          open && 'opacity-100'
        )}
      >
        {content}
      </div>
    </button>
  )
}

function WeekdayChart({ rows }: { rows: { key: string; count: number }[] }) {
  const byDay = new Map(rows.map((r) => [r.key, r.count]))
  const values = WEEKDAY_ORDER.map((d) => byDay.get(d) ?? 0)
  const max = Math.max(1, ...values)
  const total = values.reduce((s, v) => s + v, 0)
  const [active, setActive] = useState<string | null>(null)
  if (total === 0) return <div className='text-sm text-gray-400'>Нет данных за период.</div>

  return (
    <div className='flex h-32 items-end gap-3'>
      {WEEKDAY_ORDER.map((d, i) => {
        const value = values[i]
        const pct = total ? Math.round((value / total) * 100) : 0
        return (
          <ChartTooltip
            key={d}
            className='flex min-w-0 flex-1 flex-col items-center gap-1'
            open={active === d}
            onToggle={() => setActive((a) => (a === d ? null : d))}
            content={
              <span>
                <span className='font-semibold'>{fmtLeads(value)}</span>
                <span className='text-gray-400'> ({pct}%)</span>
              </span>
            }
          >
            <div className='text-xs text-gray-500 tabular-nums'>{value}</div>
            <div className='flex h-20 w-full items-end'>
              <div
                className='w-full rounded-t'
                style={{ height: `${Math.max(value > 0 ? 4 : 0, (value / max) * 100)}%`, backgroundColor: CHART_PRIMARY }}
              />
            </div>
            <div className='text-xs text-gray-400'>{WEEKDAY_LABEL[d]}</div>
          </ChartTooltip>
        )
      })}
    </div>
  )
}

function TrendChart({ trend, groupBy }: { trend: { bucket: string; leads: number; consultations: number }[]; groupBy: string }) {
  return (
    <LineChart
      xLabels={trend.map((t) => bucketLabel(t.bucket, groupBy))}
      series={[
        { key: 'leads', label: 'заявки', color: CHART_PRIMARY, values: trend.map((t) => t.leads) },
        { key: 'consultations', label: 'консультации', color: CHART_SECONDARY, values: trend.map((t) => t.consultations) },
      ]}
    />
  )
}

// Deliberately its own chart, not a third series on TrendChart above — money
// and counts live on different scales, and a chart with two y-axes is the
// single most common way to make a chart lie (one series looks bigger than
// it is just because its axis is stretched differently).
function RevenueTrendChart({
  trend,
  groupBy,
}: {
  trend: { bucket: string; revenue_earned: number }[]
  groupBy: string
}) {
  return (
    <LineChart
      xLabels={trend.map((t) => bucketLabel(t.bucket, groupBy))}
      series={[{ key: 'revenue', label: 'выручка', color: CHART_PRIMARY, values: trend.map((t) => t.revenue_earned) }]}
      formatValue={fmtMoney}
      area
    />
  )
}

// Two series (total vs organic sessions), same scale — unlike Revenue vs
// counts, these two genuinely share a y-axis (both are "sessions"), so one
// chart with two bars is honest here, same pattern as the leads/consultations
// TrendChart above.
function TrafficTrendChart({
  trend,
  groupBy,
}: {
  trend: { bucket: string; site_sessions: number; organic_sessions: number }[]
  groupBy: string
}) {
  return (
    <LineChart
      xLabels={trend.map((t) => bucketLabel(t.bucket, groupBy))}
      series={[
        { key: 'sessions', label: 'все визиты', color: CHART_PRIMARY, values: trend.map((t) => t.site_sessions) },
        { key: 'organic', label: 'из поиска', color: CHART_SECONDARY, values: trend.map((t) => t.organic_sessions) },
      ]}
    />
  )
}

// SourceList — bar sized by money the source actually brought in (revenue
// earned + case payments), not by lead count. A source with 3 leads that
// all became paying clients outranks one with 300 that went nowhere.
function SourceList({
  rows,
}: {
  rows: { key: string; leads: number; consulted_ever: number; cased_ever: number; revenue_earned: number; case_paid: number }[]
}) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const valueOf = (r: (typeof rows)[number]) => r.revenue_earned + r.case_paid
  const max = Math.max(1, ...rows.map(valueOf))
  return (
    <div className='space-y-2'>
      {rows.slice(0, LIST_ROW_LIMIT).map((r) => {
        const label = r.key === '' ? '(без источника)' : r.key
        const value = valueOf(r)
        return (
          <div key={label} className='flex flex-col gap-1 text-sm sm:flex-row sm:items-center sm:gap-3'>
            <div className='truncate text-gray-600 sm:w-40 sm:shrink-0 sm:text-right' title={label}>
              {label}
            </div>
            <div className='h-4 rounded bg-gray-100 sm:flex-1'>
              <div
                className='h-4 rounded'
                style={{ width: `${Math.max(value > 0 ? 2 : 0, (value / max) * 100)}%`, backgroundColor: CHART_PRIMARY }}
              />
            </div>
            <div className='tabular-nums text-gray-700 sm:w-56 sm:shrink-0'>
              {fmtMoney(value)} · {fmtClients(r.leads)} · {fmtCases(r.cased_ever)}
            </div>
          </div>
        )
      })}
      <TruncationNote shown={LIST_ROW_LIMIT} total={rows.length} />
    </div>
  )
}

function CategoryList({
  rows,
  emptyLabel = '(без направления)',
}: {
  rows: { key: string; cases: number; contracted: number; paid: number }[]
  emptyLabel?: string
}) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...rows.map((r) => r.contracted))
  return (
    <div className='space-y-2'>
      {rows.slice(0, LIST_ROW_LIMIT).map((r) => {
        const label = r.key === '' ? emptyLabel : r.key
        const paidPct = r.contracted ? Math.round((r.paid / r.contracted) * 100) : 0
        return (
          <div key={label} className='flex flex-col gap-1 text-sm sm:flex-row sm:items-center sm:gap-3'>
            <div className='truncate text-gray-600 sm:w-56 sm:shrink-0 sm:text-right' title={label}>
              {label}
            </div>
            <div className='relative h-4 rounded bg-gray-100 sm:flex-1'>
              <div className='h-4 rounded' style={{ width: `${Math.max(2, (r.contracted / max) * 100)}%`, backgroundColor: CHART_PRIMARY, opacity: 0.35 }} />
              <div
                className='absolute top-0 left-0 h-4 rounded'
                style={{ width: `${Math.max(r.paid > 0 ? 2 : 0, (r.paid / max) * 100)}%`, backgroundColor: CHART_PRIMARY }}
              />
            </div>
            <div className='tabular-nums text-gray-700 sm:w-44 sm:shrink-0'>
              {fmtMoney(r.contracted)} · {fmtCases(r.cases)} · {paidPct}% оплачено
            </div>
          </div>
        )
      })}
      <TruncationNote shown={LIST_ROW_LIMIT} total={rows.length} />
    </div>
  )
}

function CreatorList({ rows }: { rows: { key: string; bookings: number; revenue_earned: number }[] }) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...rows.map((r) => r.revenue_earned))
  return (
    <div className='space-y-2'>
      {rows.slice(0, LIST_ROW_LIMIT).map((r) => {
        const label = r.key === '' ? '(не указано)' : r.key
        return (
          <div key={label} className='flex flex-col gap-1 text-sm sm:flex-row sm:items-center sm:gap-3'>
            <div className='truncate text-gray-600 sm:w-32 sm:shrink-0 sm:text-right' title={label}>
              {label}
            </div>
            <div className='h-4 rounded bg-gray-100 sm:flex-1'>
              <div
                className='h-4 rounded'
                style={{ width: `${Math.max(2, (r.revenue_earned / max) * 100)}%`, backgroundColor: CHART_PRIMARY }}
              />
            </div>
            <div className='tabular-nums text-gray-700 sm:w-36 sm:shrink-0'>
              {fmtMoney(r.revenue_earned)} · {r.bookings} бр.
            </div>
          </div>
        )
      })}
      <TruncationNote shown={LIST_ROW_LIMIT} total={rows.length} />
    </div>
  )
}
