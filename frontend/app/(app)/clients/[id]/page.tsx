'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { cx } from '@/lib/cx'

type Gender = '' | 'male' | 'female'
type ClientType = 'individual' | 'legal_entity'
interface ClientDetailInfo {
  id: string
  name: string
  last_name: string
  first_name: string
  patronymic: string
  gender: Gender
  phone: string
  email: string
  client_type: ClientType
  company_name: string
  company_code: string
  address: string
  birthdate: string
  tax_id: string
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

type Segment = 'lead' | 'booked' | 'consulted' | 'client' | 'repeat' | 'lost'
interface ClientSegment {
  client_id: string
  segment: Segment
  overridden: boolean
  tags: string[]
  manual_tags: string[]
}
const SEGMENT_LABEL: Record<Segment, string> = {
  lead: 'Заявка',
  booked: 'Забронировал',
  consulted: 'Проконсультирован',
  client: 'Клиент',
  repeat: 'Повторный',
  lost: 'Потерян',
}
const SEGMENT_ORDER: Segment[] = ['lead', 'booked', 'consulted', 'client', 'repeat', 'lost']
const SEGMENT_COLOR: Record<Segment, string> = {
  lead: 'bg-gray-100 text-gray-700',
  booked: 'bg-sky-100 text-sky-800',
  consulted: 'bg-amber-100 text-amber-800',
  client: 'bg-emerald-100 text-emerald-800',
  repeat: 'bg-violet-100 text-violet-800',
  lost: 'bg-rose-100 text-rose-800',
}
const TAG_LABEL: Record<string, string> = {
  debtor: 'Должник',
  no_show_risk: 'Риск неявки',
  high_value: 'Ценный клиент',
  dormant: 'Без контакта 90+ дней',
}
const TAG_COLOR: Record<string, string> = {
  debtor: 'border border-rose-200 bg-rose-50 text-rose-700',
  no_show_risk: 'border border-orange-200 bg-orange-50 text-orange-700',
  high_value: 'border border-emerald-200 bg-emerald-50 text-emerald-700',
  dormant: 'border border-gray-200 bg-gray-50 text-gray-500',
}

const GENDER_LABEL: Record<Gender, string> = { '': 'Не вказано', male: 'Чоловіча', female: 'Жіноча' }
const CLIENT_TYPE_LABEL: Record<ClientType, string> = { individual: 'Фізична особа', legal_entity: 'Юридична особа' }

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
function daysSince(iso: string): number | null {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return Math.max(0, Math.floor((Date.now() - d.getTime()) / 86_400_000))
}

interface ClientForm {
  lastName: string
  firstName: string
  patronymic: string
  phone: string
  gender: Gender
  email: string
  clientType: ClientType
  companyName: string
  companyCode: string
  address: string
  birthdate: string
  taxId: string
}
// Splits a raw "Прізвище Ім'я По батькові" string (Ukrainian legal/business
// name order) into its three parts — mirrors consultations.SplitName on
// the backend, which now seeds last_name/first_name/patronymic this way
// for every *new* client created through the bot. This client-side twin
// only matters for clients created *before* that fix, whose structured
// fields are still blank with everything sitting in the single `name`
// field — falling back to dumping the whole string into first_name (the
// previous behavior here) looked exactly as broken as the bug this is
// fixing. A single bare word is read as a given name, not a surname —
// most self-booking clients type just that.
function splitName(raw: string): { lastName: string; firstName: string; patronymic: string } {
  const parts = raw.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return { lastName: '', firstName: '', patronymic: '' }
  if (parts.length === 1) return { lastName: '', firstName: parts[0], patronymic: '' }
  if (parts.length === 2) return { lastName: parts[0], firstName: parts[1], patronymic: '' }
  return { lastName: parts[0], firstName: parts[1], patronymic: parts.slice(2).join(' ') }
}

// Clients created through the bot/lead form only ever get the single
// combined `name` written (see webleads.ResolveClient) — the structured
// last_name/first_name/patronymic fields the card actually edits stay
// blank forever unless something seeds them. Falling back to name (split,
// same as the backend now does for new clients) means the name is at
// least visible instead of a blank field next to a client the CRM clearly
// already knows the name of — and saving the card (even for an unrelated
// field) now writes it into first_name/last_name for real, fixing the
// record going forward.
function toForm(c: ClientDetailInfo): ClientForm {
  const fallback = c.first_name || c.last_name ? null : splitName(c.name)
  return {
    lastName: c.last_name || fallback?.lastName || '',
    firstName: c.first_name || fallback?.firstName || '',
    patronymic: c.patronymic,
    phone: c.phone,
    gender: c.gender,
    email: c.email,
    clientType: c.client_type,
    companyName: c.company_name,
    companyCode: c.company_code,
    address: c.address,
    birthdate: c.birthdate,
    taxId: c.tax_id,
  }
}
function formsEqual(a: ClientForm, b: ClientForm): boolean {
  return (Object.keys(a) as (keyof ClientForm)[]).every((k) => a[k] === b[k])
}

function MetricTile({ label, value, accent }: { label: string; value: string; accent?: 'bad' | 'good' }) {
  return (
    <div className='rounded-md border border-gray-100 bg-gray-50/60 p-3'>
      <div className='text-xs text-gray-500'>{label}</div>
      <div className={cx('mt-0.5 text-lg font-semibold', accent === 'bad' ? 'text-rose-600' : accent === 'good' ? 'text-emerald-700' : 'text-gray-800')}>
        {value}
      </div>
    </div>
  )
}

export default function ClientDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const qc = useQueryClient()
  const router = useRouter()

  const detail = useQuery({
    queryKey: ['client-detail', id],
    queryFn: () => api<ClientDetail>(`/clients/${id}`),
  })
  // Segment/tags aren't part of the client card's own data — they're the
  // funnel classification (see clientsegments on the backend). GET /clients
  // is now paginated (see the list page), so this client might not even be
  // on whatever page the list happens to have cached — fetch it directly by
  // id instead of searching a list that may not contain it.
  const segments = useQuery({
    queryKey: ['client-segments', id],
    queryFn: () => api<{ items: ClientSegment[] }>(`/clients?id=${id}`),
  })
  const segment = segments.data?.items[0]

  const setOverride = useMutation({
    mutationFn: (value: Segment | null) =>
      api(`/clients/${id}/segment`, { method: 'PATCH', body: JSON.stringify({ segment_override: value }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  const addTag = useMutation({
    mutationFn: (tag: string) => api(`/clients/${id}/tags`, { method: 'POST', body: JSON.stringify({ tag }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  const removeTag = useMutation({
    mutationFn: (tag: string) => api(`/clients/${id}/tags/${encodeURIComponent(tag)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['client-segments'] }),
    onError: (e: Error) => toast.error(e.message),
  })
  // The manual-tag vocabulary — same source /clients uses for its "+ тег"
  // dropdown; managing the list itself (add/rename/delete) lives there, not
  // on this page.
  const tagDefs = useQuery({
    queryKey: ['client-tag-defs'],
    queryFn: () => api<{ items: { label: string }[] }>('/clients/tag-defs'),
  })
  const tagDefLabels = (tagDefs.data?.items ?? []).map((d) => d.label)

  // Editable draft, seeded from the loaded client — re-seeded whenever a
  // fresh fetch lands (loadedFor tracks which one we've already synced
  // from) without an Effect, so typing doesn't fight a re-render.
  const [loadedFor, setLoadedFor] = useState<ClientDetail | undefined>(undefined)
  const [form, setForm] = useState<ClientForm | null>(null)
  if (detail.data && detail.data !== loadedFor) {
    setLoadedFor(detail.data)
    setForm(toForm(detail.data.client))
  }
  function set<K extends keyof ClientForm>(k: K, v: ClientForm[K]) {
    setForm((f) => (f ? { ...f, [k]: v } : f))
  }

  const saveClient = useMutation({
    mutationFn: () => {
      if (!form) return Promise.resolve()
      return api(`/clients/${id}`, {
        method: 'PATCH',
        body: JSON.stringify({
          last_name: form.lastName,
          first_name: form.firstName,
          patronymic: form.patronymic,
          phone: form.phone,
          gender: form.gender,
          email: form.email,
          client_type: form.clientType,
          // Company fields only mean something for a legal entity — clear
          // them on save if the type was switched back, so a stale name
          // doesn't linger attached to an individual.
          company_name: form.clientType === 'legal_entity' ? form.companyName : '',
          company_code: form.clientType === 'legal_entity' ? form.companyCode : '',
          address: form.address,
          birthdate: form.birthdate,
          tax_id: form.taxId,
        }),
      })
    },
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

  // Backend refuses (409) a client with any lead/consultation/case — the
  // error message from that response is what actually reaches the toast,
  // so a client with real history can't be destroyed by this button.
  const deleteClient = useMutation({
    mutationFn: () => api(`/clients/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Клиент удалён')
      qc.invalidateQueries({ queryKey: ['client-segments'] })
      router.push('/clients')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (detail.isLoading) return <Card>Загрузка…</Card>
  if (detail.isError || !detail.data) return <Card>Не удалось загрузить карточку клиента.</Card>
  if (!form) return <Card>Загрузка…</Card>

  const d = detail.data
  const dirty = !formsEqual(form, toForm(d.client))

  const debtTotal = d.cases.reduce((sum, c) => sum + Math.max(c.fee - c.paid, 0), 0)
  const casesActive = d.cases.filter((c) => c.status === 'in_progress').length
  const consultsDone = d.consultations.filter((c) => c.status === 'completed').length
  const idle = daysSince(d.client.last_seen_at)

  return (
    <div className='space-y-6'>
      <div className='flex items-center justify-between'>
        <Link href='/clients' className='text-sm text-gray-500 hover:text-gray-700'>
          ← Все клиенты
        </Link>
        <div className='flex items-center gap-3'>
          <span className='font-mono text-xs text-gray-400 select-all'>Client ID: {d.client.id}</span>
          <button
            type='button'
            disabled={deleteClient.isPending}
            onClick={() => {
              if (window.confirm(`Удалить клиента «${d.client.name || d.client.id}»? Это необратимо.`)) deleteClient.mutate()
            }}
            className='text-xs text-rose-400 hover:text-rose-600 disabled:cursor-wait disabled:text-gray-300'
          >
            {deleteClient.isPending ? 'Удаление…' : 'Удалить клиента'}
          </button>
        </div>
      </div>

      <Card>
        <div className='mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6'>
          <div className='rounded-md border border-gray-100 bg-gray-50/60 p-3'>
            <div className='text-xs text-gray-500'>Сегмент</div>
            <div className='mt-1 flex items-center gap-1.5'>
              {segment ? (
                <select
                  value={segment.segment}
                  disabled={setOverride.isPending}
                  onChange={(e) => setOverride.mutate(e.target.value as Segment)}
                  className={cx('cursor-pointer rounded px-1.5 py-0.5 text-xs font-medium outline-none disabled:cursor-wait', SEGMENT_COLOR[segment.segment])}
                >
                  {SEGMENT_ORDER.map((s) => (
                    <option key={s} value={s}>
                      {SEGMENT_LABEL[s]}
                    </option>
                  ))}
                </select>
              ) : (
                <span className='text-sm text-gray-400'>—</span>
              )}
            </div>
          </div>
          <div className='rounded-md border border-gray-100 bg-gray-50/60 p-3'>
            <div className='mb-1 text-xs text-gray-500'>Теги</div>
            <TagsEditor
              tags={segment?.tags ?? []}
              manualTags={segment?.manual_tags ?? []}
              availableTags={tagDefLabels}
              pending={addTag.isPending || removeTag.isPending}
              onAdd={(tag) => addTag.mutate(tag)}
              onRemove={(tag) => removeTag.mutate(tag)}
            />
          </div>
          <MetricTile label='Принесено денег' value={fmtMoney(d.revenue_total)} accent='good' />
          <MetricTile label='Долг' value={debtTotal > 0 ? fmtMoney(debtTotal) : '—'} accent={debtTotal > 0 ? 'bad' : undefined} />
          <MetricTile label='Дел' value={`${d.cases.length}${casesActive ? ` (${casesActive} в работе)` : ''}`} />
          <MetricTile label='Консультаций' value={`${d.consultations.length}${consultsDone ? ` (${consultsDone} провёл)` : ''}`} />
        </div>
        <p className='mb-4 text-xs text-gray-400'>
          Первое обращение: {fmtDate(d.client.first_seen_at)} · Последняя активность: {fmtDate(d.client.last_seen_at)}
          {idle !== null && (idle === 0 ? ' (сегодня)' : ` (${idle} дн. назад)`)}
        </p>

        {/* Личные данные */}
        <div className='mb-2 text-xs font-semibold tracking-wide text-gray-400 uppercase'>Личные данные</div>
        <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
          <div>
            <Label>Фамилия</Label>
            <Input value={form.lastName} onChange={(e) => set('lastName', e.target.value)} placeholder='Фамілія' />
          </div>
          <div>
            <Label>Имя</Label>
            <Input value={form.firstName} onChange={(e) => set('firstName', e.target.value)} placeholder="Ім'я" />
          </div>
          <div>
            <Label>Отчество</Label>
            <Input value={form.patronymic} onChange={(e) => set('patronymic', e.target.value)} placeholder='По батькові' />
          </div>
          <div>
            <Label>Пол</Label>
            <Select value={form.gender} onChange={(e) => set('gender', e.target.value as Gender)}>
              {(Object.keys(GENDER_LABEL) as Gender[]).map((g) => (
                <option key={g} value={g}>
                  {GENDER_LABEL[g]}
                </option>
              ))}
            </Select>
          </div>
          <div>
            <Label>Телефон</Label>
            <Input value={form.phone} onChange={(e) => set('phone', e.target.value)} placeholder='+380...' />
          </div>
          <div>
            <Label>Email</Label>
            <Input type='email' value={form.email} onChange={(e) => set('email', e.target.value)} placeholder='client@example.com' />
          </div>
        </div>

        {/* Тип клиента */}
        <div className='mt-5 border-t border-gray-100 pt-4'>
          <div className='mb-2 text-xs font-semibold tracking-wide text-gray-400 uppercase'>Тип клиента</div>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <div>
              <Label>Физ./юр. лицо</Label>
              <Select value={form.clientType} onChange={(e) => set('clientType', e.target.value as ClientType)}>
                {(Object.keys(CLIENT_TYPE_LABEL) as ClientType[]).map((t) => (
                  <option key={t} value={t}>
                    {CLIENT_TYPE_LABEL[t]}
                  </option>
                ))}
              </Select>
            </div>
            {form.clientType === 'legal_entity' && (
              <>
                <div>
                  <Label>Название компании</Label>
                  <Input value={form.companyName} onChange={(e) => set('companyName', e.target.value)} placeholder='ТОВ «...»' />
                </div>
                <div>
                  <Label>ЄДРПОУ</Label>
                  <Input value={form.companyCode} onChange={(e) => set('companyCode', e.target.value)} placeholder='12345678' />
                </div>
              </>
            )}
          </div>
        </div>

        {/* Чувствительные данные — для документов, шифруются в базе */}
        <div className='mt-5 border-t border-gray-100 pt-4'>
          <div className='mb-2 flex items-center gap-1.5 text-xs font-semibold tracking-wide text-gray-400 uppercase'>
            <span>🔒</span> Чувствительные данные
          </div>
          <p className='mb-3 text-xs text-gray-400'>
            Для позовних заяв/клопотань — не для аналитики. Хранятся в базе зашифрованными, не в открытом виде.
          </p>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <div>
              <Label>Адреса реєстрації</Label>
              <Input value={form.address} onChange={(e) => set('address', e.target.value)} placeholder='м. Київ, вул. ...' />
            </div>
            <div>
              <Label>Дата народження</Label>
              <Input type='date' value={form.birthdate} onChange={(e) => set('birthdate', e.target.value)} />
            </div>
            <div>
              <Label>РНОКПП / ІПН</Label>
              <Input value={form.taxId} onChange={(e) => set('taxId', e.target.value)} placeholder='1234567890' />
            </div>
          </div>
        </div>

        <div className='mt-5 flex justify-end border-t border-gray-100 pt-4'>
          <Button disabled={!dirty || saveClient.isPending} onClick={() => saveClient.mutate()}>
            {saveClient.isPending ? 'Сохранение…' : 'Сохранить'}
          </Button>
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
                  <th className='py-2 pr-4'>ID</th>
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
                    <td className='py-2 pr-4 font-mono text-xs whitespace-nowrap text-gray-400 select-all'>{c.id}</td>
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
                  <th className='py-2 pr-4'>ID</th>
                  <th className='py-2 pr-4'>Дата</th>
                  <th className='py-2 pr-4'>Статус</th>
                  <th className='py-2 pr-4'>Сумма</th>
                  <th className='py-2'>Заметка</th>
                </tr>
              </thead>
              <tbody>
                {d.consultations.map((c) => (
                  <tr key={c.id} className='border-t border-gray-100 align-top'>
                    <td className='py-2 pr-4 font-mono text-xs whitespace-nowrap text-gray-400 select-all'>{c.id}</td>
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

// TagsEditor renders the four auto-computed tags read-only, every manual
// tag as a removable chip, and a dropdown to add one more — same curated
// vocabulary as the /clients list page (see there for the "Управление
// тегами" panel that actually manages the list).
function TagsEditor({
  tags,
  manualTags,
  availableTags,
  onAdd,
  onRemove,
  pending,
}: {
  tags: string[]
  manualTags: string[]
  availableTags: string[]
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
  pending: boolean
}) {
  const remaining = availableTags.filter((t) => !manualTags.includes(t))

  return (
    <div className='flex flex-wrap items-center gap-1'>
      {tags.length === 0 && manualTags.length === 0 && remaining.length === 0 && (
        <span className='text-sm text-gray-400'>—</span>
      )}
      {tags.map((t) => (
        <span key={t} className={cx('rounded px-1.5 py-0.5 text-[11px] font-medium', TAG_COLOR[t] || 'bg-gray-100 text-gray-600')}>
          {TAG_LABEL[t] || t}
        </span>
      ))}
      {manualTags.map((t) => (
        <span
          key={t}
          className='inline-flex items-center gap-1 rounded border border-dashed border-gray-300 bg-white px-1.5 py-0.5 text-[11px] font-medium text-gray-600'
        >
          {t}
          <button
            type='button'
            disabled={pending}
            onClick={() => onRemove(t)}
            aria-label={`Убрать тег ${t}`}
            className='leading-none text-gray-400 hover:text-rose-600 disabled:cursor-wait'
          >
            ×
          </button>
        </span>
      ))}
      {remaining.length > 0 && (
        <select
          value=''
          disabled={pending}
          onChange={(e) => {
            if (e.target.value) onAdd(e.target.value)
          }}
          className='rounded border border-dashed border-gray-300 bg-white px-1 py-0.5 text-[11px] text-gray-400 outline-none disabled:cursor-wait'
        >
          <option value=''>+ тег</option>
          {remaining.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      )}
    </div>
  )
}
