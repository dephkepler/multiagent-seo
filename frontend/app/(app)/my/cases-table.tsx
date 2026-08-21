'use client'

import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { dateLabel, money } from '@/lib/format'
import { CASE_STATUS_LABEL, type MyCase } from './types'

interface Props {
  cases: MyCase[]
  busyCaseID: string | null
  onSetStatus: (c: MyCase, status: string) => void
  // The client card already knows whose card it is; the list view links to it.
  showClient?: boolean
}

export function CasesTable({ cases, busyCaseID, onSetStatus, showClient = true }: Props) {
  if (cases.length === 0) {
    return (
      <div className='text-sm text-gray-500'>
        Дел пока нет. Если дело ведёшь ты, а тут его нет — значит в деле не проставлен адвокат, скажи админу.
      </div>
    )
  }

  return (
    <div className='-mx-6 overflow-x-auto sm:mx-0'>
      <table className='w-full min-w-max text-sm'>
        <thead>
          <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
            {showClient && <th className='py-2 pr-3 font-medium'>Клиент</th>}
            <th className='py-2 pr-3 font-medium'>Категория</th>
            <th className='py-2 pr-3 font-medium'>Статус</th>
            <th className='py-2 pr-3 text-right font-medium'>Гонорар</th>
            <th className='py-2 pr-3 text-right font-medium'>Оплачено</th>
            <th className='py-2 pr-3 text-right font-medium'>Остаток</th>
            <th className='py-2 pr-3 font-medium'>Оплаты</th>
            <th className='py-2 pr-3 font-medium'>Открыто</th>
            <th className='py-2 font-medium'></th>
          </tr>
        </thead>
        <tbody>
          {cases.map((c) => (
            <tr key={c.id} className='border-b border-gray-100 transition-colors hover:bg-emerald-50'>
              {showClient && (
                <td className='py-2 pr-3'>
                  <Link href={`/my/clients/${c.client_id}`} className='font-medium text-emerald-700 hover:underline'>
                    {c.client_name || 'без имени'}
                  </Link>
                  {c.client_phone && <div className='text-xs text-gray-400'>{c.client_phone}</div>}
                </td>
              )}
              <td className='py-2 pr-3'>{c.category || <span className='text-gray-400'>—</span>}</td>
              <td className='py-2 pr-3'>
                <Badge variant={c.status === 'completed' ? 'success' : c.status === 'cancelled' ? 'neutral' : 'info'}>
                  {CASE_STATUS_LABEL[c.status] ?? c.status}
                </Badge>
              </td>
              <td className='py-2 pr-3 text-right tabular-nums'>{money(c.fee)}</td>
              <td className='py-2 pr-3 text-right tabular-nums'>{money(c.paid)}</td>
              <td className={'py-2 pr-3 text-right font-medium tabular-nums ' + (c.owed > 0 ? 'text-rose-600' : 'text-gray-400')}>
                {c.owed > 0 ? money(c.owed) : '—'}
              </td>
              <td className='py-2 pr-3 text-xs text-gray-500'>
                {c.payments.length === 0 ? (
                  <span className='text-gray-400'>нет</span>
                ) : (
                  c.payments.map((p) => `${dateLabel(p.paid_at)} · ${money(p.amount)}`).join(', ')
                )}
              </td>
              <td className='py-2 pr-3 whitespace-nowrap text-gray-500'>{c.created_at.slice(0, 10)}</td>
              <td className='py-2'>
                {c.status === 'cancelled' ? (
                  <span className='text-xs text-gray-400'>отменено админом</span>
                ) : (
                  <Button
                    variant='secondary'
                    size='sm'
                    disabled={busyCaseID === c.id}
                    onClick={() => onSetStatus(c, c.status === 'completed' ? 'in_progress' : 'completed')}
                  >
                    {c.status === 'completed' ? 'Вернуть в работу' : 'Завершить'}
                  </Button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
