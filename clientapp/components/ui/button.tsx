'use client'

import { cx } from '@/lib/cx'

export function Button({
  className,
  variant = 'primary',
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'quiet' }) {
  return (
    <button
      {...props}
      className={cx(
        'rounded-control px-4 py-3 text-body font-medium disabled:opacity-50',
        variant === 'primary' ? 'bg-accent text-white' : 'text-ink-2 hover:bg-surface-2',
        className
      )}
    />
  )
}

export function Chip({
  selected,
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { selected: boolean }) {
  return (
    <button
      type='button'
      aria-pressed={selected}
      {...props}
      className={cx(
        'rounded-control border px-3 py-2 text-small',
        selected ? 'border-accent bg-accent-soft font-medium text-ink' : 'border-line bg-surface text-ink-2',
        className
      )}
    />
  )
}
