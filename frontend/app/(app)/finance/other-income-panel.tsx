'use client'

import { useId, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input, Label } from '@/components/ui/input'
import { dateLabel, money, todayISO } from '@/lib/format'
import type { OtherIncome } from './types'

export interface OtherIncomeValues {
  received_at: string
  amount: number
  source: string
  description: string
}

interface Props {
  items: OtherIncome[]
  // optional and defaulted to false: finance/page.tsx does not pass the query's
  // isLoading yet, so until it does this stays a no-op and the panel behaves as before
  loading?: boolean
  pending: boolean
  onCreate: (values: OtherIncomeValues) => Promise<unknown>
  onDelete: (income: OtherIncome) => void
}

export function OtherIncomePanel({ items, loading = false, pending, onCreate, onDelete }: Props) {
  const id = useId()
  const [receivedAt, setReceivedAt] = useState(todayISO())
  const [amount, setAmount] = useState('')
  const [source, setSource] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    const parsed = Number(amount.replace(',', '.'))
    if (!Number.isFinite(parsed) || parsed <= 0) {
      setError('Сумма должна быть больше нуля')
      return
    }
    setError('')
    // fields are cleared only once the row is really saved, so a 400 doesn't erase what was typed
    const saved = await onCreate({
      received_at: receivedAt,
      amount: parsed,
      source: source.trim(),
      description: description.trim(),
    })
    if (saved === undefined) return
    setAmount('')
    setSource('')
    setDescription('')
  }

  return (
    <div>
      <div className='text-sm text-gray-500'>
        Деньги, которые не прошли через консультацию или дело: возвраты, пополнения «от компании», разовые поступления. Доходы по клиентам
        сюда вносить не нужно — они уже считаются из CRM.
      </div>

      <form onSubmit={submit} className='mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-5'>
        <div>
          <Label htmlFor={`${id}-received-at`}>Дата</Label>
          <Input id={`${id}-received-at`} type='date' value={receivedAt} onChange={(e) => setReceivedAt(e.target.value)} />
        </div>
        <div>
          <Label htmlFor={`${id}-amount`}>Сумма, ₴</Label>
          <Input
            id={`${id}-amount`}
            inputMode='decimal'
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder='15000'
          />
        </div>
        <div>
          <Label htmlFor={`${id}-source`}>Источник</Label>
          <Input id={`${id}-source`} value={source} onChange={(e) => setSource(e.target.value)} placeholder='от компании' />
        </div>
        <div>
          <Label htmlFor={`${id}-description`}>Описание</Label>
          <Input id={`${id}-description`} value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>
        <div className='flex items-end'>
          <Button type='submit' disabled={pending} className='w-full'>
            {pending ? 'Сохранение…' : 'Добавить'}
          </Button>
        </div>
        {error && (
          <div className='text-sm text-rose-600 sm:col-span-2 lg:col-span-5' role='alert'>
            {error}
          </div>
        )}
      </form>

      {loading ? (
        <div className='mt-3 text-sm text-gray-500'>Загрузка…</div>
      ) : items.length === 0 ? (
        <div className='mt-3 text-sm text-gray-500'>Доходов за месяц нет.</div>
      ) : (
        <ul className='mt-3 grid gap-2 lg:grid-cols-2'>
          {items.map((i) => (
            <li key={i.id} className='flex items-center justify-between gap-2 rounded-md border border-gray-200 bg-white p-3'>
              <div className='min-w-0'>
                <div className='truncate font-medium'>{i.source || i.description || 'Прочий доход'}</div>
                <div className='text-xs text-gray-500'>
                  {dateLabel(i.received_at)}
                  {i.description && i.source ? ` · ${i.description}` : ''}
                </div>
              </div>
              <div className='flex shrink-0 items-center gap-2'>
                <span className='font-semibold tabular-nums'>{money(i.amount)}</span>
                <Button size='sm' variant='ghost' className='text-rose-600' onClick={() => onDelete(i)}>
                  Удалить
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
