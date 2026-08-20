'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
  manual_tags: string[]
  last_activity: string
  case_count: number
  case_fee: number
  case_paid: number
  ltv: number
}

interface ClientList {
  items: ClientSegment[]
  total: number
  segment_counts: Partial<Record<Segment, number>>
}

interface TagDef {
  label: string
  created_at: string
}
interface TagDefList {
  items: TagDef[]
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
  // Filtering/sorting now happens server-side (see GET /clients), so typing
  // fires a real HTTP request per change — debounce keeps that to one
  // request per pause in typing, not one per keystroke.
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [segmentFilter, setSegmentFilter] = useState<Segment | 'all'>('all')
  const [page, setPage] = useState(1)
  const [sortKey, setSortKey] = useState<SortKey>('activity')
  const [manageTagsOpen, setManageTagsOpen] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => clearTimeout(t)
  }, [query])

  const params = new URLSearchParams()
  if (segmentFilter !== 'all') params.set('segment', segmentFilter)
  if (debouncedQuery) params.set('search', debouncedQuery)
  params.set('sort', sortKey)
  params.set('limit', String(PAGE_SIZE))
  params.set('offset', String((page - 1) * PAGE_SIZE))
  const queryString = params.toString()

  const clients = useQuery({
    queryKey: ['client-segments', queryString],
    queryFn: () => api<ClientList>(`/clients?${queryString}`),
    placeholderData: keepPreviousData,
  })

  const setOverride = useMutation({
    mutationFn: ({ id, segment }: { id: string; segment: Segment | null }) =>
      api(`/clients/${id}/segment`, { method: 'PATCH', body: JSON.stringify({ segment_override: segment }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  const addTag = useMutation({
    mutationFn: ({ id, tag }: { id: string; tag: string }) =>
      api(`/clients/${id}/tags`, { method: 'POST', body: JSON.stringify({ tag }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  const removeTag = useMutation({
    mutationFn: ({ id, tag }: { id: string; tag: string }) =>
      api(`/clients/${id}/tags/${encodeURIComponent(tag)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  // The manual-tag vocabulary — a small curated list, so one query shared
  // across every row's dropdown and the "Управление тегами" panel below.
  const tagDefs = useQuery({
    queryKey: ['client-tag-defs'],
    queryFn: () => api<TagDefList>('/clients/tag-defs'),
  })
  const tagDefLabels = (tagDefs.data?.items ?? []).map((d) => d.label)

  const createTagDef = useMutation({
    mutationFn: (label: string) => api('/clients/tag-defs', { method: 'POST', body: JSON.stringify({ label }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-tag-defs'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  const renameTagDef = useMutation({
    mutationFn: ({ oldLabel, newLabel }: { oldLabel: string; newLabel: string }) =>
      api(`/clients/tag-defs/${encodeURIComponent(oldLabel)}`, { method: 'PATCH', body: JSON.stringify({ label: newLabel }) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['client-tag-defs'] })
      qc.invalidateQueries({ queryKey: ['client-segments'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const deleteTagDef = useMutation({
    mutationFn: (label: string) => api(`/clients/tag-defs/${encodeURIComponent(label)}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['client-tag-defs'] })
      qc.invalidateQueries({ queryKey: ['client-segments'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const items = clients.data?.items ?? []
  const total = clients.data?.total ?? 0
  const counts = clients.data?.segment_counts ?? {}
  const totalAll = SEGMENT_ORDER.reduce((sum, s) => sum + (counts[s] || 0), 0)
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const pageSafe = Math.min(page, pageCount)

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
          <div className='flex items-center gap-3'>
            <span className='text-xs text-gray-400'>
              Снимок по всей истории, без периода — не путать с фильтром дат на «Заявках»
            </span>
            <button
              type='button'
              onClick={() => setManageTagsOpen((v) => !v)}
              className='shrink-0 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50'
            >
              ⚙ Управление тегами
            </button>
          </div>
        </div>

        {manageTagsOpen && (
          <ManageTagsPanel
            defs={tagDefs.data?.items ?? []}
            loading={tagDefs.isLoading}
            onCreate={(label) => createTagDef.mutate(label)}
            onRename={(oldLabel, newLabel) => renameTagDef.mutate({ oldLabel, newLabel })}
            onDelete={(label) => deleteTagDef.mutate(label)}
          />
        )}

        <div className='mb-4 flex flex-wrap gap-2'>
          <SegmentPill active={segmentFilter === 'all'} onClick={() => onSegmentFilterChange('all')}>
            Все ({totalAll})
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
              {items.length === 0 && (
                <tr>
                  <td colSpan={8} className='py-6 text-center text-gray-400'>
                    {clients.isLoading ? 'Загрузка…' : clients.isError ? 'Не удалось загрузить' : 'Никого не найдено'}
                  </td>
                </tr>
              )}
              {items.map((c) => {
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
                      <TagsCell
                        tags={c.tags}
                        manualTags={c.manual_tags}
                        availableTags={tagDefLabels}
                        pending={addTag.isPending || removeTag.isPending}
                        onAdd={(tag) => addTag.mutate({ id: c.client_id, tag })}
                        onRemove={(tag) => removeTag.mutate({ id: c.client_id, tag })}
                      />
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

        {total > 0 && (
          <div className='mt-4 flex items-center justify-between text-xs text-gray-500'>
            <span>
              {(pageSafe - 1) * PAGE_SIZE + 1}–{Math.min(pageSafe * PAGE_SIZE, total)} из {total}
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

// TagsCell renders the four auto-computed tags read-only, every manual tag
// as a removable chip, and a dropdown to add one more — the dropdown only
// ever offers labels from the curated vocabulary (client_tag_defs), never
// free text, so a tag can't drift into a one-off spelling. The vocabulary
// itself is managed separately, see ManageTagsPanel.
function TagsCell({
  tags,
  manualTags,
  availableTags,
  onAdd,
  onRemove,
  pending,
}: {
  tags: string[]
  manualTags: string[]
  availableTags: string[]
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
  pending: boolean
}) {
  const remaining = availableTags.filter((t) => !manualTags.includes(t))

  return (
    <div className='flex flex-wrap items-center gap-1'>
      {tags.map((t) => (
        <span key={t} className={cx('rounded px-1.5 py-0.5 text-[11px] font-medium', TAG_COLOR[t] || 'bg-gray-100 text-gray-600')}>
          {TAG_LABEL[t] || t}
        </span>
      ))}
      {manualTags.map((t) => (
        <span
          key={t}
          className='inline-flex items-center gap-1 rounded border border-dashed border-gray-300 bg-white px-1.5 py-0.5 text-[11px] font-medium text-gray-600'
        >
          {t}
          <button
            type='button'
            disabled={pending}
            onClick={() => onRemove(t)}
            aria-label={`Убрать тег ${t}`}
            className='leading-none text-gray-400 hover:text-rose-600 disabled:cursor-wait'
          >
            ×
          </button>
        </span>
      ))}
      {remaining.length > 0 && (
        <select
          value=''
          disabled={pending}
          onChange={(e) => {
            if (e.target.value) onAdd(e.target.value)
          }}
          className='rounded border border-dashed border-gray-300 bg-white px-1 py-0.5 text-[11px] text-gray-400 outline-none disabled:cursor-wait'
        >
          <option value=''>+ тег</option>
          {remaining.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      )}
    </div>
  )
}

// ManageTagsPanel is the vocabulary's own CRUD — separate from TagsCell,
// which only ever picks from it. Rename cascades to every client carrying
// the old label (see backend clientsegments.RenameTagDef); delete removes
// it from every client too.
function ManageTagsPanel({
  defs,
  loading,
  onCreate,
  onRename,
  onDelete,
}: {
  defs: TagDef[]
  loading: boolean
  onCreate: (label: string) => void
  onRename: (oldLabel: string, newLabel: string) => void
  onDelete: (label: string) => void
}) {
  const [newLabel, setNewLabel] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')

  function submitCreate() {
    const label = newLabel.trim()
    setNewLabel('')
    if (label) onCreate(label)
  }

  function submitRename(oldLabel: string) {
    const label = editValue.trim()
    setEditing(null)
    if (label && label !== oldLabel) onRename(oldLabel, label)
  }

  return (
    <div className='mb-4 rounded-md border border-gray-200 bg-gray-50/60 p-3'>
      <div className='mb-2 text-xs font-medium text-gray-500'>
        Список тегов — переименование применяется сразу ко всем клиентам с этим тегом
      </div>
      {loading ? (
        <p className='text-sm text-gray-400'>Загрузка…</p>
      ) : defs.length === 0 ? (
        <p className='mb-2 text-sm text-gray-400'>Тегов ещё нет.</p>
      ) : (
        <div className='mb-2 flex flex-wrap gap-2'>
          {defs.map((d) =>
            editing === d.label ? (
              <input
                key={d.label}
                autoFocus
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') submitRename(d.label)
                  if (e.key === 'Escape') setEditing(null)
                }}
                onBlur={() => submitRename(d.label)}
                maxLength={40}
                className='w-32 rounded border border-emerald-300 px-1.5 py-0.5 text-xs outline-none'
              />
            ) : (
              <span
                key={d.label}
                className='inline-flex items-center gap-1.5 rounded border border-gray-200 bg-white px-1.5 py-0.5 text-xs text-gray-700'
              >
                <button
                  type='button'
                  onClick={() => {
                    setEditing(d.label)
                    setEditValue(d.label)
                  }}
                  className='hover:underline'
                  title='Переименовать'
                >
                  {d.label}
                </button>
                <button
                  type='button'
                  onClick={() => onDelete(d.label)}
                  aria-label={`Удалить тег ${d.label}`}
                  className='leading-none text-gray-400 hover:text-rose-600'
                >
                  ×
                </button>
              </span>
            )
          )}
        </div>
      )}
      <div className='flex items-center gap-2'>
        <input
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') submitCreate()
          }}
          maxLength={40}
          placeholder='Новый тег…'
          className='w-40 rounded border border-gray-200 px-1.5 py-1 text-xs outline-none focus:border-emerald-400'
        />
        <button
          type='button'
          disabled={!newLabel.trim()}
          onClick={submitCreate}
          className='rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-600 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40'
        >
          Добавить
        </button>
      </div>
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
