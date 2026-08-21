import type { ReactNode } from 'react'
import { cx } from '@/lib/cx'

export function Card({
  children,
  className,
  role,
}: {
  children: ReactNode
  className?: string
  role?: string
}) {
  return (
    <section role={role} className={cx('rounded-card border border-line bg-surface p-4', className)}>
      {children}
    </section>
  )
}
