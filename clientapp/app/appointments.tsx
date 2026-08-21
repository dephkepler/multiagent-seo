'use client'

import { Card } from '@/components/ui/card'
import type { Consultation } from '@/lib/api-types'
import { STATUS_LABELS, formatDateTime } from '@/lib/format'

export function Appointments({
  consultations,
  notificationsOn,
}: {
  consultations: Consultation[]
  notificationsOn: boolean
}) {
  // Only what is still ahead: a client opening this wants to know when to come,
  // and their history is the firm's record, not their to-do list.
  const upcoming = consultations
    .filter((c) => c.status === 'requested' || c.status === 'scheduled')
    .sort((a, b) => a.scheduled_at.localeCompare(b.scheduled_at))

  if (upcoming.length === 0) return null

  return (
    <Card className='flex flex-col gap-3'>
      <h2 className='text-h2 font-semibold text-ink'>Ваші записи</h2>
      <ul className='flex flex-col gap-2'>
        {upcoming.map((c) => (
          <li key={c.id} className='flex flex-col gap-0.5 rounded-control bg-surface-2 px-3 py-2.5'>
            <span className='text-body text-ink'>{formatDateTime(c.scheduled_at)}</span>
            <span className={c.status === 'scheduled' ? 'text-small text-good' : 'text-small text-warn'}>
              {STATUS_LABELS[c.status] ?? c.status}
            </span>
          </li>
        ))}
      </ul>
      {!notificationsOn && (
        <p className='text-small text-ink-3'>
          Нагадування поки не надходитимуть — напишіть боту будь-що, щоб він міг надіслати їх Вам.
        </p>
      )}
    </Card>
  )
}
