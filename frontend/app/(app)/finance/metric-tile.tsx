import { cx } from '@/lib/cx'

interface Props {
  label: string
  value: string
  hint?: string
  accent?: 'good' | 'bad'
}

export function MetricTile({ label, value, hint, accent }: Props) {
  return (
    <div className='rounded-lg border border-gray-200 bg-white p-3 sm:p-4'>
      <div className='text-[11px] tracking-wide text-gray-500 uppercase sm:text-xs'>{label}</div>
      <div
        className={cx(
          'mt-1 text-lg font-semibold tabular-nums sm:text-xl',
          accent === 'good' && 'text-emerald-700',
          accent === 'bad' && 'text-rose-700'
        )}
      >
        {value}
      </div>
      {hint && <div className='mt-0.5 text-[11px] text-gray-400'>{hint}</div>}
    </div>
  )
}
