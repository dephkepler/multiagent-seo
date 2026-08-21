'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { SectionHeader } from '@/components/ui/section-header'
import { cx } from '@/lib/cx'
import {
  categoryColorClass,
  SEGMENT_COLOR,
  SEGMENT_LABEL,
  SEGMENT_ORDER,
  type Segment,
  TAG_BADGE_VARIANT,
  TAG_LABEL,
} from '@/lib/client-tags'

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
  category: string
  created_at: string
}
interface TagDefList {
  items: TagDef[]
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
  const [tagFilter, setTagFilter] = useState<string>('all')
  const [page, setPage] = useState(1)
  const [sortKey, setSortKey] = useState<SortKey>('activity')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [manageTagsOpen, setManageTagsOpen] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => clearTimeout(t)
  }, [query])

  const params = new URLSearchParams()
  if (segmentFilter !== 'all') params.set('segment', segmentFilter)
  if (tagFilter !== 'all') params.set('tag', tagFilter)
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
  // across every row's dropdown, the filter row, and the management panel.
  const tagDefs = useQuery({
    queryKey: ['client-tag-defs'],
    queryFn: () => api<TagDefList>('/clients/tag-defs'),
  })
  const defs = tagDefs.data?.items ?? []
  const categories = [...new Set(defs.map((d) => d.category))]
  const defsByCategory = new Map<string, TagDef[]>()
  for (const d of defs) {
    defsByCategory.set(d.category, [...(defsByCategory.get(d.category) ?? []), d])
  }

  const createTagDef = useMutation({
    mutationFn: ({ label, category }: { label: string; category: string }) =>
      api('/clients/tag-defs', { method: 'POST', body: JSON.stringify({ label, category }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-tag-defs'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  const updateTagDef = useMutation({
    mutationFn: ({ label, newLabel, newCategory }: { label: string; newLabel?: string; newCategory?: string }) =>
      api(`/clients/tag-defs/${encodeURIComponent(label)}`, {
        method: 'PATCH',
        body: JSON.stringify({ label: newLabel, category: newCategory }),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['client-tag-defs'] })
      qc.invalidateQueries({ queryKey: ['client-segments'] })
      // Same reasoning as deleteTagDef below: a filter stuck on the old
      // label would silently match nothing once every client_tags row has
      // cascaded to the new one.
      if (vars.newLabel) setTagFilter((f) => (f === vars.label ? vars.newLabel! : f))
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const deleteTagDef = useMutation({
    mutationFn: (label: string) => api(`/clients/tag-defs/${encodeURIComponent(label)}`, { method: 'DELETE' }),
    onSuccess: (_data, deletedLabel) => {
      qc.invalidateQueries({ queryKey: ['client-tag-defs'] })
      qc.invalidateQueries({ queryKey: ['client-segments'] })
      // A filter stuck on a now-gone label would silently match nothing —
      // reset it instead of leaving staff staring at an empty table.
      setTagFilter((f) => (f === deletedLabel ? 'all' : f))
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const activeFilterCount = (segmentFilter !== 'all' ? 1 : 0) + (tagFilter !== 'all' ? 1 : 0)

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
  const onTagFilterChange = resetToFirstPage(setTagFilter)
  const onSortKeyChange = resetToFirstPage(setSortKey)

  return (
    <div className='space-y-6'>
      <Card>
        <SectionHeader title='Клиенты' as='h1' />
        <p className='-mt-2 mb-4 text-xs text-gray-400'>
          Снимок по всей истории, без периода — не путать с фильтром дат на «Заявках»
        </p>

        <div className='mb-3 flex flex-wrap items-center gap-2'>
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder='Поиск по имени или телефону…'
            className='w-full sm:max-w-sm'
          />
          <Button type='button' variant='secondary' size='sm' onClick={() => setFiltersOpen((v) => !v)}>
            Фильтры{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''} {filtersOpen ? '▴' : '▾'}
          </Button>
        </div>

        {filtersOpen && (
          <div className='mb-4 space-y-3 rounded-md border border-gray-200 bg-gray-50/60 p-3'>
            <div>
              <div className='mb-1.5 text-[11px] font-medium text-gray-500'>Сегмент</div>
              <div className='flex flex-wrap gap-2'>
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
              <p className='mt-1.5 text-[11px] text-gray-400'>
                Считается сам — выберите вручную в списке, чтобы закрепить свой (например клиент ушёл к другому
                адвокату)
              </p>
            </div>

            {defs.length > 0 && (
              <div className='border-t border-gray-200 pt-2.5'>
                <div className='mb-1.5 text-[11px] font-medium text-gray-500'>Теги</div>
                <div className='flex flex-wrap items-center gap-x-4 gap-y-1.5'>
                  <TagFilterPill active={tagFilter === 'all'} onClick={() => onTagFilterChange('all')}>
                    Все теги
                  </TagFilterPill>
                  {categories.map((category) => (
                    <div key={category} className='flex flex-wrap items-center gap-1.5'>
                      <span className='text-[11px] whitespace-nowrap text-gray-400'>{category}:</span>
                      {(defsByCategory.get(category) ?? []).map((d) => (
                        <TagFilterPill
                          key={d.label}
                          active={tagFilter === d.label}
                          onClick={() => onTagFilterChange(d.label)}
                          colorClass={categoryColorClass(category, categories)}
                        >
                          {d.label}
                        </TagFilterPill>
                      ))}
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className='border-t border-gray-200 pt-2.5'>
              <button
                type='button'
                onClick={() => setManageTagsOpen((v) => !v)}
                className='text-[11px] font-medium text-gray-500 hover:text-gray-700 hover:underline'
              >
                ⚙ Управление тегами {manageTagsOpen ? '▴' : '▾'}
              </button>
              {manageTagsOpen && (
                <ManageTagsPanel
                  categories={categories}
                  defsByCategory={defsByCategory}
                  onCreate={(label, category) => createTagDef.mutate({ label, category })}
                  onRenameLabel={(label, newLabel) => updateTagDef.mutate({ label, newLabel })}
                  onMoveCategory={(label, newCategory) => updateTagDef.mutate({ label, newCategory })}
                  onDelete={(label) => deleteTagDef.mutate(label)}
                />
              )}
            </div>
          </div>
        )}

        {clients.isLoading && <div className='py-6 text-center text-sm text-gray-400'>Загрузка…</div>}
        {clients.isError && !clients.isLoading && (
          <div className='py-6 text-center text-sm text-rose-600'>Не удалось загрузить</div>
        )}
        {!clients.isLoading && !clients.isError && items.length === 0 && (
          <div className='py-6 text-center text-sm text-gray-400'>Никого не найдено</div>
        )}

        {items.length > 0 && (
          <div className={clients.isFetching ? 'opacity-50 transition-opacity' : undefined}>
            {/* mobile: cards — an 8-column table is unreadable on a phone even with horizontal scroll */}
            <ul className='grid gap-2 md:hidden'>
              {items.map((c) => {
                const debt = c.case_fee - c.case_paid
                const segmentPending = setOverride.isPending && setOverride.variables?.id === c.client_id
                return (
                  <li key={c.client_id} className='rounded-md border border-gray-200 bg-white p-3'>
                    <div className='flex items-start justify-between gap-2'>
                      <Link
                        href={`/clients/${c.client_id}`}
                        className='min-w-0 truncate font-medium text-emerald-700 hover:underline'
                        title={c.name || undefined}
                      >
                        {c.name || '—'}
                      </Link>
                      <span className='shrink-0 text-xs text-gray-500'>{c.phone || '—'}</span>
                    </div>

                    <div className='mt-2 flex items-center gap-1.5'>
                      <select
                        value={c.segment}
                        disabled={setOverride.isPending}
                        onChange={(e) => setOverride.mutate({ id: c.client_id, segment: e.target.value as Segment })}
                        className={cx(
                          'cursor-pointer rounded px-2 py-1 text-xs font-medium outline-none disabled:cursor-wait',
                          SEGMENT_COLOR[c.segment]
                        )}
                      >
                        {SEGMENT_ORDER.map((s) => (
                          <option key={s} value={s}>
                            {SEGMENT_LABEL[s]}
                          </option>
                        ))}
                      </select>
                      {segmentPending && <span className='text-xs text-gray-400'>…</span>}
                    </div>

                    <div className='mt-2'>
                      <TagsCell
                        tags={c.tags}
                        manualTags={c.manual_tags}
                        categories={categories}
                        defsByCategory={defsByCategory}
                        pending={addTag.isPending || removeTag.isPending}
                        onAdd={(tag) => addTag.mutate({ id: c.client_id, tag })}
                        onRemove={(tag) => removeTag.mutate({ id: c.client_id, tag })}
                      />
                    </div>

                    <dl className='mt-2.5 grid grid-cols-2 gap-y-1 border-t border-gray-100 pt-2 text-xs'>
                      <div className='flex items-center justify-between pr-2'>
                        <dt className='text-gray-400'>Дел</dt>
                        <dd className='text-gray-600'>{c.case_count || '—'}</dd>
                      </div>
                      <div className='flex items-center justify-between'>
                        <dt className='text-gray-400'>Долг</dt>
                        <dd className={debt > 0 ? 'font-medium text-rose-600' : 'text-gray-400'}>
                          {debt > 0 ? fmtMoney(debt) : '—'}
                        </dd>
                      </div>
                      <div className='flex items-center justify-between pr-2'>
                        <dt className='text-gray-400'>LTV</dt>
                        <dd className={cx('tabular-nums', c.ltv > 0 ? 'font-medium text-emerald-700' : 'text-gray-400')}>
                          {c.ltv > 0 ? fmtMoney(c.ltv) : '—'}
                        </dd>
                      </div>
                      <div className='flex items-center justify-between'>
                        <dt className='text-gray-400'>Активность</dt>
                        <dd className='text-gray-600'>{fmtDate(c.last_activity)}</dd>
                      </div>
                    </dl>
                  </li>
                )
              })}
            </ul>

            <div className='hidden overflow-x-auto md:block'>
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
                  {items.map((c) => {
                    const debt = c.case_fee - c.case_paid
                    const segmentPending = setOverride.isPending && setOverride.variables?.id === c.client_id
                    return (
                      <tr key={c.client_id} className='border-t border-gray-100'>
                        <td className='max-w-[180px] py-2 pr-4 font-medium'>
                          <Link
                            href={`/clients/${c.client_id}`}
                            className='block truncate text-emerald-700 hover:underline'
                            title={c.name || undefined}
                          >
                            {c.name || '—'}
                          </Link>
                        </td>
                        <td className='max-w-[140px] py-2 pr-4 text-gray-500'>
                          <span className='block truncate' title={c.phone || undefined}>
                            {c.phone || '—'}
                          </span>
                        </td>
                        <td className='py-2 pr-4'>
                          <div className='flex items-center gap-1.5'>
                            <select
                              value={c.segment}
                              disabled={setOverride.isPending}
                              onChange={(e) => setOverride.mutate({ id: c.client_id, segment: e.target.value as Segment })}
                              className={cx(
                                'cursor-pointer rounded px-2 py-1 text-xs font-medium outline-none disabled:cursor-wait',
                                SEGMENT_COLOR[c.segment]
                              )}
                            >
                              {SEGMENT_ORDER.map((s) => (
                                <option key={s} value={s}>
                                  {SEGMENT_LABEL[s]}
                                </option>
                              ))}
                            </select>
                            {segmentPending && <span className='text-xs text-gray-400'>…</span>}
                          </div>
                        </td>
                        <td className='py-2 pr-4'>
                          <TagsCell
                            tags={c.tags}
                            manualTags={c.manual_tags}
                            categories={categories}
                            defsByCategory={defsByCategory}
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
          </div>
        )}

        {total > 0 && (
          <div className='mt-4 flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500'>
            <span>
              {(pageSafe - 1) * PAGE_SIZE + 1}–{Math.min(pageSafe * PAGE_SIZE, total)} из {total}
            </span>
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                disabled={pageSafe <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                ← Назад
              </Button>
              <span className='shrink-0'>
                Стр. {pageSafe} из {pageCount}
              </span>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                disabled={pageSafe >= pageCount}
                onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
              >
                Вперёд →
              </Button>
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
// as a chip colored by its category, and a grouped dropdown (native
// <select> with one <optgroup> per category — keeps the picker inside the
// browser's own floating layer, so it can't be clipped by the table's
// horizontal scroll container the way a hand-built popover could) to add
// one more. The dropdown only ever offers labels from the curated
// vocabulary, never free text — see ManageTagsPanel for managing the list
// itself.
function TagsCell({
  tags,
  manualTags,
  categories,
  defsByCategory,
  onAdd,
  onRemove,
  pending,
}: {
  tags: string[]
  manualTags: string[]
  categories: string[]
  defsByCategory: Map<string, TagDef[]>
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
  pending: boolean
}) {
  const labelToCategory = new Map<string, string>()
  for (const [category, defs] of defsByCategory) {
    for (const d of defs) labelToCategory.set(d.label, category)
  }
  const hasRemaining = categories.some((category) =>
    (defsByCategory.get(category) ?? []).some((d) => !manualTags.includes(d.label))
  )

  return (
    <div className='flex flex-wrap items-center gap-1'>
      {tags.map((t) => (
        <Badge key={t} variant={TAG_BADGE_VARIANT[t] || 'neutral'}>
          {TAG_LABEL[t] || t}
        </Badge>
      ))}
      {manualTags.map((t) => (
        <span
          key={t}
          className={cx(
            'inline-flex items-center gap-0.5 rounded border px-1 py-px text-[10px] font-medium',
            categoryColorClass(labelToCategory.get(t) ?? '', categories)
          )}
        >
          {t}
          <button
            type='button'
            disabled={pending}
            onClick={() => onRemove(t)}
            aria-label={`Убрать тег ${t}`}
            className='px-0.5 leading-none opacity-60 hover:text-rose-600 hover:opacity-100 disabled:cursor-wait'
          >
            ×
          </button>
        </span>
      ))}
      {hasRemaining && (
        <div className='relative'>
          <select
            value=''
            disabled={pending}
            onChange={(e) => {
              if (e.target.value) onAdd(e.target.value)
            }}
            aria-label='Добавить тег'
            className='min-h-[22px] cursor-pointer appearance-none rounded-full border border-gray-200 bg-gray-50 py-px pr-3.5 pl-1.5 text-[10px] font-medium text-gray-500 outline-none hover:bg-gray-100 disabled:cursor-wait'
          >
            <option value=''>+ тег</option>
            {categories.map((category) => {
              const remaining = (defsByCategory.get(category) ?? []).filter((d) => !manualTags.includes(d.label))
              if (remaining.length === 0) return null
              return (
                <optgroup key={category} label={category}>
                  {remaining.map((d) => (
                    <option key={d.label} value={d.label}>
                      {d.label}
                    </option>
                  ))}
                </optgroup>
              )
            })}
          </select>
          <span className='pointer-events-none absolute top-1/2 right-0.5 -translate-y-1/2 text-[8px] text-gray-400'>▾</span>
        </div>
      )}
    </div>
  )
}

// ManageTagsPanel is the vocabulary's own CRUD, grouped by category —
// separate from TagsCell, which only ever picks from it. Renaming a label
// cascades to every client carrying it (see backend
// clientsegments.UpdateTagDef); deleting one removes it from every client
// too.
function ManageTagsPanel({
  categories,
  defsByCategory,
  onCreate,
  onRenameLabel,
  onMoveCategory,
  onDelete,
}: {
  categories: string[]
  defsByCategory: Map<string, TagDef[]>
  onCreate: (label: string, category: string) => void
  onRenameLabel: (label: string, newLabel: string) => void
  onMoveCategory: (label: string, newCategory: string) => void
  onDelete: (label: string) => void
}) {
  const [newLabel, setNewLabel] = useState('')
  const [newCategory, setNewCategory] = useState('')
  const [customCategory, setCustomCategory] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')

  function submitCreate() {
    const label = newLabel.trim()
    const category = newCategory.trim()
    if (label && category) onCreate(label, category)
    setNewLabel('')
    setNewCategory('')
    setCustomCategory(false)
  }

  function submitRename(label: string) {
    const value = editValue.trim()
    setEditing(null)
    if (value && value !== label) onRenameLabel(label, value)
  }

  return (
    <div className='mt-2 rounded-md border border-gray-200 bg-white p-3'>
      <div className='mb-3 text-xs font-medium text-gray-500'>
        Словарь тегов, по категориям — переименование или смена категории применяется сразу ко всем клиентам с этим
        тегом
      </div>

      {categories.length === 0 ? (
        <p className='mb-3 text-sm text-gray-400'>Тегов ещё нет.</p>
      ) : (
        <div className='mb-3 space-y-2'>
          {categories.map((category) => (
            <div key={category} className='flex flex-wrap items-center gap-1.5'>
              <span className='w-28 shrink-0 text-xs font-medium text-gray-500'>{category}</span>
              {(defsByCategory.get(category) ?? []).map((d) =>
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
                    maxLength={24}
                    className='w-32 rounded border border-emerald-300 px-1.5 py-0.5 text-xs outline-none'
                  />
                ) : (
                  <span
                    key={d.label}
                    className={cx(
                      'inline-flex items-center gap-1.5 rounded border px-1.5 py-0.5 text-xs',
                      categoryColorClass(category, categories)
                    )}
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
                    <select
                      value={category}
                      onChange={(e) => onMoveCategory(d.label, e.target.value)}
                      title='Переместить в другую категорию'
                      className='cursor-pointer appearance-none bg-transparent pr-2 text-xs opacity-60 outline-none hover:opacity-100'
                    >
                      {categories.map((c) => (
                        <option key={c} value={c}>
                          {c}
                        </option>
                      ))}
                    </select>
                    <button
                      type='button'
                      onClick={() => onDelete(d.label)}
                      aria-label={`Удалить тег ${d.label}`}
                      className='px-1 leading-none opacity-60 hover:text-rose-600 hover:opacity-100'
                    >
                      ×
                    </button>
                  </span>
                )
              )}
            </div>
          ))}
        </div>
      )}

      <div className='flex flex-wrap items-center gap-2 border-t border-gray-200 pt-2'>
        <input
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          maxLength={24}
          placeholder='Новый тег…'
          className='w-36 rounded border border-gray-200 px-1.5 py-1 text-xs outline-none focus:border-emerald-400'
        />
        {customCategory || categories.length === 0 ? (
          <input
            value={newCategory}
            onChange={(e) => setNewCategory(e.target.value)}
            maxLength={24}
            placeholder='Новая категория…'
            className='w-36 rounded border border-gray-200 px-1.5 py-1 text-xs outline-none focus:border-emerald-400'
          />
        ) : (
          <select
            value={newCategory}
            onChange={(e) => {
              if (e.target.value === '__new__') {
                setCustomCategory(true)
                setNewCategory('')
              } else {
                setNewCategory(e.target.value)
              }
            }}
            className='rounded border border-gray-200 px-1.5 py-1 text-xs text-gray-600 outline-none'
          >
            <option value=''>Категория…</option>
            {categories.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
            <option value='__new__'>+ новая категория</option>
          </select>
        )}
        <button
          type='button'
          disabled={!newLabel.trim() || !newCategory.trim()}
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
        'rounded-full px-3 py-1.5 text-xs font-medium transition',
        active ? cx(colorClass || 'bg-gray-800 text-white', 'ring-2 ring-gray-300 ring-offset-1') : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
      )}
    >
      {children}
    </button>
  )
}

// TagFilterPill mirrors SegmentPill but stays outline-styled even when
// inactive (colorClass already carries a border+bg+text triplet from
// categoryColorClass) — a solid fill on every one of a dozen+ tags would be
// louder than the segment row above it, which only ever has six.
function TagFilterPill({
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
        'rounded-full border px-2.5 py-1 text-[11px] font-medium transition',
        colorClass || 'border-gray-200 bg-white text-gray-600',
        active ? 'ring-2 ring-gray-300 ring-offset-1' : 'opacity-70 hover:opacity-100'
      )}
    >
      {children}
    </button>
  )
}
