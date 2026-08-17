'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { cx } from '@/lib/cx'

interface ClientDetailInfo {
  id: string
  name: string
  phone: string
  first_seen_at: string
  last_seen_at: string
}
interface ClientLead {
  id: string
  received_at: string
  message: string
  page: string
}
type ConsultationStatus = 'scheduled' | 'completed' | 'cancelled' | 'no_show'
interface ClientConsultation {
  id: string
  scheduled_at: string
  price: number
  status: ConsultationStatus
  case_note: string
}
type CaseStatus = 'in_progress' | 'completed' | 'cancelled'
interface ClientCase {
  id: string
  description: string
  category: string
  status: CaseStatus
  fee: number
  paid: number
  created_at: string
}
interface ClientNote {
  id: string
  text: string
  created_by: string
  created_at: string
}
interface ClientDetail {
  client: ClientDetailInfo
  revenue_total: number
  leads: ClientLead[]
  consultations: ClientConsultation[]
  cases: ClientCase[]
  notes: ClientNote[]
}

const CONSULT_STATUS_LABEL: Record<ConsultationStatus, string> = {
  scheduled: 'Запланирована',
  completed: 'Провёл',
  cancelled: 'Отменил',
  no_show: 'Не пришёл',
}
const CASE_STATUS_LABEL: Record<CaseStatus, string> = {
  in_progress: 'В работе',
  completed: 'Завершено',
  cancelled: 'Отменено',
}
const CASE_STATUS_COLOR: Record<CaseStatus, string> = {
  in_progress: 'bg-sky-100 text-sky-800',
  completed: 'bg-emerald-100 text-emerald-800',
  cancelled: 'bg-gray-100 text-gray-600',
}

function fmtMoney(n: number): string {
  return Math.round(n).toLocaleString('ru-RU') + ' ₴'
}
function fmtDateTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}
function fmtDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString('ru-RU')
}

export default function ClientDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const qc = useQueryClient()

  const detail = useQuery({
    queryKey: ['client-detail', id],
    queryFn: () => api<ClientDetail>(`/clients/${id}`),
  })

  // Editable draft fields, seeded from the loaded client — re-seeded
  // whenever a fresh fetch lands (loadedFor tracks which one we've already
  // synced from) without an Effect, so typing doesn't fight a re-render.
  const [loadedFor, setLoadedFor] = useState<ClientDetail | undefined>(undefined)
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  if (detail.data && detail.data !== loadedFor) {
    setLoadedFor(detail.data)
    setName(detail.data.client.name)
    setPhone(detail.data.client.phone)
  }

  const saveClient = useMutation({
    mutationFn: () => api(`/clients/${id}`, { method: 'PATCH', body: JSON.stringify({ name, phone }) }),
    onSuccess: () => {
      toast.success('Сохранено')
      qc.invalidateQueries({ queryKey: ['client-detail', id] })
      qc.invalidateQueries({ queryKey: ['client-segments'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const [noteText, setNoteText] = useState('')
  const addNote = useMutation({
    mutationFn: () => api(`/clients/${id}/notes`, { method: 'POST', body: JSON.stringify({ text: noteText }) }),
    onSuccess: () => {
      setNoteText('')
      qc.invalidateQueries({ queryKey: ['client-detail', id] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (detail.isLoading) return <Card>Загрузка…</Card>
  if (detail.isError || !detail.data) return <Card>Не удалось загрузить карточку клиента.</Card>

  const d = detail.data
  const dirty = name !== d.client.name || phone !== d.client.phone

  return (
    <div className='space-y-6'>
      <Link href='/clients' className='text-sm text-gray-500 hover:text-gray-700'>
        ← Все клиенты
      </Link>

      <Card>
        <div className='grid grid-cols-1 gap-4 md:grid-cols-[1fr_1fr_auto]'>
          <div>
            <Label>Имя</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder='Имя Фамилия' />
          </div>
          <div>
            <Label>Телефон</Label>
            <Input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder='+380...' />
          </div>
          <div className='flex items-end'>
            <Button disabled={!dirty || saveClient.isPending} onClick={() => saveClient.mutate()}>
              {saveClient.isPending ? 'Сохранение…' : 'Сохранить'}
            </Button>
          </div>
        </div>
        <div className='mt-4 flex flex-wrap gap-x-8 gap-y-1 text-xs text-gray-500'>
          <span>Первое обращение: {fmtDate(d.client.first_seen_at)}</span>
          <span>Последняя активность: {fmtDate(d.client.last_seen_at)}</span>
          <span className='font-medium text-emerald-700'>Принесено денег: {fmtMoney(d.revenue_total)}</span>
        </div>
      </Card>

      <Card>
        <h2 className='mb-3 text-sm font-semibold text-gray-700'>Дела ({d.cases.length})</h2>
        {d.cases.length === 0 ? (
          <p className='text-sm text-gray-400'>Дел пока не было.</p>
        ) : (
          <div className='overflow-x-auto'>
            <table className='w-full text-sm'>
              <thead className='text-left text-xs uppercase text-gray-500'>
                <tr>
                  <th className='py-2 pr-4'>Дата</th>
                  <th className='py-2 pr-4'>Категория</th>
                  <th className='py-2 pr-4'>Описание</th>
                  <th className='py-2 pr-4'>Статус</th>
                  <th className='py-2 pr-4'>Сумма</th>
                  <th className='py-2'>Оплачено</th>
                </tr>
              </thead>
              <tbody>
                {d.cases.map((c) => (
                  <tr key={c.id} className='border-t border-gray-100 align-top'>
                    <td className='py-2 pr-4 whitespace-nowrap text-gray-500'>{fmtDate(c.created_at)}</td>
                    <td className='py-2 pr-4 text-gray-500'>{c.category || '—'}</td>
                    <td className='max-w-xs py-2 pr-4'>{c.description || '—'}</td>
                    <td className='py-2 pr-4'>
                      <span className={cx('rounded px-2 py-0.5 text-xs font-medium', CASE_STATUS_COLOR[c.status])}>
                        {CASE_STATUS_LABEL[c.status] || c.status}
                      </span>
                    </td>
                    <td className='py-2 pr-4 whitespace-nowrap'>{fmtMoney(c.fee)}</td>
                    <td className={cx('py-2 whitespace-nowrap', c.paid < c.fee ? 'font-medium text-rose-600' : 'text-gray-500')}>
                      {fmtMoney(c.paid)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card>
        <h2 className='mb-3 text-sm font-semibold text-gray-700'>Консультации ({d.consultations.length})</h2>
        {d.consultations.length === 0 ? (
          <p className='text-sm text-gray-400'>Консультаций пока не было.</p>
        ) : (
          <div className='overflow-x-auto'>
            <table className='w-full text-sm'>
              <thead className='text-left text-xs uppercase text-gray-500'>
                <tr>
                  <th className='py-2 pr-4'>Дата</th>
                  <th className='py-2 pr-4'>Статус</th>
                  <th className='py-2 pr-4'>Сумма</th>
                  <th className='py-2'>Заметка</th>
                </tr>
              </thead>
              <tbody>
                {d.consultations.map((c) => (
                  <tr key={c.id} className='border-t border-gray-100 align-top'>
                    <td className='py-2 pr-4 whitespace-nowrap text-gray-500'>{fmtDateTime(c.scheduled_at)}</td>
                    <td className='py-2 pr-4 text-gray-500'>{CONSULT_STATUS_LABEL[c.status] || c.status}</td>
                    <td className='py-2 pr-4 whitespace-nowrap'>{fmtMoney(c.price)}</td>
                    <td className='py-2 text-gray-500'>{c.case_note || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card>
        <h2 className='mb-3 text-sm font-semibold text-gray-700'>Заявки ({d.leads.length})</h2>
        {d.leads.length === 0 ? (
          <p className='text-sm text-gray-400'>Заявок пока не было.</p>
        ) : (
          <div className='space-y-3'>
            {d.leads.map((l) => (
              <div key={l.id} className='rounded-md border border-gray-100 p-3'>
                <div className='mb-1 flex items-center justify-between text-xs text-gray-400'>
                  <span>{fmtDateTime(l.received_at)}</span>
                  <span>{l.page || '—'}</span>
                </div>
                <p className='text-sm whitespace-pre-wrap'>{l.message || '—'}</p>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <h2 className='mb-1 text-sm font-semibold text-gray-700'>Заметки ({d.notes.length})</h2>
        <p className='mb-3 text-xs text-gray-400'>
          Ручной журнал звонков/контактов — система не подключена ни к какой телефонии, это то, что вы сами сюда впишете.
        </p>
        <div className='mb-4 flex gap-2'>
          <Input
            value={noteText}
            onChange={(e) => setNoteText(e.target.value)}
            placeholder='Например: звонил, уточнил сроки подачи документов…'
            onKeyDown={(e) => {
              if (e.key === 'Enter' && noteText.trim()) addNote.mutate()
            }}
          />
          <Button disabled={!noteText.trim() || addNote.isPending} onClick={() => addNote.mutate()}>
            {addNote.isPending ? 'Добавление…' : 'Добавить'}
          </Button>
        </div>
        {d.notes.length === 0 ? (
          <p className='text-sm text-gray-400'>Заметок пока нет.</p>
        ) : (
          <div className='space-y-2'>
            {d.notes.map((n) => (
              <div key={n.id} className='rounded-md bg-gray-50 p-3 text-sm'>
                <p className='whitespace-pre-wrap'>{n.text}</p>
                <p className='mt-1 text-xs text-gray-400'>{fmtDateTime(n.created_at)}</p>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
