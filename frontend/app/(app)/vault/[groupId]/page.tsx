'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { SectionHeader } from '@/components/ui/section-header'

interface VaultGroup {
  id: string
  name: string
  entryCount: number
  createdAt: string
}

interface VaultEntry {
  id: string
  groupId: string
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

export default function VaultGroupPage() {
  const params = useParams<{ groupId: string }>()
  const groupId = params.groupId
  const qc = useQueryClient()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [addOpen, setAddOpen] = useState(false)

  // Same query key/fetch as the main groups page — react-query dedupes this
  // against that page's identical call instead of firing a second request.
  const groups = useQuery({
    queryKey: ['vault-groups'],
    queryFn: () => api<VaultGroup[]>('/vault-groups'),
  })
  const group = groups.data?.find((g) => g.id === groupId)

  const entries = useQuery({
    queryKey: ['vault-entries', groupId],
    queryFn: () => api<VaultEntry[]>(`/vault-entries?groupId=${groupId}`),
  })

  const create = useMutation({
    mutationFn: (body: EntryForm) =>
      api<VaultEntry>('/vault-entries', { method: 'POST', body: JSON.stringify({ ...body, groupId }) }),
    onSuccess: () => {
      toast.success('Пароль добавлен')
      qc.invalidateQueries({ queryKey: ['vault-entries', groupId] })
      qc.invalidateQueries({ queryKey: ['vault-groups'] })
      setAddOpen(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: EntryForm }) =>
      api<VaultEntry>(`/vault-entries/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Пароль обновлён')
      qc.invalidateQueries({ queryKey: ['vault-entries', groupId] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => api(`/vault-entries/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Пароль удалён')
      qc.invalidateQueries({ queryKey: ['vault-entries', groupId] })
      qc.invalidateQueries({ queryKey: ['vault-groups'] })
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
    if (await copyText(value)) {
      toast.success(`${label} скопирован`)
      return
    }
    toast.error(`${label} не скопирован — раскройте поле и выделите вручную`)
  }

  return (
    <div className='space-y-6'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex min-w-0 items-center gap-3'>
          <Link href='/vault' className='text-sm text-gray-500 hover:text-gray-700'>
            ← Все группы
          </Link>
          <h1 className='truncate text-base font-semibold text-gray-900'>{group ? group.name : 'Группа'}</h1>
        </div>
        <Button size='sm' onClick={() => setAddOpen(true)}>
          + Добавить
        </Button>
      </div>

      <Card>
        <div className='mb-4 flex flex-wrap items-start gap-2'>
          <Badge variant='warning'>Обычный текст</Badge>
          <p className='text-xs text-gray-400'>
            Хранится в виде обычного текста — это внутренний инструмент, а не настоящее хранилище. Не сохраняйте здесь
            то, что не положили бы в общий документ.
          </p>
        </div>

        {entries.isError && (
          <div className='rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700'>Не удалось загрузить</div>
        )}

        {!entries.isError && (entries.data || []).length === 0 && (
          <div className='py-6 text-center text-sm text-gray-400'>
            {entries.isLoading ? 'Загрузка…' : 'В этой группе пока нет паролей — добавьте первый'}
          </div>
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
                    label='Логин'
                    value={entry.username || '—'}
                    onCopy={entry.username ? () => copy(entry.username, 'Логин') : undefined}
                  />
                  {entry.password && (
                    <Field
                      label='Пароль'
                      value={revealed.has(entry.id) ? entry.password : '•'.repeat(Math.max(entry.password.length, 8))}
                      onCopy={() => copy(entry.password, 'Пароль')}
                      onToggle={() => toggleReveal(entry.id)}
                      revealed={revealed.has(entry.id)}
                    />
                  )}
                </div>

                {entry.notes && <p className='break-words whitespace-pre-wrap text-xs text-gray-500'>{entry.notes}</p>}

                <div className='flex items-center justify-between border-t border-gray-100 pt-3'>
                  <Button variant='ghost' size='sm' onClick={() => setEditingId(entry.id)}>
                    Изменить
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-rose-600 hover:bg-rose-50'
                    onClick={() => {
                      if (confirm(`Удалить «${entry.title}»? Это действие нельзя отменить.`)) del.mutate(entry.id)
                    }}
                  >
                    Удалить
                  </Button>
                </div>
              </Card>
            )
          )}
        </div>
      </Card>

      {addOpen && (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4' onClick={() => setAddOpen(false)}>
          <div
            role='dialog'
            aria-modal='true'
            className='max-h-[85vh] w-full max-w-md overflow-auto rounded-lg bg-white p-6 shadow-xl'
            onClick={(e) => e.stopPropagation()}
          >
            <SectionHeader
              title='Новый пароль'
              action={
                <button
                  onClick={() => setAddOpen(false)}
                  aria-label='Закрыть'
                  className='rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700'
                >
                  ✕
                </button>
              }
            />
            <EntryFormFields onSubmit={(body) => create.mutate(body)} busy={create.isPending} onCancel={() => setAddOpen(false)} />
          </div>
        </div>
      )}
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
            {revealed ? 'Скрыть' : 'Показать'}
          </button>
        )}
        {onCopy && (
          <button
            type='button'
            onClick={onCopy}
            className='-my-1.5 rounded px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-800'
          >
            Копировать
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
        <Label>Название</Label>
        <Input value={form.title} onChange={(e) => set('title', e.target.value)} placeholder='Gmail — компания' required />
      </div>
      <div>
        <Label>Ссылка</Label>
        <Input value={form.url} onChange={(e) => set('url', e.target.value)} placeholder='https://…' />
      </div>
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <div>
          <Label>Логин</Label>
          <Input value={form.username} onChange={(e) => set('username', e.target.value)} />
        </div>
        <div>
          <Label>Пароль</Label>
          <Input value={form.password} onChange={(e) => set('password', e.target.value)} className='font-mono' />
        </div>
      </div>
      <div>
        <Label>Заметки</Label>
        <Input value={form.notes} onChange={(e) => set('notes', e.target.value)} placeholder='опционально' />
      </div>
      <div className='flex gap-2'>
        <Button type='submit' size='sm' disabled={busy}>
          {busy ? 'Сохранение…' : initial ? 'Сохранить' : 'Добавить пароль'}
        </Button>
        {onCancel && (
          <Button type='button' variant='secondary' size='sm' onClick={onCancel}>
            Отмена
          </Button>
        )}
      </div>
    </form>
  )
}
