'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { categoryColorClass, sortTagsByCategory, TAG_BADGE_VARIANT, TAG_LABEL } from '@/lib/client-tags'
import { cx } from '@/lib/cx'

export interface TagDef {
  label: string
  category: string
  created_at: string
}

// The one place a client's tags render — used by both the clients list
// (one table cell per row) and the single-client card (one editor in the
// Overview). Used to be two near-identical components that quietly drifted
// apart: manual tags rendered as a small rounded *rectangle* here, a pill
// there, and the add-tag control was a bare circle in one and a different
// circle in the other. One component now, one shape (a rounded-full
// "capsule") for every chip and for the add-tag control, so a tag looks and
// behaves the same wherever it shows up.
export function TagChips({
  tags,
  manualTags,
  categories,
  defsByCategory,
  onAdd,
  onRemove,
  pending,
  maxVisible = 4,
  className,
}: {
  tags: string[]
  manualTags: string[]
  categories: string[]
  defsByCategory: Map<string, TagDef[]>
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
  pending: boolean
  maxVisible?: number
  className?: string
}) {
  const [expanded, setExpanded] = useState(false)
  const labelToCategory = new Map<string, string>()
  for (const [category, defs] of defsByCategory) {
    for (const d of defs) labelToCategory.set(d.label, category)
  }
  const hasRemaining = categories.some((category) =>
    (defsByCategory.get(category) ?? []).some((d) => !manualTags.includes(d.label))
  )
  const sortedManual = sortTagsByCategory(manualTags, categories, labelToCategory)

  type Chip = { kind: 'auto' | 'manual'; value: string }
  const chips: Chip[] = [
    ...tags.map((t): Chip => ({ kind: 'auto', value: t })),
    ...sortedManual.map((t): Chip => ({ kind: 'manual', value: t })),
  ]
  const overflow = chips.length - maxVisible
  const visibleChips = expanded ? chips : chips.slice(0, maxVisible)

  return (
    <div className={cx('flex flex-wrap items-center gap-1.5', className)}>
      {chips.length === 0 && !hasRemaining && <span className='text-sm text-gray-400'>—</span>}
      {visibleChips.map((chip) =>
        chip.kind === 'auto' ? (
          <Badge key={`auto-${chip.value}`} variant={TAG_BADGE_VARIANT[chip.value] || 'neutral'}>
            {TAG_LABEL[chip.value] || chip.value}
          </Badge>
        ) : (
          <span
            key={`manual-${chip.value}`}
            className={cx(
              'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium',
              categoryColorClass(labelToCategory.get(chip.value) ?? '', categories)
            )}
          >
            {chip.value}
            <button
              type='button'
              disabled={pending}
              onClick={() => onRemove(chip.value)}
              aria-label={`Убрать тег ${chip.value}`}
              className='leading-none opacity-60 hover:text-rose-600 hover:opacity-100 disabled:cursor-wait'
            >
              ×
            </button>
          </span>
        )
      )}
      {!expanded && overflow > 0 && (
        <button
          type='button'
          onClick={() => setExpanded(true)}
          className='text-[11px] font-medium text-gray-400 hover:text-gray-600 hover:underline'
        >
          +{overflow}
        </button>
      )}
      {expanded && chips.length > maxVisible && (
        <button
          type='button'
          onClick={() => setExpanded(false)}
          className='text-[11px] text-gray-400 hover:text-gray-600 hover:underline'
        >
          свернуть
        </button>
      )}
      {hasRemaining && (
        <select
          value=''
          disabled={pending}
          onChange={(e) => {
            if (e.target.value) onAdd(e.target.value)
          }}
          aria-label='Добавить тег'
          title='Добавить тег'
          className='h-[22px] cursor-pointer appearance-none rounded-full border border-dashed border-gray-300 bg-white px-2 text-[11px] font-medium text-gray-400 outline-none hover:border-gray-400 hover:text-gray-600 disabled:cursor-wait'
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
      )}
    </div>
  )
}
