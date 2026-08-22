'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
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

export default function VaultPage() {
  const qc = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [name, setName] = useState('')

  const groups = useQuery({
    queryKey: ['vault-groups'],
    queryFn: () => api<VaultGroup[]>('/vault-groups'),
  })

  const create = useMutation({
    mutationFn: (body: { name: string }) => api<VaultGroup>('/vault-groups', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Группа добавлена')
      qc.invalidateQueries({ queryKey: ['vault-groups'] })
      setAddOpen(false)
      setName('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => api(`/vault-groups/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['vault-groups'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='space-y-6'>
      <SectionHeader
        title='Хранилище паролей'
        as='h1'
        action={
          <Button size='sm' onClick={() => setAddOpen(true)}>
            + Добавить группу
          </Button>
        }
      />

      {groups.isError && (
        <Card className='border-red-200 bg-red-50 text-sm text-red-700'>Не удалось загрузить</Card>
      )}

      {!groups.isError && (groups.data || []).length === 0 && (
        <Card className='text-center text-sm text-gray-400'>
          {groups.isLoading ? 'Загрузка…' : 'Пока нет групп — добавьте первую'}
        </Card>
      )}

      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        {(groups.data || []).map((group) => (
          <Link key={group.id} href={`/vault/${group.id}`} className='block'>
            <Card className='space-y-3 transition hover:border-emerald-300 hover:shadow-md'>
              <div className='flex items-start justify-between gap-2'>
                <h3 className='truncate font-bold text-gray-900'>{group.name}</h3>
                <Button
                  variant='ghost'
                  size='sm'
                  className='shrink-0 text-rose-600 hover:bg-rose-50'
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    if (confirm(`Удалить группу «${group.name}»? Это действие нельзя отменить.`)) del.mutate(group.id)
                  }}
                >
                  Удалить
                </Button>
              </div>
              <dl className='flex items-center gap-1.5 text-xs'>
                <dt className='text-gray-400'>Паролей</dt>
                <dd className='text-gray-600'>{group.entryCount}</dd>
              </dl>
            </Card>
          </Link>
        ))}
      </div>

      {addOpen && (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4' onClick={() => setAddOpen(false)}>
          <div
            role='dialog'
            aria-modal='true'
            className='w-full max-w-md rounded-lg bg-white p-6 shadow-xl'
            onClick={(e) => e.stopPropagation()}
          >
            <SectionHeader
              title='Новая группа'
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
            <form
              onSubmit={(e) => {
                e.preventDefault()
                if (name.trim()) create.mutate({ name: name.trim() })
              }}
              className='space-y-3'
            >
              <div>
                <Label>Название</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder='Соцсети' required />
              </div>
              <div className='flex gap-2'>
                <Button type='submit' size='sm' disabled={create.isPending}>
                  {create.isPending ? 'Создание…' : 'Создать'}
                </Button>
                <Button type='button' variant='secondary' size='sm' onClick={() => setAddOpen(false)}>
                  Отмена
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
