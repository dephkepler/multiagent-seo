'use client'

import { useMemo, useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cx } from '@/lib/cx'

type Segment = 'lead' | 'booked' | 'consulted' | 'client' | 'repeat' | 'lost'

interface ClientSegment {
  client_id: string
  name: string
  phone: string
  segment: Segment
  overridden: boolean
  tags: string[]
  last_activity: string
  case_count: number
  case_fee: number
  case_paid: number
  ltv: number
}

const SEGMENT_LABEL: Record<Segment, string> = {
  lead: 'Заявка',
  booked: 'Забронировал',
  consulted: 'Проконсультирован',
  client: 'Клиент',
  repeat: 'Повторный',
  lost: 'Потерян',
}
// Порядок — как в воронке (см. backend clientsegments.Derive), не алфавитный.
const SEGMENT_ORDER: Segment[] = ['lead', 'booked', 'consulted', 'client', 'repeat', 'lost']
const SEGMENT_COLOR: Record<Segment, string> = {
  lead: 'bg-gray-100 text-gray-700',
  booked: 'bg-sky-100 text-sky-800',
  consulted: 'bg-amber-100 text-amber-800', // самый денежный сегмент — есть кого дожимать
  client: 'bg-emerald-100 text-emerald-800',
  repeat: 'bg-violet-100 text-violet-800',
  lost: 'bg-rose-100 text-rose-800',
}

const TAG_LABEL: Record<string, string> = {
  debtor: 'Должник',
  no_show_risk: 'Риск неявки',
  high_value: 'Ценный клиент',
  dormant: 'Без контакта 90+ дней',
}
const TAG_COLOR: Record<string, string> = {
  debtor: 'border border-rose-200 bg-rose-50 text-rose-700',
  no_show_risk: 'border border-orange-200 bg-orange-50 text-orange-700',
  high_value: 'border border-emerald-200 bg-emerald-50 text-emerald-700',
  dormant: 'border border-gray-200 bg-gray-50 text-gray-500',
}

const PAGE_SIZE = 25

function fmtMoney(n: number): string {
  return Math.round(n).toLocaleString('ru-RU') + ' ₴'
}
function fmtDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString('ru-RU')
}

type SortKey = 'activity' | 'ltv'

export default function ClientsPage() {
  const qc = useQueryClient()
  const [query, setQuery] = useState('')
  const [segmentFilter, setSegmentFilter] = useState<Segment | 'all'>('all')
  const [page, setPage] = useState(1)
  const [sortKey, setSortKey] = useState<SortKey>('activity')

  const clients = useQuery({
    queryKey: ['client-segments'],
    queryFn: () => api<ClientSegment[]>('/clients'),
  })

  const setOverride = useMutation({
    mutationFn: ({ id, segment }: { id: string; segment: Segment | null }) =>
      api(`/clients/${id}/segment`, { method: 'PATCH', body: JSON.stringify({ segment_override: segment }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  const counts = useMemo(() => {
    const c: Partial<Record<Segment, number>> = {}
    for (const cl of clients.data || []) c[cl.segment] = (c[cl.segment] || 0) + 1
    return c
  }, [clients.data])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (clients.data || [])
      .filter((c) => segmentFilter === 'all' || c.segment === segmentFilter)
      .filter((c) => !q || c.name.toLowerCase().includes(q) || c.phone.includes(q))
      .sort((a, b) =>
        sortKey === 'ltv' ? b.ltv - a.ltv : new Date(b.last_activity).getTime() - new Date(a.last_activity).getTime()
      )
  }, [clients.data, query, segmentFilter, sortKey])

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageSafe = Math.min(page, pageCount)
  const pageItems = filtered.slice((pageSafe - 1) * PAGE_SIZE, pageSafe * PAGE_SIZE)

  function resetToFirstPage<T>(setter: (v: T) => void) {
    return (v: T) => {
      setter(v)
      setPage(1)
    }
  }
  const onQueryChange = resetToFirstPage(setQuery)
  const onSegmentFilterChange = resetToFirstPage(setSegmentFilter)
  const onSortKeyChange = resetToFirstPage(setSortKey)

  return (
    <div className='space-y-6'>
      <Card>
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-lg font-semibold'>Клиенты</h1>
          <span className='text-xs text-gray-400'>
            Снимок по всей истории, без периода — не путать с фильтром дат на «Заявках»
          </span>
        </div>

        <div className='mb-4 flex flex-wrap gap-2'>
          <SegmentPill active={segmentFilter === 'all'} onClick={() => onSegmentFilterChange('all')}>
            Все ({clients.data?.length ?? 0})
          </SegmentPill>
          {SEGMENT_ORDER.map((s) => (
            <SegmentPill
              key={s}
              active={segmentFilter === s}
              onClick={() => onSegmentFilterChange(s)}
              colorClass={SEGMENT_COLOR[s]}
            >
              {SEGMENT_LABEL[s]} ({counts[s] || 0})
            </SegmentPill>
          ))}
        </div>

        <div className='mb-4 flex items-center justify-between gap-4'>
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder='Поиск по имени или телефону…'
            className='max-w-sm'
          />
          <span className='shrink-0 text-xs text-gray-400'>
            Сегмент обычно считается сам — выберите вручную в таблице, чтобы закрепить свой (например клиент ушёл к
            другому адвокату)
          </span>
        </div>

        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead className='text-left text-xs uppercase text-gray-500'>
              <tr>
                <th className='py-2 pr-4'>Имя</th>
                <th className='py-2 pr-4'>Телефон</th>
                <th className='py-2 pr-4'>Сегмент</th>
                <th className='py-2 pr-4'>Теги</th>
                <th className='py-2 pr-4'>Дел</th>
                <th className='py-2 pr-4'>Долг</th>
                <SortableHeader label='LTV' active={sortKey === 'ltv'} onClick={() => onSortKeyChange('ltv')} />
                <SortableHeader
                  label='Последняя активность'
                  active={sortKey === 'activity'}
                  onClick={() => onSortKeyChange('activity')}
                  last
                />
              </tr>
            </thead>
            <tbody>
              {pageItems.length === 0 && (
                <tr>
                  <td colSpan={8} className='py-6 text-center text-gray-400'>
                    {clients.isLoading ? 'Загрузка…' : clients.isError ? 'Не удалось загрузить' : 'Никого не найдено'}
                  </td>
                </tr>
              )}
              {pageItems.map((c) => {
                const debt = c.case_fee - c.case_paid
                return (
                  <tr key={c.client_id} className='border-t border-gray-100'>
                    <td className='py-2 pr-4 font-medium'>
                      <Link href={`/clients/${c.client_id}`} className='text-emerald-700 hover:underline'>
                        {c.name || '—'}
                      </Link>
                    </td>
                    <td className='py-2 pr-4 text-gray-500'>{c.phone || '—'}</td>
                    <td className='py-2 pr-4'>
                      <div className='flex items-center gap-1.5'>
                        <select
                          value={c.segment}
                          disabled={setOverride.isPending}
                          onChange={(e) => setOverride.mutate({ id: c.client_id, segment: e.target.value as Segment })}
                          className={cx(
                            'cursor-pointer rounded px-1.5 py-0.5 text-xs font-medium outline-none disabled:cursor-wait',
                            SEGMENT_COLOR[c.segment]
                          )}
                        >
                          {SEGMENT_ORDER.map((s) => (
                            <option key={s} value={s}>
                              {SEGMENT_LABEL[s]}
                            </option>
                          ))}
                        </select>
                      </div>
                    </td>
                    <td className='py-2 pr-4'>
                      <div className='flex flex-wrap gap-1'>
                        {c.tags.map((t) => (
                          <span key={t} className={cx('rounded px-1.5 py-0.5 text-[11px] font-medium', TAG_COLOR[t] || 'bg-gray-100 text-gray-600')}>
                            {TAG_LABEL[t] || t}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className='py-2 pr-4 text-gray-500'>{c.case_count || '—'}</td>
                    <td className={cx('py-2 pr-4', debt > 0 ? 'font-medium text-rose-600' : 'text-gray-400')}>
                      {debt > 0 ? fmtMoney(debt) : '—'}
                    </td>
                    <td className={cx('py-2 pr-4 tabular-nums', c.ltv > 0 ? 'font-medium text-emerald-700' : 'text-gray-400')}>
                      {c.ltv > 0 ? fmtMoney(c.ltv) : '—'}
                    </td>
                    <td className='py-2 text-gray-500'>{fmtDate(c.last_activity)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        {filtered.length > 0 && (
          <div className='mt-4 flex items-center justify-between text-xs text-gray-500'>
            <span>
              {(pageSafe - 1) * PAGE_SIZE + 1}–{Math.min(pageSafe * PAGE_SIZE, filtered.length)} из {filtered.length}
            </span>
            <div className='flex items-center gap-2'>
              <button
                type='button'
                disabled={pageSafe <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className='rounded-md border border-gray-200 px-2 py-1 disabled:cursor-not-allowed disabled:opacity-40'
              >
                ← Назад
              </button>
              <span>
                Стр. {pageSafe} из {pageCount}
              </span>
              <button
                type='button'
                disabled={pageSafe >= pageCount}
                onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
                className='rounded-md border border-gray-200 px-2 py-1 disabled:cursor-not-allowed disabled:opacity-40'
              >
                Вперёд →
              </button>
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

// SortableHeader is a plain <th> that doubles as a sort toggle — clicking
// the currently-inactive one switches to it (always descending, both
// columns only make sense high-to-low: newest activity, biggest LTV).
function SortableHeader({
  label,
  active,
  onClick,
  last,
}: {
  label: string
  active: boolean
  onClick: () => void
  last?: boolean
}) {
  return (
    <th className={cx('py-2', !last && 'pr-4')}>
      <button
        type='button'
        onClick={onClick}
        className={cx('flex items-center gap-1 hover:text-gray-700', active && 'text-gray-700')}
      >
        {label}
        {active && <span aria-hidden>↓</span>}
      </button>
    </th>
  )
}

function SegmentPill({
  active,
  onClick,
  colorClass,
  children,
}: {
  active: boolean
  onClick: () => void
  colorClass?: string
  children: React.ReactNode
}) {
  return (
    <button
      type='button'
      onClick={onClick}
      className={cx(
        'rounded-full px-3 py-1 text-xs font-medium transition',
        active ? cx(colorClass || 'bg-gray-800 text-white', 'ring-2 ring-gray-300 ring-offset-1') : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
      )}
    >
      {children}
    </button>
  )
}
