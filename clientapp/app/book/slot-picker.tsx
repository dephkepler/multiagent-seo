'use client'

import { useMemo, useState } from 'react'
import { Chip } from '@/components/ui/button'
import { dayKey, formatDay, formatTime } from '@/lib/format'

/**
 * Two steps, day then hour, rather than one long list. Fourteen days of eight
 * hours is over a hundred buttons, and scrolling past a hundred to reach next
 * Tuesday is how a booking form loses people.
 */
export function SlotPicker({
  slots,
  selected,
  onSelect,
}: {
  slots: string[]
  selected: string | null
  onSelect: (slot: string | null) => void
}) {
  const days = useMemo(() => groupByDay(slots), [slots])
  const [openDay, setOpenDay] = useState<string | null>(() => days[0]?.key ?? null)

  if (slots.length === 0) {
    return (
      <p className='text-body text-ink-2'>
        Вільних годин найближчим часом немає. Надішліть заявку без часу — ми зателефонуємо і підберемо.
      </p>
    )
  }

  const day = days.find((d) => d.key === openDay) ?? days[0]

  return (
    <div className='flex flex-col gap-3'>
      <div className='-mx-4 flex gap-2 overflow-x-auto px-4 pb-1'>
        {days.map((d) => (
          <Chip
            key={d.key}
            selected={d.key === day.key}
            onClick={() => setOpenDay(d.key)}
            className='shrink-0 whitespace-nowrap'
          >
            {formatDay(d.slots[0])}
          </Chip>
        ))}
      </div>

      <div className='grid grid-cols-4 gap-2'>
        {day.slots.map((slot) => (
          <Chip
            key={slot}
            selected={slot === selected}
            // Tapping the chosen hour again clears it, which is the only way
            // back to "no time, call me" once something is picked.
            onClick={() => onSelect(slot === selected ? null : slot)}
            className='justify-center text-center'
          >
            {formatTime(slot)}
          </Chip>
        ))}
      </div>
    </div>
  )
}

function groupByDay(slots: string[]): Array<{ key: string; slots: string[] }> {
  const days = new Map<string, string[]>()
  for (const slot of slots) {
    const key = dayKey(slot)
    const existing = days.get(key)
    if (existing) existing.push(slot)
    else days.set(key, [slot])
  }
  return [...days.entries()].map(([key, slots]) => ({ key, slots }))
}
