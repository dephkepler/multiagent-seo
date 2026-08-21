import { Card } from './card'
import { cx } from '@/lib/cx'

export interface StatTileProps {
  label: string
  value: string
  hint?: string
  accent?: 'good' | 'bad'
  /** When true and accent is set, colors the whole tile (border+bg), not just the value — for a number that itself crossed a threshold worth flagging. */
  emphasize?: boolean
}

export function StatTile({ label, value, hint, accent, emphasize }: StatTileProps) {
  return (
    <Card
      className={cx(
        'p-3 sm:p-4',
        emphasize && accent === 'good' && 'border-emerald-200 bg-emerald-50',
        emphasize && accent === 'bad' && 'border-rose-200 bg-rose-50'
      )}
    >
      <div className='text-xs text-gray-500'>{label}</div>
      <div className={cx('mt-1 text-2xl font-semibold tabular-nums', accent === 'good' && 'text-emerald-700', accent === 'bad' && 'text-rose-700')}>
        {value}
      </div>
      {hint && <div className='mt-1 text-xs text-gray-400'>{hint}</div>}
    </Card>
  )
}
