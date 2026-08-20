import { cx } from '@/lib/cx'

type Variant = 'neutral' | 'success' | 'warning' | 'danger' | 'info'

interface Props extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: Variant
}

const variants: Record<Variant, string> = {
  neutral: 'bg-gray-100 text-gray-600',
  success: 'bg-emerald-100 text-emerald-800',
  warning: 'bg-amber-100 text-amber-800',
  danger: 'bg-rose-100 text-rose-700',
  info: 'bg-sky-100 text-sky-800',
}

export function Badge({ variant = 'neutral', className, ...rest }: Props) {
  return (
    <span
      className={cx(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap',
        variants[variant],
        className
      )}
      {...rest}
    />
  )
}
