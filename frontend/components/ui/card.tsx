import { cx } from '@/lib/cx'

export function Card({ className, children }: { className?: string; children: React.ReactNode }) {
  return <div className={cx('rounded-lg border border-gray-200 bg-white p-6 shadow-sm', className)}>{children}</div>
}
