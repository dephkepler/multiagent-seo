'use client'

import { useId, type ReactNode } from 'react'
import { cx } from '@/lib/cx'

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string
  hint?: string
  error?: string
  children: (props: { id: string; describedBy?: string }) => ReactNode
}) {
  const id = useId()
  const hintId = `${id}-hint`
  const errorId = `${id}-error`
  // Whichever message is showing is the one announced, and the error wins —
  // otherwise a screen reader reads the hint and never the reason it failed.
  const describedBy = error ? errorId : hint ? hintId : undefined

  return (
    <div className='flex flex-col gap-1.5'>
      <label htmlFor={id} className='text-small font-medium text-ink-2'>
        {label}
      </label>
      {children({ id, describedBy })}
      {error ? (
        <p id={errorId} className='text-small text-bad' role='alert'>
          {error}
        </p>
      ) : (
        hint && (
          <p id={hintId} className='text-small text-ink-3'>
            {hint}
          </p>
        )
      )}
    </div>
  )
}

const control =
  'w-full rounded-control border border-line bg-surface-2 px-3 py-2.5 text-ink placeholder:text-ink-3 ' +
  'focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/30'

export function TextInput({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={cx(control, className)} />
}

export function TextArea({ className, ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className={cx(control, 'min-h-24 resize-y', className)} />
}
