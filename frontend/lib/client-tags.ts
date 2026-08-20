import type { Variant as BadgeVariant } from '@/components/ui/badge'

// Shared between the clients list and a single client's card so a segment/tag
// reads as the same color/label wherever staff sees it — duplicating this map
// per page risks exactly the drift this file exists to prevent (see
// categoryColorClass below).
export type Segment = 'lead' | 'booked' | 'consulted' | 'client' | 'repeat' | 'lost'

export const SEGMENT_LABEL: Record<Segment, string> = {
  lead: 'Заявка',
  booked: 'Забронировал',
  consulted: 'Проконсультирован',
  client: 'Клиент',
  repeat: 'Повторный',
  lost: 'Потерян',
}
// Порядок — как в воронке (см. backend clientsegments.Derive), не алфавитный.
export const SEGMENT_ORDER: Segment[] = ['lead', 'booked', 'consulted', 'client', 'repeat', 'lost']
export const SEGMENT_COLOR: Record<Segment, string> = {
  lead: 'bg-gray-100 text-gray-700',
  booked: 'bg-sky-100 text-sky-800',
  consulted: 'bg-amber-100 text-amber-800', // самый денежный сегмент — есть кого дожимать
  client: 'bg-emerald-100 text-emerald-800',
  repeat: 'bg-violet-100 text-violet-800',
  lost: 'bg-rose-100 text-rose-800',
}

export const TAG_LABEL: Record<string, string> = {
  debtor: 'Должник',
  no_show_risk: 'Риск неявки',
  high_value: 'Ценный клиент',
  dormant: 'Без контакта 90+ дней',
}
export const TAG_BADGE_VARIANT: Record<string, BadgeVariant> = {
  debtor: 'danger',
  no_show_risk: 'warning',
  high_value: 'success',
  dormant: 'neutral',
}

// One color per category, assigned by position in the *alphabetically
// sorted* category list — sorted so the assignment doesn't depend on
// whatever order the API happens to return defs in (that's not guaranteed
// stable), and gives manual tags a real visual "level" (which group
// they're in) instead of all looking the same regardless of category.
// Deliberately avoids gray/sky/amber/emerald/violet/rose/orange — every
// hue SEGMENT_COLOR or TAG_BADGE_VARIANT already uses on this same row —
// so a manual tag's color can't be mistaken for a segment or an auto-tag.
export const CATEGORY_PALETTE = [
  'border-teal-200 bg-teal-50 text-teal-700',
  'border-cyan-200 bg-cyan-50 text-cyan-700',
  'border-fuchsia-200 bg-fuchsia-50 text-fuchsia-700',
  'border-indigo-200 bg-indigo-50 text-indigo-700',
  'border-lime-200 bg-lime-50 text-lime-700',
  'border-blue-200 bg-blue-50 text-blue-700',
]
export function categoryColorClass(category: string, categories: string[]): string {
  const sorted = [...categories].sort()
  const idx = Math.max(0, sorted.indexOf(category))
  return CATEGORY_PALETTE[idx % CATEGORY_PALETTE.length]
}
