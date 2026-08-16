'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
  tags: string[]
  last_activity: string
  case_count: number
  case_fee: number
  case_paid: number
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
}
const TAG_COLOR: Record<string, string> = {
  debtor: 'border border-rose-200 bg-rose-50 text-rose-700',
  no_show_risk: 'border border-orange-200 bg-orange-50 text-orange-700',
}

function fmtMoney(n: number): string {
  return Math.round(n).toLocaleString('ru-RU') + ' ₴'
}
function fmtDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString('ru-RU')
}

export default function ClientsPage() {
  const [query, setQuery] = useState('')
  const [segmentFilter, setSegmentFilter] = useState<Segment | 'all'>('all')

  const clients = useQuery({
    queryKey: ['client-segments'],
    queryFn: () => api<ClientSegment[]>('/clients'),
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
      .sort((a, b) => new Date(b.last_activity).getTime() - new Date(a.last_activity).getTime())
  }, [clients.data, query, segmentFilter])

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
          <SegmentPill active={segmentFilter === 'all'} onClick={() => setSegmentFilter('all')}>
            Все ({clients.data?.length ?? 0})
          </SegmentPill>
          {SEGMENT_ORDER.map((s) => (
            <SegmentPill
              key={s}
              active={segmentFilter === s}
              onClick={() => setSegmentFilter(s)}
              colorClass={SEGMENT_COLOR[s]}
            >
              {SEGMENT_LABEL[s]} ({counts[s] || 0})
            </SegmentPill>
          ))}
        </div>

        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder='Поиск по имени или телефону…'
          className='mb-4 max-w-sm'
        />

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
                <th className='py-2'>Последняя активность</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={7} className='py-6 text-center text-gray-400'>
                    {clients.isLoading ? 'Загрузка…' : clients.isError ? 'Не удалось загрузить' : 'Никого не найдено'}
                  </td>
                </tr>
              )}
              {filtered.map((c) => {
                const debt = c.case_fee - c.case_paid
                return (
                  <tr key={c.client_id} className='border-t border-gray-100'>
                    <td className='py-2 pr-4 font-medium'>{c.name || '—'}</td>
                    <td className='py-2 pr-4 text-gray-500'>{c.phone || '—'}</td>
                    <td className='py-2 pr-4'>
                      <span className={cx('rounded px-2 py-0.5 text-xs font-medium', SEGMENT_COLOR[c.segment])}>
                        {SEGMENT_LABEL[c.segment] || c.segment}
                      </span>
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
                    <td className='py-2 text-gray-500'>{fmtDate(c.last_activity)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
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
