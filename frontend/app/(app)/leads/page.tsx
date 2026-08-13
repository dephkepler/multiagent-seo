'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
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
  by_page: { key: string; count: number }[]
  by_creator: { key: string; bookings: number; revenue_earned: number }[]
  by_status: { key: string; count: number }[]
  by_category: { key: string; cases: number; contracted: number; paid: number }[]
}

const STATUS_LABEL: Record<string, string> = {
  scheduled: 'Запланирована',
  completed: 'Провёл',
  cancelled: 'Отменил',
  no_show: 'Не пришёл',
}
const STATUS_COLOR: Record<string, string> = {
  scheduled: 'bg-gray-400',
  completed: 'bg-emerald-500',
  cancelled: 'bg-red-500',
  no_show: 'bg-amber-500',
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
    setPickerOpen(false)
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
      if (e.key === 'Escape') setPickerOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [pickerOpen])

  return (
    <div className='max-w-6xl space-y-6'>
      <div>
        <h1 className='text-lg font-semibold'>Заявки и консультации</h1>
        <p className='mt-1 text-sm text-gray-500'>
          Считается напрямую из базы (<code className='rounded bg-gray-100 px-1'>leads</code>,{' '}
          <code className='rounded bg-gray-100 px-1'>consultations</code>) — включает историю до запуска бота и
          растёт с каждой новой заявкой.
        </p>
      </div>

      <div ref={pickerRef} className='relative flex items-center gap-3'>
        <button
          type='button'
          onClick={() => setPickerOpen((v) => !v)}
          className={cx(
            'flex items-center gap-2 rounded-lg border bg-white px-3 py-2 text-sm shadow-sm',
            pickerOpen ? 'border-emerald-400 ring-2 ring-emerald-100' : 'border-gray-200 hover:border-gray-300'
          )}
        >
          <span aria-hidden>📅</span>
          <span className='font-medium'>{activePreset || `${fmtDate(from)} – ${fmtDate(to)}`}</span>
          {activePreset && <span className='text-gray-400'>{fmtDate(from)} – {fmtDate(to)}</span>}
          <span className={cx('text-gray-400 transition-transform', pickerOpen && 'rotate-180')}>▾</span>
        </button>
        <span className='text-xs text-gray-400'>группировка: {groupBy === 'day' ? 'по дням' : 'по месяцам'}</span>

        {pickerOpen && (
          <div className='absolute top-full left-0 z-20 mt-2 flex w-[420px] gap-4 rounded-lg border border-gray-200 bg-white p-4 shadow-lg'>
            <div className='flex w-36 shrink-0 flex-col gap-1'>
              {PRESETS.map((p) => (
                <button
                  key={p.label}
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
            <div className='w-px bg-gray-100' />
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
                onClick={() => setPickerOpen(false)}
                className='mt-auto self-end rounded-md bg-emerald-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-600'
              >
                Готово
              </button>
            </div>
          </div>
        )}
      </div>

      {isError && (
        <Card className='border-red-200 bg-red-50 text-sm text-red-700'>
          Не удалось загрузить статистику{error instanceof Error ? `: ${error.message}` : ''}.
        </Card>
      )}

      {isLoading && <Card className='text-sm text-gray-500'>Загрузка…</Card>}

      {data && (
        <>
          <div className='grid grid-cols-3 gap-4'>
            <KpiTile label='Заявок' value={data.totals.leads.toLocaleString('ru-RU')} sub={`${fmtDate(data.range.from)} – ${fmtDate(data.range.to)}`} />
            <KpiTile label='Уникальных клиентов' value={data.totals.clients.toLocaleString('ru-RU')} />
            <KpiTile label='Консультаций забронировано' value={data.totals.consultations.toLocaleString('ru-RU')} />
          </div>

          <div>
            <div className='mb-2 text-xs font-medium tracking-wide text-gray-400 uppercase'>
              Консультации — цена самой консультации (обычно 500–800 ₴), не связанные с суммой дела
            </div>
            <div className='grid grid-cols-3 gap-4'>
              <KpiTile label='Забронировано (весь потенциал)' value={fmtMoney(data.totals.revenue_booked)} />
              <KpiTile
                label='Заработано (провёл)'
                value={fmtMoney(data.totals.revenue_earned)}
                sub={
                  data.totals.avg_ticket > 0
                    ? `средний чек ${fmtMoney(data.totals.avg_ticket)} — цена состоявшейся консультации, не дела`
                    : undefined
                }
                accent='good'
              />
              <KpiTile label='Потеряно (отменил / не пришёл)' value={fmtMoney(data.totals.revenue_lost)} accent='bad' />
            </div>
          </div>

          <div>
            <div className='mb-2 text-xs font-medium tracking-wide text-gray-400 uppercase'>
              Дела (клопотання/позов/супровід) — вот тут реальные деньги бизнеса
            </div>
            <div className='grid grid-cols-4 gap-4'>
              <KpiTile
                label='Дел'
                value={(data.totals.cases_in_progress + data.totals.cases_completed).toLocaleString('ru-RU')}
                sub={`${data.totals.cases_in_progress} в работе, ${data.totals.cases_completed} выполнено`}
              />
              <KpiTile label='Законтрактовано' value={fmtMoney(data.totals.case_fee_contracted)} />
              <KpiTile
                label='Получено оплат'
                value={fmtMoney(data.totals.case_paid)}
                sub='Растёт по мере оплаты частями — не "сумма договора", а реально поступившее'
                accent='good'
              />
              <KpiTile label='Долг клиентов' value={fmtMoney(data.totals.case_owed)} accent={data.totals.case_owed > 0 ? 'bad' : undefined} />
            </div>
          </div>

          {data.totals.site_sessions > 0 && (
            <div>
              <div className='mb-2 text-xs font-medium tracking-wide text-gray-400 uppercase'>
                Трафик сайта (GA4) — визиты, не заявки
              </div>
              <div className='grid grid-cols-3 gap-4'>
                <KpiTile label='Визитов на сайт' value={data.totals.site_sessions.toLocaleString('ru-RU')} />
                <KpiTile
                  label='Из поиска (Сео)'
                  value={data.totals.organic_sessions.toLocaleString('ru-RU')}
                  sub={`${Math.round((data.totals.organic_sessions / data.totals.site_sessions) * 100)}% от всего трафика`}
                />
                <KpiTile
                  label='Заявка на визит'
                  value={`${((data.totals.leads / data.totals.site_sessions) * 100).toFixed(1)}%`}
                  sub={`${data.totals.leads} заявок из ${data.totals.site_sessions} визитов`}
                />
              </div>
            </div>
          )}

          {(() => {
            const cancelled = data.by_status.find((s) => s.key === 'cancelled')?.count ?? 0
            const pct = data.totals.consultations ? (cancelled / data.totals.consultations) * 100 : 0
            if (!data.totals.consultations) return null
            return (
              <Card className={cx('p-4', pct >= 30 && 'border-red-200 bg-red-50')}>
                <div className='text-xs text-gray-500'>Отменено из забронированных консультаций</div>
                <div className={cx('mt-1 text-2xl font-semibold tabular-nums', pct >= 30 && 'text-red-700')}>{pct.toFixed(0)}%</div>
                <div className='mt-1 text-xs text-gray-400'>{cancelled} из {data.totals.consultations}</div>
              </Card>
            )
          })()}

          {/* Cifры выше открыты всегда; графики свёрнуты по умолчанию — каждый
              открывается своей кнопкой независимо от остальных и остаётся
              открытым, пока не нажать ту же кнопку ещё раз. */}
          <div className='space-y-2'>
            <CollapsibleChart icon='🎯' title='Какое направление приносит больше денег'>
              <p className='mb-4 text-xs text-gray-400'>
                По сумме договора (законтрактовано) и отдельно — сколько из этого реально оплачено. Категорию
                сотрудник выбирает при заведении дела через <code className='rounded bg-gray-100 px-1'>/case</code> в
                боте.
              </p>
              <CategoryList rows={data.by_category} />
            </CollapsibleChart>

            <CollapsibleChart icon='📈' title='Обращения по периоду' subtitle={groupBy === 'day' ? 'по дням' : 'по месяцам'}>
              <p className='mb-4 text-xs text-gray-400'>Заявки vs забронированные консультации, {groupBy === 'day' ? 'по дням' : 'по месяцам'}</p>
              <TrendChart trend={data.trend} groupBy={data.range.group_by} />
            </CollapsibleChart>

            <CollapsibleChart icon='💰' title='Выручка по периоду'>
              <p className='mb-4 text-xs text-gray-400'>
                Только проведённые консультации — своя шкала, это деньги, не штуки, специально отдельный график.
              </p>
              <RevenueTrendChart trend={data.trend} groupBy={data.range.group_by} />
            </CollapsibleChart>

            {data.totals.site_sessions > 0 && (
              <CollapsibleChart icon='🌐' title='Трафик сайта по периоду'>
                <p className='mb-4 text-xs text-gray-400'>
                  Все визиты и отдельно — сколько из поиска (Сео). Из GA4, своя шкала — цифры на порядок больше заявок.
                </p>
                <TrafficTrendChart trend={data.trend} groupBy={data.range.group_by} />
              </CollapsibleChart>
            )}

            <CollapsibleChart icon='🌐' title='Источники (page)'>
              <p className='mb-4 text-xs text-gray-400'>
                Пусто — письма без формы с сайта (в т.ч. вся история до бота). Заполнено — реальная страница/форма.
              </p>
              <HBarList rows={data.by_page} emptyLabel='(без источника)' />
            </CollapsibleChart>

            <CollapsibleChart icon='👤' title='Кто приносит выручку'>
              <p className='mb-4 text-xs text-gray-400'>
                Сортировка по заработанному, не по числу записей — сотрудник с меньшим числом броней, но выше
                конверсией, может быть выше. <code className='rounded bg-gray-100 px-1'>import:Имя</code> — из
                истории (55k), число — Telegram ID сотрудника.
              </p>
              <CreatorList rows={data.by_creator} />
            </CollapsibleChart>

            <CollapsibleChart icon='📋' title='Что происходит с забронированными консультациями'>
              <p className='mb-4 text-xs text-gray-400'>
                Статус ставит сотрудник кнопками под сообщением о брони в Telegram — не через админку.
              </p>
              <HBarList
                rows={data.by_status.map((s) => ({ ...s, key: STATUS_LABEL[s.key] ?? s.key }))}
                emptyLabel='(без статуса)'
                colorFor={(label) => {
                  const entry = Object.entries(STATUS_LABEL).find(([, v]) => v === label)
                  return entry ? STATUS_COLOR[entry[0]] : 'bg-gray-400'
                }}
              />
            </CollapsibleChart>
          </div>
        </>
      )}
    </div>
  )
}

function KpiTile({
  label,
  value,
  sub,
  accent,
}: {
  label: string
  value: string
  sub?: string
  accent?: 'good' | 'bad'
}) {
  return (
    <Card className='p-4'>
      <div className='text-xs text-gray-500'>{label}</div>
      <div
        className={cx(
          'mt-1 text-2xl font-semibold tabular-nums',
          accent === 'good' && 'text-emerald-700',
          accent === 'bad' && 'text-red-700'
        )}
      >
        {value}
      </div>
      {sub && <div className='mt-1 text-xs text-gray-400'>{sub}</div>}
    </Card>
  )
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
        className='flex w-full items-center gap-3 p-4 text-left hover:bg-gray-50'
      >
        <span className='text-xl' aria-hidden>
          {icon}
        </span>
        <span className='flex-1'>
          <span className='block text-sm font-medium'>{title}</span>
          {subtitle && <span className='block text-xs text-gray-400'>{subtitle}</span>}
        </span>
        <span className={cx('text-gray-400 transition-transform', open && 'rotate-180')} aria-hidden>
          ▾
        </span>
      </button>
      {open && <div className='border-t border-gray-100 p-4 pt-4'>{children}</div>}
    </Card>
  )
}

function HBarList({
  rows,
  emptyLabel,
  colorFor,
}: {
  rows: { key: string; count: number }[]
  emptyLabel: string
  colorFor?: (label: string) => string
}) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const total = rows.reduce((s, r) => s + r.count, 0)
  const max = Math.max(...rows.map((r) => r.count))
  return (
    <div className='space-y-2'>
      {rows.slice(0, 8).map((r) => {
        const label = r.key === '' ? emptyLabel : r.key
        const pct = total ? ((r.count / total) * 100).toFixed(1) : '0.0'
        return (
          <div key={label} className='flex items-center gap-3 text-sm'>
            <div className='w-40 shrink-0 truncate text-right text-gray-600' title={label}>
              {label}
            </div>
            <div className='h-4 flex-1 rounded bg-gray-100'>
              <div
                className={cx('h-4 rounded', colorFor ? colorFor(label) : 'bg-emerald-500')}
                style={{ width: `${Math.max(2, (r.count / max) * 100)}%` }}
              />
            </div>
            <div className='w-24 shrink-0 tabular-nums text-gray-700'>
              {r.count} · {pct}%
            </div>
          </div>
        )
      })}
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

function TrendChart({ trend, groupBy }: { trend: { bucket: string; leads: number; consultations: number }[]; groupBy: string }) {
  if (!trend.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...trend.map((t) => Math.max(t.leads, t.consultations)))
  const step = Math.max(1, Math.ceil(trend.length / 14))

  return (
    <div>
      <div className='mb-2 flex items-center gap-4 text-xs text-gray-500'>
        <span className='flex items-center gap-1'>
          <span className='h-2 w-2 rounded-full bg-emerald-500' /> заявки
        </span>
        <span className='flex items-center gap-1'>
          <span className='h-2 w-2 rounded-full bg-sky-500' /> консультации
        </span>
      </div>
      {/* h-40 gives this row a definite 160px height so the flex-1 bar-wells below can
          resolve their own height:100% against something real — items-end here would
          size each column to its content instead of stretching it, collapsing the bars. */}
      <div className='flex h-40 gap-1 border-b border-gray-200'>
        {trend.map((t) => (
          <div
            key={t.bucket}
            className='flex min-w-0 flex-1 items-end justify-center gap-0.5'
            title={`${bucketLabel(t.bucket, groupBy)}: ${t.leads} заявок, ${t.consultations} консультаций`}
          >
            <div className='w-1/2 max-w-[10px] rounded-t bg-emerald-500' style={{ height: `${Math.max(2, (t.leads / max) * 100)}%` }} />
            <div className='w-1/2 max-w-[10px] rounded-t bg-sky-500' style={{ height: `${Math.max(2, (t.consultations / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className='mt-1 flex gap-1'>
        {trend.map((t, i) => (
          <div key={t.bucket} className='min-w-0 flex-1 truncate text-center text-[10px] text-gray-400'>
            {i % step === 0 ? bucketLabel(t.bucket, groupBy) : ''}
          </div>
        ))}
      </div>
    </div>
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
  if (!trend.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...trend.map((t) => t.revenue_earned))
  const step = Math.max(1, Math.ceil(trend.length / 14))

  return (
    <div>
      <div className='flex h-32 gap-1 border-b border-gray-200'>
        {trend.map((t) => (
          <div
            key={t.bucket}
            className='flex min-w-0 flex-1 items-end justify-center'
            title={`${bucketLabel(t.bucket, groupBy)}: ${fmtMoney(t.revenue_earned)}`}
          >
            <div className='w-3/5 max-w-[18px] rounded-t bg-emerald-500' style={{ height: `${Math.max(2, (t.revenue_earned / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className='mt-1 flex gap-1'>
        {trend.map((t, i) => (
          <div key={t.bucket} className='min-w-0 flex-1 truncate text-center text-[10px] text-gray-400'>
            {i % step === 0 ? bucketLabel(t.bucket, groupBy) : ''}
          </div>
        ))}
      </div>
    </div>
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
  if (!trend.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...trend.map((t) => t.site_sessions))
  const step = Math.max(1, Math.ceil(trend.length / 14))

  return (
    <div>
      <div className='mb-2 flex items-center gap-4 text-xs text-gray-500'>
        <span className='flex items-center gap-1'>
          <span className='h-2 w-2 rounded-full bg-sky-500' /> все визиты
        </span>
        <span className='flex items-center gap-1'>
          <span className='h-2 w-2 rounded-full bg-emerald-500' /> из поиска
        </span>
      </div>
      <div className='flex h-40 gap-1 border-b border-gray-200'>
        {trend.map((t) => (
          <div
            key={t.bucket}
            className='flex min-w-0 flex-1 items-end justify-center gap-0.5'
            title={`${bucketLabel(t.bucket, groupBy)}: ${t.site_sessions} визитов, ${t.organic_sessions} из поиска`}
          >
            <div className='w-1/2 max-w-[10px] rounded-t bg-sky-500' style={{ height: `${Math.max(2, (t.site_sessions / max) * 100)}%` }} />
            <div className='w-1/2 max-w-[10px] rounded-t bg-emerald-500' style={{ height: `${Math.max(2, (t.organic_sessions / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className='mt-1 flex gap-1'>
        {trend.map((t, i) => (
          <div key={t.bucket} className='min-w-0 flex-1 truncate text-center text-[10px] text-gray-400'>
            {i % step === 0 ? bucketLabel(t.bucket, groupBy) : ''}
          </div>
        ))}
      </div>
    </div>
  )
}

function CategoryList({ rows }: { rows: { key: string; cases: number; contracted: number; paid: number }[] }) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...rows.map((r) => r.contracted))
  return (
    <div className='space-y-2'>
      {rows.slice(0, 8).map((r) => {
        const label = r.key === '' ? '(без напрямку)' : r.key
        const paidPct = r.contracted ? Math.round((r.paid / r.contracted) * 100) : 0
        return (
          <div key={label} className='flex items-center gap-3 text-sm'>
            <div className='w-56 shrink-0 truncate text-right text-gray-600' title={label}>
              {label}
            </div>
            <div className='relative h-4 flex-1 rounded bg-gray-100'>
              <div className='h-4 rounded bg-emerald-200' style={{ width: `${Math.max(2, (r.contracted / max) * 100)}%` }} />
              <div
                className='absolute top-0 left-0 h-4 rounded bg-emerald-500'
                style={{ width: `${Math.max(r.paid > 0 ? 2 : 0, (r.paid / max) * 100)}%` }}
              />
            </div>
            <div className='w-44 shrink-0 tabular-nums text-gray-700'>
              {fmtMoney(r.contracted)} · {r.cases} дел · {paidPct}% оплачено
            </div>
          </div>
        )
      })}
    </div>
  )
}

function CreatorList({ rows }: { rows: { key: string; bookings: number; revenue_earned: number }[] }) {
  if (!rows.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...rows.map((r) => r.revenue_earned))
  return (
    <div className='space-y-2'>
      {rows.slice(0, 8).map((r) => {
        const label = r.key === '' ? '(не указано)' : r.key
        return (
          <div key={label} className='flex items-center gap-3 text-sm'>
            <div className='w-32 shrink-0 truncate text-right text-gray-600' title={label}>
              {label}
            </div>
            <div className='h-4 flex-1 rounded bg-gray-100'>
              <div
                className='h-4 rounded bg-emerald-500'
                style={{ width: `${Math.max(2, (r.revenue_earned / max) * 100)}%` }}
              />
            </div>
            <div className='w-36 shrink-0 tabular-nums text-gray-700'>
              {fmtMoney(r.revenue_earned)} · {r.bookings} бр.
            </div>
          </div>
        )
      })}
    </div>
  )
}
