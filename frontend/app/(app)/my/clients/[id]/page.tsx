'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api, ApiError } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Section } from '@/components/ui/section'
import { StatTile } from '@/components/ui/stat-tile'
import { money } from '@/lib/format'
import { CasesTable } from '../../cases-table'
import { CONSULT_STATUS_LABEL, type MyCase, type MyClientCard } from '../../types'

export default function MyClientPage() {
  const params = useParams<{ id: string }>()
  const clientID = params.id
  const qc = useQueryClient()
  const [note, setNote] = useState('')
  const [busyCaseID, setBusyCaseID] = useState<string | null>(null)

  const card = useQuery({
    queryKey: ['my-client', clientID],
    queryFn: () => api<MyClientCard>(`/my/clients/${clientID}`),
    retry: false,
  })

  const addNote = useMutation({
    mutationFn: (text: string) => api(`/my/clients/${clientID}/notes`, { method: 'POST', body: JSON.stringify({ text }) }),
    onSuccess: () => {
      setNote('')
      toast.success('Заметка добавлена')
      qc.invalidateQueries({ queryKey: ['my-client', clientID] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const setStatus = useMutation({
    mutationFn: (vars: { id: string; status: string }) =>
      api(`/my/cases/${vars.id}/status`, { method: 'PATCH', body: JSON.stringify({ status: vars.status }) }),
    onSuccess: () => {
      toast.success('Статус дела обновлён')
      qc.invalidateQueries({ queryKey: ['my-client', clientID] })
      qc.invalidateQueries({ queryKey: ['my-cases'] })
      qc.invalidateQueries({ queryKey: ['my-stats'] })
    },
    onError: (e: Error) => toast.error(e.message),
    onSettled: () => setBusyCaseID(null),
  })

  function changeStatus(c: MyCase, status: string) {
    setBusyCaseID(c.id)
    setStatus.mutate({ id: c.id, status })
  }

  if (card.isLoading) return <div className='text-sm text-gray-500'>Загрузка…</div>

  // 404 is also the answer for a client that exists but belongs to a colleague —
  // by design, so an id cannot be probed for existence.
  if (card.isError) {
    const notMine = card.error instanceof ApiError && card.error.status === 404
    return (
      <div className='space-y-3'>
        <Link href='/my' className='text-sm text-emerald-700 hover:underline'>
          ← Мой кабинет
        </Link>
        <Card>
          <div className='text-sm text-gray-700'>
            {notMine ? 'Такого клиента у тебя нет — он появится, когда на тебя оформят дело с ним.' : 'Не удалось загрузить карточку клиента.'}
          </div>
        </Card>
      </div>
    )
  }

  const data = card.data!

  return (
    <div className='space-y-4'>
      <Link href='/my' className='text-sm text-emerald-700 hover:underline'>
        ← Мой кабинет
      </Link>

      <div>
        <h1 className='text-xl font-semibold tracking-tight text-gray-900'>{data.client.name || 'Без имени'}</h1>
        <p className='mt-1 text-sm text-gray-500'>{data.client.phone || 'телефон не указан'}</p>
      </div>

      <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
        <StatTile label='Моих дел' value={String(data.client.cases)} />
        <StatTile label='Гонорары' value={money(data.client.fee)} />
        <StatTile label='Оплачено' value={money(data.client.paid)} />
        <StatTile label='Должен' value={money(data.client.owed)} accent={data.client.owed > 0 ? 'bad' : undefined} />
      </div>

      <Section title='Дела' summary={<span className='text-sm text-gray-500'>{data.cases.length}</span>}>
        <CasesTable cases={data.cases} busyCaseID={busyCaseID} onSetStatus={changeStatus} showClient={false} />
      </Section>

      <Section title='Консультации' summary={<span className='text-sm text-gray-500'>{data.consultations.length}</span>} defaultOpen={false}>
        {data.consultations.length === 0 ? (
          <div className='text-sm text-gray-500'>Консультаций не было.</div>
        ) : (
          <div className='-mx-6 overflow-x-auto sm:mx-0'>
            <table className='w-full min-w-max text-sm'>
              <thead>
                <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
                  <th className='py-2 pr-3 font-medium'>Дата</th>
                  <th className='py-2 pr-3 font-medium'>Статус</th>
                  <th className='py-2 pr-3 text-right font-medium'>Цена</th>
                  <th className='py-2 font-medium'>Тема</th>
                </tr>
              </thead>
              <tbody>
                {data.consultations.map((c) => (
                  <tr key={c.id} className='border-b border-gray-100 transition-colors hover:bg-emerald-50'>
                    <td className='py-2 pr-3 whitespace-nowrap'>{c.scheduled_at.slice(0, 10)}</td>
                    <td className='py-2 pr-3'>
                      <Badge variant={c.status === 'completed' ? 'success' : c.status === 'cancelled' ? 'neutral' : 'info'}>
                        {CONSULT_STATUS_LABEL[c.status] ?? c.status}
                      </Badge>
                    </td>
                    <td className='py-2 pr-3 text-right tabular-nums'>{c.price > 0 ? money(c.price) : '—'}</td>
                    <td className='py-2 text-gray-600'>{c.case_note || <span className='text-gray-400'>—</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section title='Заметки' summary={<span className='text-sm text-gray-500'>{data.notes.length}</span>}>
        <form
          className='flex flex-col gap-2 sm:flex-row'
          onSubmit={(e) => {
            e.preventDefault()
            const text = note.trim()
            if (!text) return
            addNote.mutate(text)
          }}
        >
          <textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
            placeholder='Что обсудили, о чём договорились…'
            className='w-full rounded-md border border-gray-200 bg-white px-3 py-2 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100'
          />
          <Button type='submit' disabled={addNote.isPending || !note.trim()} className='sm:self-end'>
            {addNote.isPending ? 'Сохраняю…' : 'Добавить'}
          </Button>
        </form>

        {data.notes.length === 0 ? (
          <div className='mt-4 text-sm text-gray-500'>Заметок пока нет.</div>
        ) : (
          <ul className='mt-4 space-y-2'>
            {data.notes.map((n) => (
              <li key={n.id} className='rounded-md border border-gray-200 bg-white p-3 text-sm'>
                <div className='whitespace-pre-wrap text-gray-800'>{n.text}</div>
                <div className='mt-1 text-xs text-gray-400'>{n.created_at.slice(0, 16).replace('T', ' ')}</div>
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
  )
}
