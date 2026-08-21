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
import { Badge } from '@/components/ui/badge'
import { SectionHeader } from '@/components/ui/section-header'
import { StatTile } from '@/components/ui/stat-tile'
import { cx } from '@/lib/cx'
import {
  categoryColorClass,
  SEGMENT_COLOR,
  SEGMENT_LABEL,
  SEGMENT_ORDER,
  type Segment,
  sortTagsByCategory,
  TAG_BADGE_VARIANT,
  TAG_LABEL,
} from '@/lib/client-tags'

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

interface ClientSegment {
  client_id: string
  segment: Segment
  overridden: boolean
  tags: string[]
  manual_tags: string[]
}

interface TagDef {
  label: string
  category: string
  created_at: string
}

const GENDER_LABEL: Record<Gender, string> = { '': 'Не вказано', male: 'Чоловіча', female: 'Жіноча' }
const CLIENT_TYPE_LABEL: Record<ClientType, string> = { individual: 'Фізична особа', legal_entity: 'Юридична особа' }

const CONSULT_STATUS_LABEL: Record<ConsultationStatus, string> = {
  scheduled: 'Запланирована',
  completed: 'Проведена',
  cancelled: 'Отменена',
  no_show: 'Клиент не пришёл',
}
const CASE_STATUS_LABEL: Record<CaseStatus, string> = {
  in_progress: 'В работе',
  completed: 'Завершено',
  cancelled: 'Отменено',
}
const CASE_STATUS_VARIANT: Record<CaseStatus, 'info' | 'success' | 'neutral'> = {
  in_progress: 'info',
  completed: 'success',
  cancelled: 'neutral',
}
const CONSULT_STATUS_VARIANT: Record<ConsultationStatus, 'info' | 'success' | 'neutral' | 'danger'> = {
  scheduled: 'info',
  completed: 'success',
  cancelled: 'neutral',
  no_show: 'danger',
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
  // dropdown; managing the list itself (add/rename/delete/categories) lives
  // there, not on this page.
  const tagDefs = useQuery({
    queryKey: ['client-tag-defs'],
    queryFn: () => api<{ items: TagDef[] }>('/clients/tag-defs'),
  })
  const defs = tagDefs.data?.items ?? []
  const categories = [...new Set(defs.map((d) => d.category))]
  const defsByCategory = new Map<string, TagDef[]>()
  for (const d of defs) {
    defsByCategory.set(d.category, [...(defsByCategory.get(d.category) ?? []), d])
  }

  // Editable draft, seeded from the loaded client — re-seeded whenever a
  // fresh fetch lands (loadedFor tracks which one we've already synced
  // from) without an Effect, so typing doesn't fight a re-render.
  const [loadedFor, setLoadedFor] = useState<ClientDetail | undefined>(undefined)
  const [form, setForm] = useState<ClientForm | null>(null)
  const dirtyNow = form != null && loadedFor != null && !formsEqual(form, toForm(loadedFor.client))
  if (detail.data && detail.data !== loadedFor && !dirtyNow) {
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
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex min-w-0 items-center gap-3'>
          <Link
            href='/clients'
            onClick={(e) => {
              if (dirty && !window.confirm('Есть несохранённые изменения в карточке клиента. Уйти без сохранения?')) {
                e.preventDefault()
              }
            }}
            className='text-sm text-gray-500 hover:text-gray-700'
          >
            ← Все клиенты
          </Link>
          <span className='truncate font-mono text-xs text-gray-400 select-all'>Client ID: {d.client.id}</span>
        </div>
        <button
          type='button'
          disabled={deleteClient.isPending}
          onClick={() => {
            if (window.confirm(`Удалить клиента «${d.client.name || d.client.id}»? Это необратимо.`)) deleteClient.mutate()
          }}
          className='shrink-0 rounded-md border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs font-medium text-rose-600 hover:border-rose-300 hover:bg-rose-100 disabled:cursor-wait disabled:opacity-50'
        >
          {deleteClient.isPending ? 'Удаление…' : 'Удалить клиента'}
        </button>
      </div>

      <Card>
        <SectionHeader title='Обзор' />
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6'>
          <Card className='col-span-2 p-3 sm:col-span-1 sm:p-4'>
            <div className='text-xs text-gray-500'>Сегмент</div>
            <div className='mt-1 flex items-center gap-1.5'>
              {segment ? (
                <>
                  <select
                    value={segment.segment}
                    disabled={setOverride.isPending}
                    onChange={(e) => setOverride.mutate(e.target.value === '' ? null : (e.target.value as Segment))}
                    className={cx(
                      'cursor-pointer rounded px-1.5 py-0.5 text-xs font-medium outline-none disabled:cursor-wait disabled:opacity-60',
                      SEGMENT_COLOR[segment.segment]
                    )}
                  >
                    {SEGMENT_ORDER.map((s) => (
                      <option key={s} value={s}>
                        {SEGMENT_LABEL[s]}
                      </option>
                    ))}
                    {segment.overridden && <option value=''>Авто (по правилам)</option>}
                  </select>
                  {setOverride.isPending && <span className='text-[10px] text-gray-400'>сохранение…</span>}
                </>
              ) : (
                <span className='text-sm text-gray-400'>—</span>
              )}
            </div>
          </Card>
          <Card className='col-span-2 p-3 sm:col-span-1 sm:p-4'>
            <div className='mb-1 flex items-center gap-1.5 text-xs text-gray-500'>
              Теги
              {(addTag.isPending || removeTag.isPending) && <span className='text-[10px] text-gray-400'>сохранение…</span>}
            </div>
            <TagsEditor
              tags={segment?.tags ?? []}
              manualTags={segment?.manual_tags ?? []}
              categories={categories}
              defsByCategory={defsByCategory}
              pending={addTag.isPending || removeTag.isPending}
              onAdd={(tag) => addTag.mutate(tag)}
              onRemove={(tag) => removeTag.mutate(tag)}
            />
          </Card>
          <StatTile label='Принесено денег' value={fmtMoney(d.revenue_total)} accent='good' />
          <StatTile label='Долг' value={debtTotal > 0 ? fmtMoney(debtTotal) : '—'} accent={debtTotal > 0 ? 'bad' : undefined} />
          <StatTile label='Дел' value={`${d.cases.length}${casesActive ? ` (${casesActive} в работе)` : ''}`} />
          <StatTile label='Консультаций' value={`${d.consultations.length}${consultsDone ? ` (${consultsDone} провёл)` : ''}`} />
        </div>
        <p className='mt-4 text-xs text-gray-400'>
          Первое обращение: {fmtDate(d.client.first_seen_at)} · Последняя активность: {fmtDate(d.client.last_seen_at)}
          {idle !== null && (idle === 0 ? ' (сегодня)' : ` (${idle} дн. назад)`)}
        </p>
      </Card>

      <Card>
        <SectionHeader
          title='Личные данные'
          action={dirty && <Badge variant='warning'>Есть несохранённые изменения</Badge>}
        />
        <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
          <div>
            <Label>Фамилия</Label>
            <Input value={form.lastName} onChange={(e) => set('lastName', e.target.value)} placeholder='Фамилия' />
          </div>
          <div>
            <Label>Имя</Label>
            <Input value={form.firstName} onChange={(e) => set('firstName', e.target.value)} placeholder='Имя' />
          </div>
          <div>
            <Label>Отчество</Label>
            <Input value={form.patronymic} onChange={(e) => set('patronymic', e.target.value)} placeholder='Отчество' />
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

        <div className='mt-6'>
          <SectionHeader title='Тип клиента' />
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
          </div>
          {form.clientType === 'legal_entity' && (
            <div className='mt-4 grid grid-cols-1 gap-4 md:grid-cols-3'>
              <div>
                <Label>Название компании</Label>
                <Input value={form.companyName} onChange={(e) => set('companyName', e.target.value)} placeholder='ТОВ «...»' />
              </div>
              <div>
                <Label>ЄДРПОУ</Label>
                <Input value={form.companyCode} onChange={(e) => set('companyCode', e.target.value)} placeholder='12345678' />
              </div>
            </div>
          )}
        </div>

        <div className='mt-6'>
          <SectionHeader
            title={
              <span className='flex items-center gap-1.5'>
                <span>🔒</span> Чувствительные данные
              </span>
            }
          />
          <p className='mb-3 text-xs text-gray-400'>
            Для исковых заявлений/ходатайств — не для аналитики. Хранятся в базе зашифрованными, не в открытом виде.
          </p>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <div>
              <Label>Адрес регистрации</Label>
              <Input value={form.address} onChange={(e) => set('address', e.target.value)} placeholder='г. Киев, ул. ...' />
            </div>
            <div>
              <Label>Дата рождения</Label>
              <Input type='date' value={form.birthdate} onChange={(e) => set('birthdate', e.target.value)} />
            </div>
            <div>
              <Label>РНОКПП / ІПН</Label>
              <Input value={form.taxId} onChange={(e) => set('taxId', e.target.value)} placeholder='1234567890' />
            </div>
          </div>
        </div>

        <div className='mt-6 flex justify-end border-t border-gray-100 pt-4'>
          <Button disabled={!dirty || saveClient.isPending} onClick={() => saveClient.mutate()}>
            {saveClient.isPending ? 'Сохранение…' : 'Сохранить'}
          </Button>
        </div>
      </Card>

      <Card>
        <SectionHeader title={`Дела (${d.cases.length})`} />
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
                      <Badge variant={CASE_STATUS_VARIANT[c.status]}>{CASE_STATUS_LABEL[c.status] || c.status}</Badge>
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
        <SectionHeader title={`Консультации (${d.consultations.length})`} />
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
                    <td className='py-2 pr-4'>
                      <Badge variant={CONSULT_STATUS_VARIANT[c.status]}>{CONSULT_STATUS_LABEL[c.status] || c.status}</Badge>
                    </td>
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
        <SectionHeader title={`Заявки (${d.leads.length})`} />
        {d.leads.length === 0 ? (
          <p className='text-sm text-gray-400'>Заявок пока не было.</p>
        ) : (
          <div className='space-y-3'>
            {d.leads.map((l) => (
              <div key={l.id} className='rounded-md border border-gray-100 p-3'>
                <div className='mb-1 flex items-center justify-between gap-2 text-xs text-gray-400'>
                  <span className='shrink-0'>{fmtDateTime(l.received_at)}</span>
                  <span className='min-w-0 truncate' title={l.page}>
                    {l.page || '—'}
                  </span>
                </div>
                <p className='text-sm whitespace-pre-wrap'>{l.message || '—'}</p>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <SectionHeader title={`Заметки (${d.notes.length})`} />
        <p className='mb-3 text-xs text-gray-400'>
          Ручной журнал звонков/контактов — система не подключена ни к какой телефонии, это то, что вы сами сюда впишете.
        </p>
        <div className='mb-4 flex gap-2'>
          <textarea
            value={noteText}
            onChange={(e) => setNoteText(e.target.value)}
            placeholder='Например: звонил, уточнил сроки подачи документов… (Ctrl/Cmd+Enter — добавить)'
            rows={2}
            className='w-full resize-y rounded-md border border-gray-200 bg-white px-3 py-2 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100 disabled:cursor-not-allowed disabled:opacity-50'
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && noteText.trim() && !addNote.isPending) addNote.mutate()
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

// Same VISIBLE_TAGS/overflow-toggle reasoning as the /clients list page's
// TagsCell: an unbounded tag list here would grow this tile taller than the
// metric tiles next to it in the overview grid.
const VISIBLE_TAGS = 4

// TagsEditor renders the auto-computed tags read-only, every manual tag as
// a chip colored by its category (grouped together via sortTagsByCategory,
// not the API's alphabetical order), capped to VISIBLE_TAGS with a "+N"
// expand toggle, and a compact dropdown (one <optgroup> per category) to
// add one more — same curated vocabulary and coloring as the /clients list
// page (see there for the "Управление тегами" panel that actually manages
// the list).
function TagsEditor({
  tags,
  manualTags,
  categories,
  defsByCategory,
  onAdd,
  onRemove,
  pending,
}: {
  tags: string[]
  manualTags: string[]
  categories: string[]
  defsByCategory: Map<string, TagDef[]>
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
  pending: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const labelToCategory = new Map<string, string>()
  for (const [category, defs] of defsByCategory) {
    for (const d of defs) labelToCategory.set(d.label, category)
  }
  const hasRemaining = categories.some((category) =>
    (defsByCategory.get(category) ?? []).some((d) => !manualTags.includes(d.label))
  )
  const sortedManual = sortTagsByCategory(manualTags, categories, labelToCategory)

  type Chip = { kind: 'auto' | 'manual'; value: string }
  const chips: Chip[] = [
    ...tags.map((t): Chip => ({ kind: 'auto', value: t })),
    ...sortedManual.map((t): Chip => ({ kind: 'manual', value: t })),
  ]
  const overflow = chips.length - VISIBLE_TAGS
  const visibleChips = expanded ? chips : chips.slice(0, VISIBLE_TAGS)

  return (
    <div className='flex flex-wrap items-center gap-1'>
      {chips.length === 0 && !hasRemaining && <span className='text-sm text-gray-400'>—</span>}
      {visibleChips.map((chip) =>
        chip.kind === 'auto' ? (
          <Badge key={`auto-${chip.value}`} variant={TAG_BADGE_VARIANT[chip.value] || 'neutral'}>
            {TAG_LABEL[chip.value] || chip.value}
          </Badge>
        ) : (
          <span
            key={`manual-${chip.value}`}
            className={cx(
              'inline-flex items-center gap-0.5 rounded border px-1 py-px text-[10px] font-medium',
              categoryColorClass(labelToCategory.get(chip.value) ?? '', categories)
            )}
          >
            {chip.value}
            <button
              type='button'
              disabled={pending}
              onClick={() => onRemove(chip.value)}
              aria-label={`Убрать тег ${chip.value}`}
              className='px-0.5 leading-none opacity-60 hover:text-rose-600 hover:opacity-100 disabled:cursor-wait'
            >
              ×
            </button>
          </span>
        )
      )}
      {!expanded && overflow > 0 && (
        <button
          type='button'
          onClick={() => setExpanded(true)}
          className='text-[10px] font-medium text-gray-400 hover:text-gray-600 hover:underline'
        >
          +{overflow}
        </button>
      )}
      {expanded && chips.length > VISIBLE_TAGS && (
        <button
          type='button'
          onClick={() => setExpanded(false)}
          className='text-[10px] text-gray-400 hover:text-gray-600 hover:underline'
        >
          свернуть
        </button>
      )}
      {hasRemaining && (
        <div className='relative'>
          <select
            value=''
            disabled={pending}
            onChange={(e) => {
              if (e.target.value) onAdd(e.target.value)
            }}
            aria-label='Добавить тег'
            title='Добавить тег'
            className='h-[22px] w-[22px] cursor-pointer appearance-none rounded-full border border-gray-200 bg-gray-50 text-center text-[11px] leading-[20px] font-medium text-gray-400 outline-none hover:bg-gray-100 disabled:cursor-wait'
          >
            <option value=''>+</option>
            {categories.map((category) => {
              const remaining = (defsByCategory.get(category) ?? []).filter((d) => !manualTags.includes(d.label))
              if (remaining.length === 0) return null
              return (
                <optgroup key={category} label={category}>
                  {remaining.map((d) => (
                    <option key={d.label} value={d.label}>
                      {d.label}
                    </option>
                  ))}
                </optgroup>
              )
            })}
          </select>
        </div>
      )}
    </div>
  )
}
