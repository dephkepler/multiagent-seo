'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { SectionHeader } from '@/components/ui/section-header'

interface VaultEntry {
  id: string
  title: string
  url: string
  username: string
  password: string
  notes: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

interface EntryForm {
  title: string
  url: string
  username: string
  password: string
  notes: string
}

const emptyForm: EntryForm = { title: '', url: '', username: '', password: '', notes: '' }

export default function VaultPage() {
  const qc = useQueryClient()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<Set<string>>(new Set())

  const entries = useQuery({
    queryKey: ['vault-entries'],
    queryFn: () => api<VaultEntry[]>('/vault-entries'),
  })

  const create = useMutation({
    mutationFn: (body: EntryForm) => api<VaultEntry>('/vault-entries', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Entry added')
      qc.invalidateQueries({ queryKey: ['vault-entries'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: EntryForm }) =>
      api<VaultEntry>(`/vault-entries/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Entry updated')
      qc.invalidateQueries({ queryKey: ['vault-entries'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => api(`/vault-entries/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Entry removed')
      qc.invalidateQueries({ queryKey: ['vault-entries'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function toggleReveal(id: string) {
    setRevealed((r) => {
      const next = new Set(r)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${label} copied`)
    } catch {
      toast.error('Clipboard unavailable')
    }
  }

  return (
    <div className='space-y-6'>
      <Card>
        <SectionHeader title='Add password' as='h1' />
        <div className='mb-4 flex flex-wrap items-start gap-2'>
          <Badge variant='warning'>Plain text</Badge>
          <p className='text-xs text-gray-400'>
            Stored as plain text — this is an internal tool, not a real vault yet. Don&apos;t put anything here you
            wouldn&apos;t put in a shared doc.
          </p>
        </div>
        <EntryFormFields onSubmit={(body) => create.mutate(body)} busy={create.isPending} />
      </Card>

      <div>
        <SectionHeader
          title='Saved passwords'
          action={
            <Button variant='secondary' size='sm' onClick={() => entries.refetch()} disabled={entries.isFetching}>
              {entries.isFetching ? 'Refreshing…' : 'Refresh'}
            </Button>
          }
        />

        {entries.isError && (
          <Card className='border-red-200 bg-red-50 text-sm text-red-700'>
            Failed to load passwords{entries.error instanceof Error ? `: ${entries.error.message}` : ''}.
          </Card>
        )}

        {!entries.isError && (entries.data || []).length === 0 && (
          <Card className='text-center text-sm text-gray-400'>
            {entries.isLoading ? 'Loading…' : 'No passwords saved yet — add one above'}
          </Card>
        )}

        <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
          {(entries.data || []).map((entry) =>
            editingId === entry.id ? (
              <Card key={entry.id}>
                <EntryFormFields
                  initial={entry}
                  busy={update.isPending}
                  onSubmit={(body) => {
                    update.mutate({ id: entry.id, body })
                    setEditingId(null)
                  }}
                  onCancel={() => setEditingId(null)}
                />
              </Card>
            ) : (
              <Card key={entry.id} className='space-y-3'>
                <div className='min-w-0'>
                  <h3 className='truncate font-medium text-gray-900'>{entry.title}</h3>
                  {entry.url && (
                    <a
                      className='block truncate text-xs text-emerald-700 hover:underline'
                      href={entry.url}
                      target='_blank'
                      rel='noreferrer'
                    >
                      {entry.url}
                    </a>
                  )}
                </div>

                <div className='space-y-2'>
                  <Field
                    label='Login'
                    value={entry.username || '—'}
                    onCopy={entry.username ? () => copy(entry.username, 'Login') : undefined}
                  />
                  {entry.password && (
                    <Field
                      label='Password'
                      value={revealed.has(entry.id) ? entry.password : '•'.repeat(Math.max(entry.password.length, 8))}
                      onCopy={() => copy(entry.password, 'Password')}
                      onToggle={() => toggleReveal(entry.id)}
                      revealed={revealed.has(entry.id)}
                    />
                  )}
                </div>

                {entry.notes && <p className='break-words whitespace-pre-wrap text-xs text-gray-500'>{entry.notes}</p>}

                <div className='flex items-center justify-between border-t border-gray-100 pt-3'>
                  <Button variant='ghost' size='sm' onClick={() => setEditingId(entry.id)}>
                    Edit
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-rose-600 hover:bg-rose-50'
                    onClick={() => {
                      if (confirm(`Delete "${entry.title}"? This can't be undone.`)) del.mutate(entry.id)
                    }}
                  >
                    Delete
                  </Button>
                </div>
              </Card>
            )
          )}
        </div>
      </div>
    </div>
  )
}

function Field({
  label,
  value,
  onCopy,
  onToggle,
  revealed,
}: {
  label: string
  value: string
  onCopy?: () => void
  onToggle?: () => void
  revealed?: boolean
}) {
  return (
    <div className='flex items-center justify-between gap-2'>
      <div className='min-w-0'>
        <div className='text-[10px] uppercase tracking-wide text-gray-400'>{label}</div>
        <div className='truncate font-mono text-sm text-gray-800'>{value}</div>
      </div>
      <div className='flex shrink-0 gap-1'>
        {onToggle && (
          <button
            type='button'
            onClick={onToggle}
            className='-my-1.5 rounded px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-800'
          >
            {revealed ? 'Hide' : 'Show'}
          </button>
        )}
        {onCopy && (
          <button
            type='button'
            onClick={onCopy}
            className='-my-1.5 rounded px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-800'
          >
            Copy
          </button>
        )}
      </div>
    </div>
  )
}

function EntryFormFields({
  initial,
  onSubmit,
  onCancel,
  busy,
}: {
  initial?: VaultEntry
  onSubmit: (body: EntryForm) => void
  onCancel?: () => void
  busy: boolean
}) {
  const [form, setForm] = useState<EntryForm>(
    initial
      ? { title: initial.title, url: initial.url, username: initial.username, password: initial.password, notes: initial.notes }
      : emptyForm
  )
  function set<K extends keyof EntryForm>(k: K, v: EntryForm[K]) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(form)
        if (!initial) setForm(emptyForm)
      }}
      className='space-y-3'
    >
      <div>
        <Label>Title</Label>
        <Input value={form.title} onChange={(e) => set('title', e.target.value)} placeholder='Gmail — company' required />
      </div>
      <div>
        <Label>URL</Label>
        <Input value={form.url} onChange={(e) => set('url', e.target.value)} placeholder='https://…' />
      </div>
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <div>
          <Label>Login</Label>
          <Input value={form.username} onChange={(e) => set('username', e.target.value)} />
        </div>
        <div>
          <Label>Password</Label>
          <Input value={form.password} onChange={(e) => set('password', e.target.value)} className='font-mono' />
        </div>
      </div>
      <div>
        <Label>Notes</Label>
        <Input value={form.notes} onChange={(e) => set('notes', e.target.value)} placeholder='optional' />
      </div>
      <div className='flex gap-2'>
        <Button type='submit' size='sm' disabled={busy}>
          {busy ? 'Saving…' : initial ? 'Save' : 'Add password'}
        </Button>
        {onCancel && (
          <Button type='button' variant='secondary' size='sm' onClick={onCancel}>
            Cancel
          </Button>
        )}
      </div>
    </form>
  )
}
