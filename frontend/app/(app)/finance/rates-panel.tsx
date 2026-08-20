'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AdvocateRate } from './types'

interface Props {
  rates: AdvocateRate[]
  pendingId: string | null
  onSave: (advocateID: string, percent: number) => void
}

export function RatesPanel({ rates, pendingId, onSave }: Props) {
  return (
    <div>
      <div className='text-sm text-gray-500'>
        Процент от денег, которые адвокат собрал по делам за месяц. 0 — выплата не считается автоматически. Расчёт всегда попадает в
        черновики: это платёж человеку, его подтверждают руками.
      </div>

      {rates.length === 0 ? (
        <div className='mt-3 text-sm text-gray-500'>Адвокатов нет — добавьте через бота командой /advocate.</div>
      ) : (
        <ul className='mt-3 grid gap-2 lg:grid-cols-2'>
          {rates.map((rate) => (
            <RateRow
              key={`${rate.advocate_id}:${rate.commission_percent}`}
              rate={rate}
              pending={pendingId === rate.advocate_id}
              onSave={onSave}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function RateRow({
  rate,
  pending,
  onSave,
}: {
  rate: AdvocateRate
  pending: boolean
  onSave: (advocateID: string, percent: number) => void
}) {
  const [value, setValue] = useState(String(rate.commission_percent))
  const [error, setError] = useState('')
  const dirty = Number(value.replace(',', '.')) !== rate.commission_percent

  function save() {
    const parsed = Number(value.replace(',', '.'))
    if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) {
      setError('От 0 до 100')
      return
    }
    setError('')
    onSave(rate.advocate_id, parsed)
  }

  return (
    <li className='flex flex-wrap items-center gap-2 rounded-md border border-gray-200 bg-white p-3'>
      <div className='min-w-0 flex-1'>
        <div className='truncate font-medium'>{rate.full_name}</div>
        {!rate.is_active && <Badge variant='neutral'>Неактивен</Badge>}
        {error && <div className='text-xs text-rose-600'>{error}</div>}
      </div>
      <div className='flex items-center gap-1'>
        <Input inputMode='decimal' value={value} onChange={(e) => setValue(e.target.value)} className='w-20' />
        <span className='text-sm text-gray-500'>%</span>
      </div>
      <Button size='sm' variant='secondary' disabled={!dirty || pending} onClick={save}>
        {pending ? '…' : 'Сохранить'}
      </Button>
    </li>
  )
}
