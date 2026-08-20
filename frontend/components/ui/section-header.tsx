import { cx } from '@/lib/cx'

export function SectionHeader({
  title,
  action,
  className,
  as: Heading = 'h2',
}: {
  title: React.ReactNode
  action?: React.ReactNode
  className?: string
  /** Heading level — pass 'h1' when this is the page's own title, not a sub-section. Defaults to 'h2'. */
  as?: 'h1' | 'h2' | 'h3'
}) {
  return (
    <div className={cx('mb-4 flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 pb-3', className)}>
      <Heading className='text-base font-semibold text-gray-900'>{title}</Heading>
      {action}
    </div>
  )
}
