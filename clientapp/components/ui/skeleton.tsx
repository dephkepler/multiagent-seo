import { cx } from '@/lib/cx'

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx('animate-pulse rounded-card bg-surface-2', className)} />
}
