'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { dateLabel, money, monthLabel } from '@/lib/format'
import { ORIGIN_LABEL, type Expense } from './types'

interface Props {
  drafts: Expense[]
  month: string
  generating: boolean
  pendingId: string | null
  // one in-flight confirm at a time: react-query only remembers the last variables,
  // so a second tap would re-enable the first button mid-request and double-submit
  busy: boolean
  onGenerate: () => void
  onConfirm: (id: string) => void
  onEdit: (expense: Expense) => void
  onDelete: (expense: Expense) => void
}

export function Drafts({ drafts, month, generating, pendingId, busy, onGenerate, onConfirm, onEdit, onDelete }: Props) {
  const sum = drafts.reduce((acc, d) => acc + d.amount, 0)

  return (
    <div className='rounded-lg border border-amber-200 bg-amber-50/50 p-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <div className='font-semibold'>
            К подтверждению
            {drafts.length > 0 && <span className='ml-2 text-sm font-normal text-gray-500'>{money(sum)}</span>}
          </div>
          <div className='text-xs text-gray-500'>Автоматические расходы не попадают в P&L, пока их не подтвердят</div>
        </div>
        <Button variant='secondary' size='sm' onClick={onGenerate} disabled={generating}>
          {generating ? 'Проверяю…' : `Сгенерировать за ${monthLabel(month)}`}
        </Button>
      </div>

      {drafts.length === 0 ? (
        <div className='mt-3 text-sm text-gray-500'>Черновиков нет — всё подтверждено.</div>
      ) : (
        <ul className='mt-3 grid gap-2 lg:grid-cols-2'>
          {drafts.map((d) => (
            <li key={d.id} className='rounded-md border border-gray-200 bg-white p-3 hover:border-emerald-300 hover:bg-emerald-50/40'>
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <div className='truncate font-medium'>{d.vendor || d.description || d.category_label}</div>
                  <div className='mt-0.5 truncate text-xs text-gray-500'>{d.description}</div>
                  <div className='mt-1 flex flex-wrap items-center gap-1'>
                    <Badge variant='neutral'>{d.category_label}</Badge>
                    <Badge variant={d.origin === 'derived' ? 'info' : 'warning'}>{ORIGIN_LABEL[d.origin]}</Badge>
                    <span className='text-xs text-gray-400'>{dateLabel(d.spent_at)}</span>
                  </div>
                </div>
                <div className='shrink-0 text-right font-semibold tabular-nums'>{money(d.amount)}</div>
              </div>

              <div className='mt-3 flex flex-wrap gap-2'>
                <Button size='sm' onClick={() => onConfirm(d.id)} disabled={busy}>
                  {pendingId === d.id ? 'Провожу…' : 'Подтвердить'}
                </Button>
                <Button size='sm' variant='secondary' onClick={() => onEdit(d)}>
                  Изменить
                </Button>
                <Button size='sm' variant='ghost' className='text-rose-600' onClick={() => onDelete(d)}>
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
