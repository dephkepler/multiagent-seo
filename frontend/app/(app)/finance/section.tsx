'use client'

import { useState } from 'react'
import { Card } from '@/components/ui/card'
import { cx } from '@/lib/cx'

interface Props {
  title: string
  // The one number worth seeing with the block collapsed.
  summary?: React.ReactNode
  defaultOpen?: boolean
  children: React.ReactNode
}

// A block that collapses to its own bottom line. On a page this dense, reading
// seven totals is often the whole job, and the details are what you open when a
// total looks wrong.
export function Section({ title, summary, defaultOpen = true, children }: Props) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <Card>
      <div
        className={cx('flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 pb-3', open ? 'mb-4' : 'border-b-0 pb-0')}
      >
        <button
          type='button'
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className='flex min-h-[40px] items-center gap-2 text-base font-semibold text-gray-900 hover:text-emerald-700'
        >
          <span className={cx('text-gray-400 transition-transform', open && 'rotate-90')} aria-hidden>
            ▶
          </span>
          {title}
        </button>
        <div className='flex items-center gap-3 tabular-nums'>{summary}</div>
      </div>
      {open && children}
    </Card>
  )
}
