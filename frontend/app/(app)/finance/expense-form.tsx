'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { PAYMENT_LABEL, type Category, type Expense, type PaymentMethod } from './types'
import { todayISO } from './format'

export interface ExpenseValues {
  spent_at: string
  amount: number
  category_code: string
  payment_method: PaymentMethod
  vendor: string
  description: string
}

interface Props {
  categories: Category[]
  initial?: Expense
  submitLabel: string
  pending?: boolean
  onSubmit: (values: ExpenseValues) => void
  onCancel?: () => void
}

export function ExpenseForm({ categories, initial, submitLabel, pending, onSubmit, onCancel }: Props) {
  const [spentAt, setSpentAt] = useState(initial?.spent_at ?? todayISO())
  const [amount, setAmount] = useState(initial ? String(initial.amount) : '')
  const [category, setCategory] = useState(initial?.category_code ?? '')
  const [method, setMethod] = useState<PaymentMethod>(initial?.payment_method ?? 'card')
  const [vendor, setVendor] = useState(initial?.vendor ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [error, setError] = useState('')

  // falls back to the first category once the vocabulary loads, so the select's
  // displayed value and the submitted value can never disagree
  const selectedCategory = category || categories[0]?.code || ''

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const parsed = Number(amount.replace(',', '.'))
    if (!Number.isFinite(parsed) || parsed <= 0) {
      setError('Сумма должна быть больше нуля')
      return
    }
    if (!selectedCategory) {
      setError('Выберите категорию')
      return
    }
    setError('')
    onSubmit({
      spent_at: spentAt,
      amount: parsed,
      category_code: selectedCategory,
      payment_method: method,
      vendor: vendor.trim(),
      description: description.trim(),
    })
  }

  return (
    <form onSubmit={submit} className='grid gap-2 sm:grid-cols-2 lg:grid-cols-12'>
      <div className='lg:col-span-2'>
        <Label>Дата</Label>
        <Input type='date' value={spentAt} onChange={(e) => setSpentAt(e.target.value)} />
      </div>
      <div className='lg:col-span-2'>
        <Label>Сумма, ₴</Label>
        <Input inputMode='decimal' value={amount} onChange={(e) => setAmount(e.target.value)} placeholder='9140' />
      </div>
      <div className='lg:col-span-3'>
        <Label>Категория</Label>
        <Select value={selectedCategory} onChange={(e) => setCategory(e.target.value)}>
          {categories.map((c) => (
            <option key={c.code} value={c.code}>
              {c.label}
            </option>
          ))}
        </Select>
      </div>
      <div className='lg:col-span-2'>
        <Label>Оплата</Label>
        <Select value={method} onChange={(e) => setMethod(e.target.value as PaymentMethod)}>
          {(Object.keys(PAYMENT_LABEL) as PaymentMethod[]).map((m) => (
            <option key={m} value={m}>
              {PAYMENT_LABEL[m]}
            </option>
          ))}
        </Select>
      </div>
      <div className='lg:col-span-3'>
        <Label>Контрагент</Label>
        <Input value={vendor} onChange={(e) => setVendor(e.target.value)} placeholder='Алсана' />
      </div>
      <div className='sm:col-span-2 lg:col-span-9'>
        <Label>Описание</Label>
        <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder='Контекст, зарплата за месяц' />
      </div>
      <div className='flex items-end gap-2 sm:col-span-2 lg:col-span-3'>
        <Button type='submit' disabled={pending} className='w-full'>
          {pending ? 'Сохранение…' : submitLabel}
        </Button>
        {onCancel && (
          <Button type='button' variant='secondary' onClick={onCancel} className='w-full'>
            Отмена
          </Button>
        )}
      </div>
      {error && <div className='text-sm text-rose-600 sm:col-span-2 lg:col-span-12'>{error}</div>}
    </form>
  )
}

function Label({ children }: { children: React.ReactNode }) {
  return <div className='mb-1 text-xs text-gray-500'>{children}</div>
}
