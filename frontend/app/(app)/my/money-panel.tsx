'use client'

import { monthLabel, money } from '@/lib/format'
import type { MySettlement } from './types'

export function MoneyPanel({ data }: { data: MySettlement }) {
  return (
    <div>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
        <Figure label='Собрано по моим делам' value={money(data.collected)} />
        <Figure label='Моя ставка' value={data.commission_percent > 0 ? data.commission_percent + '%' : 'не задана'} />
        <Figure label='Начислено мне' value={money(data.accrued)} />
        <Figure
          label='Осталось получить'
          value={money(data.outstanding)}
          accent={data.outstanding > 0 ? 'good' : undefined}
        />
      </div>

      {data.commission_percent === 0 && (
        <div className='mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800'>
          Ставка не задана, поэтому начисление считается как 0. Ставку выставляет админ.
        </div>
      )}

      {data.months.length > 0 && (
        <div className='mt-4 -mx-6 overflow-x-auto sm:mx-0'>
          <table className='w-full min-w-max text-sm'>
            <thead>
              <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
                <th className='py-2 pr-3 font-medium'>Месяц</th>
                <th className='py-2 pr-3 text-right font-medium'>Собрано</th>
                <th className='py-2 text-right font-medium'>Начислено</th>
              </tr>
            </thead>
            <tbody>
              {data.months.map((m) => (
                <tr key={m.month} className='border-b border-gray-100 transition-colors hover:bg-emerald-50'>
                  <td className='py-2 pr-3'>{monthLabel(m.month)}</td>
                  <td className='py-2 pr-3 text-right tabular-nums'>{money(m.collected)}</td>
                  <td className='py-2 text-right tabular-nums'>{money(m.accrued)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data.paid_is_partial && (
        <div className='mt-3 text-xs text-gray-500'>
          Выплачено {money(data.paid)} — здесь видны только выплаты, проведённые через систему. Если что-то отдавали
          наличными или общей суммой без разбивки по людям, в этой цифре этого нет, и «осталось получить» может быть
          завышено. Сверяйся с админом, а не считай это окончательным расчётом.
        </div>
      )}
    </div>
  )
}

function Figure({ label, value, accent }: { label: string; value: string; accent?: 'good' }) {
  return (
    <div className='rounded-md border border-gray-200 bg-white p-3'>
      <div className='text-[11px] tracking-wide text-gray-500 uppercase'>{label}</div>
      <div className={'mt-1 font-semibold tabular-nums ' + (accent === 'good' ? 'text-emerald-700' : '')}>{value}</div>
    </div>
  )
}
