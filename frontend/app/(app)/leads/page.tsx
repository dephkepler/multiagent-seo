'use client'

import { useMemo, useState } from 'react'
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
  }
  trend: { bucket: string; leads: number; consultations: number; revenue_earned: number }[]
  by_page: { key: string; count: number }[]
  by_creator: { key: string; bookings: number; revenue_earned: number }[]
  by_status: { key: string; count: number }[]
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
  }

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

      <Card className='flex flex-wrap items-center gap-2'>
        <span className='text-xs text-gray-500'>Период:</span>
        {PRESETS.map((p) => (
          <button
            key={p.label}
            onClick={() => applyPreset(p)}
            className={cx(
              'rounded-full border px-3 py-1 text-xs',
              activePreset === p.label
                ? 'border-emerald-500 bg-emerald-500 text-white font-medium'
                : 'border-gray-200 text-gray-600 hover:border-emerald-300'
            )}
          >
            {p.label}
          </button>
        ))}
        <span className='mx-1 h-5 w-px bg-gray-200' />
        <span className='flex items-center gap-1 text-xs text-gray-600'>
          с
          <input
            type='date'
            value={from}
            onChange={(e) => {
              setFrom(e.target.value)
              setActivePreset('')
            }}
            className='rounded border border-gray-200 px-2 py-1 text-xs'
          />
          по
          <input
            type='date'
            value={to}
            onChange={(e) => {
              setTo(e.target.value)
              setActivePreset('')
            }}
            className='rounded border border-gray-200 px-2 py-1 text-xs'
          />
        </span>
        <span className='ml-auto text-xs text-gray-400'>
          группировка: {groupBy === 'day' ? 'по дням' : 'по месяцам'}
        </span>
      </Card>

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

          <Card>
            <h2 className='mb-1 text-sm font-medium'>Обращения по периоду</h2>
            <p className='mb-4 text-xs text-gray-400'>Заявки vs забронированные консультации, {groupBy === 'day' ? 'по дням' : 'по месяцам'}</p>
            <TrendChart trend={data.trend} groupBy={data.range.group_by} />
          </Card>

          <Card>
            <h2 className='mb-1 text-sm font-medium'>Выручка по периоду</h2>
            <p className='mb-4 text-xs text-gray-400'>
              Только проведённые консультации — своя шкала, это деньги, не штуки, специально отдельный график.
            </p>
            <RevenueTrendChart trend={data.trend} groupBy={data.range.group_by} />
          </Card>

          <div className='grid gap-4 md:grid-cols-2'>
            <Card>
              <h2 className='mb-1 text-sm font-medium'>Источники (page)</h2>
              <p className='mb-4 text-xs text-gray-400'>
                Пусто — письма без формы с сайта (в т.ч. вся история до бота). Заполнено — реальная страница/форма.
              </p>
              <HBarList rows={data.by_page} emptyLabel='(без источника)' />
            </Card>
            <Card>
              <h2 className='mb-1 text-sm font-medium'>Кто приносит выручку</h2>
              <p className='mb-4 text-xs text-gray-400'>
                Сортировка по заработанному, не по числу записей — сотрудник с меньшим числом броней, но выше
                конверсией, может быть выше. <code className='rounded bg-gray-100 px-1'>import:Имя</code> — из
                истории (55k), число — Telegram ID сотрудника.
              </p>
              <CreatorList rows={data.by_creator} />
            </Card>
          </div>

          <Card>
            <h2 className='mb-1 text-sm font-medium'>Что происходит с забронированными консультациями</h2>
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
          </Card>
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
