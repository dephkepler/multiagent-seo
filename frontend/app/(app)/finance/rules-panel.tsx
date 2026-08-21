'use client'

import { useId, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input, Label } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { money, todayISO } from '@/lib/format'
import { PAYMENT_LABEL, type Category, type PaymentMethod, type Rule } from './types'

export interface RuleValues {
  name: string
  category_code: string
  vendor: string
  payment_method: PaymentMethod
  amount: number
  day_of_month: number
  auto_post: boolean
  active_from: string
  active_to?: string
  is_active: boolean
}

interface Props {
  rules: Rule[]
  // optional and defaulted to false: finance/page.tsx does not pass the query's
  // isLoading yet, so until it does this stays a no-op and the panel behaves as before
  loading?: boolean
  categories: Category[]
  pending: boolean
  onCreate: (values: RuleValues) => Promise<unknown>
  onUpdate: (id: string, values: RuleValues) => Promise<unknown>
  onDelete: (rule: Rule) => void
}

export function RulesPanel({ rules, loading = false, categories, pending, onCreate, onUpdate, onDelete }: Props) {
  const [editing, setEditing] = useState<Rule | null>(null)
  const [adding, setAdding] = useState(false)

  return (
    <div>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='text-sm text-gray-500'>
          Шаблон превращается в расход раз в месяц, в указанный день. Без «сразу проводить» — сначала черновик.
        </div>
        {!adding && !editing && (
          <Button size='sm' variant='secondary' onClick={() => setAdding(true)}>
            Добавить шаблон
          </Button>
        )}
      </div>

      {(adding || editing) && (
        <div className='mt-3 rounded-md border border-gray-200 bg-gray-50 p-3'>
          <RuleForm
            key={editing?.id ?? 'new'}
            categories={categories}
            initial={editing ?? undefined}
            pending={pending}
            submitLabel={editing ? 'Сохранить' : 'Создать'}
            onCancel={() => {
              setAdding(false)
              setEditing(null)
            }}
            onSubmit={async (values) => {
              const saved = editing ? await onUpdate(editing.id, values) : await onCreate(values)
              if (saved === undefined) return
              setAdding(false)
              setEditing(null)
            }}
          />
        </div>
      )}

      {loading ? (
        <div className='mt-3 text-sm text-gray-500'>Загрузка…</div>
      ) : rules.length === 0 ? (
        <div className='mt-3 text-sm text-gray-500'>Шаблонов нет.</div>
      ) : (
        <ul className='mt-3 grid gap-2 lg:grid-cols-2'>
          {rules.map((r) => (
            <li key={r.id} className='rounded-md border border-gray-200 bg-white p-3 hover:border-emerald-300 hover:bg-emerald-50/40'>
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <div className='truncate font-medium'>{r.name}</div>
                  <div className='mt-1 flex flex-wrap items-center gap-1'>
                    <Badge variant='neutral'>{r.category_label}</Badge>
                    <Badge variant='neutral'>{PAYMENT_LABEL[r.payment_method]}</Badge>
                    <span className='text-xs text-gray-500'>{r.day_of_month}-го числа</span>
                    {r.auto_post ? <Badge variant='success'>Сразу проводить</Badge> : <Badge variant='warning'>Черновик</Badge>}
                    {!r.is_active && <Badge variant='danger'>Отключён</Badge>}
                  </div>
                  {r.vendor && <div className='mt-1 text-xs text-gray-500'>{r.vendor}</div>}
                </div>
                <div className='shrink-0 text-right font-semibold tabular-nums'>{money(r.amount)}</div>
              </div>
              <div className='mt-2 flex gap-2'>
                <Button size='sm' variant='secondary' onClick={() => setEditing(r)}>
                  Изменить
                </Button>
                <Button size='sm' variant='ghost' className='text-rose-600' onClick={() => onDelete(r)}>
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

interface FormProps {
  categories: Category[]
  initial?: Rule
  submitLabel: string
  pending: boolean
  onSubmit: (values: RuleValues) => void | Promise<void>
  onCancel: () => void
}

function RuleForm({ categories, initial, submitLabel, pending, onSubmit, onCancel }: FormProps) {
  const id = useId()
  const [name, setName] = useState(initial?.name ?? '')
  const [category, setCategory] = useState(initial?.category_code ?? '')
  const [vendor, setVendor] = useState(initial?.vendor ?? '')
  const [method, setMethod] = useState<PaymentMethod>(initial?.payment_method ?? 'card')
  const [amount, setAmount] = useState(initial ? String(initial.amount) : '')
  const [day, setDay] = useState(String(initial?.day_of_month ?? 1))
  const [autoPost, setAutoPost] = useState(initial?.auto_post ?? false)
  const [activeFrom, setActiveFrom] = useState(initial?.active_from ?? todayISO())
  const [activeTo, setActiveTo] = useState(initial?.active_to ?? '')
  const [isActive, setIsActive] = useState(initial?.is_active ?? true)
  const [error, setError] = useState('')

  // categories load after first render, so state seeded from categories[0] would stay empty
  const selectedCategory = category || categories[0]?.code || ''

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const parsedAmount = Number(amount.replace(',', '.'))
    const parsedDay = Number(day)
    if (!name.trim()) return setError('Название обязательно')
    if (!Number.isFinite(parsedAmount) || parsedAmount <= 0) return setError('Сумма должна быть больше нуля')
    if (!selectedCategory) return setError('Выберите категорию')
    if (!Number.isInteger(parsedDay) || parsedDay < 1 || parsedDay > 28) {
      return setError('День месяца — от 1 до 28, чтобы шаблон не пропускал февраль')
    }
    setError('')
    onSubmit({
      name: name.trim(),
      category_code: selectedCategory,
      vendor: vendor.trim(),
      payment_method: method,
      amount: parsedAmount,
      day_of_month: parsedDay,
      auto_post: autoPost,
      active_from: activeFrom,
      active_to: activeTo || undefined,
      is_active: isActive,
    })
  }

  return (
    <form onSubmit={submit} className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
      <div>
        <Label htmlFor={`${id}-name`}>Название</Label>
        <Input id={`${id}-name`} value={name} onChange={(e) => setName(e.target.value)} placeholder='Хостинг' />
      </div>
      <div>
        <Label htmlFor={`${id}-category`}>Категория</Label>
        <Select id={`${id}-category`} value={selectedCategory} onChange={(e) => setCategory(e.target.value)}>
          {categories.map((c) => (
            <option key={c.code} value={c.code}>
              {c.label}
            </option>
          ))}
        </Select>
      </div>
      <div>
        <Label htmlFor={`${id}-amount`}>Сумма, ₴</Label>
        <Input id={`${id}-amount`} inputMode='decimal' value={amount} onChange={(e) => setAmount(e.target.value)} placeholder='642' />
      </div>
      <div>
        <Label htmlFor={`${id}-day`}>День месяца</Label>
        <Input id={`${id}-day`} inputMode='numeric' value={day} onChange={(e) => setDay(e.target.value)} placeholder='14' />
      </div>
      <div>
        <Label htmlFor={`${id}-vendor`}>Контрагент</Label>
        <Input id={`${id}-vendor`} value={vendor} onChange={(e) => setVendor(e.target.value)} placeholder='Бинотель' />
      </div>
      <div>
        <Label htmlFor={`${id}-method`}>Оплата</Label>
        <Select id={`${id}-method`} value={method} onChange={(e) => setMethod(e.target.value as PaymentMethod)}>
          {(Object.keys(PAYMENT_LABEL) as PaymentMethod[]).map((m) => (
            <option key={m} value={m}>
              {PAYMENT_LABEL[m]}
            </option>
          ))}
        </Select>
      </div>
      <div>
        <Label htmlFor={`${id}-active-from`}>Действует с</Label>
        <Input id={`${id}-active-from`} type='date' value={activeFrom} onChange={(e) => setActiveFrom(e.target.value)} />
      </div>
      <div>
        <Label htmlFor={`${id}-active-to`}>Действует до</Label>
        <Input id={`${id}-active-to`} type='date' value={activeTo} onChange={(e) => setActiveTo(e.target.value)} />
      </div>

      <label className='flex items-center gap-2 text-sm sm:col-span-2'>
        <input type='checkbox' checked={autoPost} onChange={(e) => setAutoPost(e.target.checked)} />
        Проводить сразу, без подтверждения
      </label>
      <label className='flex items-center gap-2 text-sm sm:col-span-2'>
        <input type='checkbox' checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
        Шаблон активен
      </label>

      <div className='flex gap-2 sm:col-span-2 lg:col-span-4'>
        <Button type='submit' disabled={pending}>
          {pending ? 'Сохранение…' : submitLabel}
        </Button>
        <Button type='button' variant='secondary' onClick={onCancel}>
          Отмена
        </Button>
      </div>
      {error && (
        <div className='text-sm text-rose-600 sm:col-span-2 lg:col-span-4' role='alert'>
          {error}
        </div>
      )}
    </form>
  )
}
