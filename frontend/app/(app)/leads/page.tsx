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
    revenue: number
    avg_ticket: number
  }
  trend: { bucket: string; leads: number; consultations: number }[]
  by_page: { key: string; count: number }[]
  by_creator: { key: string; count: number }[]
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
          <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
            <KpiTile label='Заявок' value={data.totals.leads.toLocaleString('ru-RU')} sub={`${fmtDate(data.range.from)} – ${fmtDate(data.range.to)}`} />
            <KpiTile label='Уникальных клиентов' value={data.totals.clients.toLocaleString('ru-RU')} />
            <KpiTile label='Консультаций забронировано' value={data.totals.consultations.toLocaleString('ru-RU')} />
            <KpiTile
              label='Выручка'
              value={fmtMoney(data.totals.revenue)}
              sub={data.totals.avg_ticket > 0 ? `средний чек ${fmtMoney(data.totals.avg_ticket)}` : undefined}
            />
          </div>

          <Card>
            <h2 className='mb-1 text-sm font-medium'>Обращения по периоду</h2>
            <p className='mb-4 text-xs text-gray-400'>Заявки vs забронированные консультации, {groupBy === 'day' ? 'по дням' : 'по месяцам'}</p>
            <TrendChart trend={data.trend} groupBy={data.range.group_by} />
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
              <h2 className='mb-1 text-sm font-medium'>Кто забронировал</h2>
              <p className='mb-4 text-xs text-gray-400'>
                <code className='rounded bg-gray-100 px-1'>import:Имя</code> — из истории (55k), число — Telegram ID
                сотрудника, бронировавшего вживую.
              </p>
              <HBarList rows={data.by_creator} emptyLabel='(не указано)' />
            </Card>
          </div>
        </>
      )}
    </div>
  )
}

function KpiTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <Card className='p-4'>
      <div className='text-xs text-gray-500'>{label}</div>
      <div className='mt-1 text-2xl font-semibold tabular-nums'>{value}</div>
      {sub && <div className='mt-1 text-xs text-gray-400'>{sub}</div>}
    </Card>
  )
}

function HBarList({ rows, emptyLabel }: { rows: { key: string; count: number }[]; emptyLabel: string }) {
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
                className='h-4 rounded bg-emerald-500'
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

function TrendChart({ trend, groupBy }: { trend: { bucket: string; leads: number; consultations: number }[]; groupBy: string }) {
  if (!trend.length) return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  const max = Math.max(1, ...trend.map((t) => Math.max(t.leads, t.consultations)))
  const step = Math.max(1, Math.ceil(trend.length / 14))

  function label(bucket: string): string {
    if (groupBy === 'month') {
      const [y, m] = bucket.split('-')
      const names = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек']
      return `${names[parseInt(m, 10) - 1]}'${y.slice(2)}`
    }
    const [, m, d] = bucket.split('-')
    return `${d}.${m}`
  }

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
            title={`${label(t.bucket)}: ${t.leads} заявок, ${t.consultations} консультаций`}
          >
            <div className='w-1/2 max-w-[10px] rounded-t bg-emerald-500' style={{ height: `${Math.max(2, (t.leads / max) * 100)}%` }} />
            <div className='w-1/2 max-w-[10px] rounded-t bg-sky-500' style={{ height: `${Math.max(2, (t.consultations / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className='mt-1 flex gap-1'>
        {trend.map((t, i) => (
          <div key={t.bucket} className='min-w-0 flex-1 truncate text-center text-[10px] text-gray-400'>
            {i % step === 0 ? label(t.bucket) : ''}
          </div>
        ))}
      </div>
    </div>
  )
}
