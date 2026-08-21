'use client'

import { Badge } from '@/components/ui/badge'
import { money } from './format'

export interface AdvocateSettlementRow {
  advocate_id: string
  full_name: string
  commission_percent: number
  collected: number
  accrued: number
  paid: number
  outstanding: number
}

export interface SettlementData {
  items: AdvocateSettlementRow[]
  unattributed_paid: number
  consult_income: number
  case_income: number
  total_accrued: number
  total_paid: number
  total_outstanding: number
}

interface Props {
  data?: SettlementData
  loading: boolean
}

export function SettlementPanel({ data, loading }: Props) {
  if (loading) return <div className='text-sm text-gray-500'>Загрузка…</div>
  if (!data) return <div className='text-sm text-rose-600'>Не удалось загрузить расчёты.</div>

  return (
    <div>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
        <Figure label='Принесли консультации' value={money(data.consult_income)} />
        <Figure label='Принесли дела' value={money(data.case_income)} />
        <Figure label='Начислено адвокатам' value={money(data.total_accrued)} />
        <Figure label='Осталось отдать' value={money(data.total_outstanding)} accent={data.total_outstanding > 0 ? 'bad' : undefined} />
      </div>

      {data.items.length === 0 ? (
        <div className='mt-4 text-sm text-gray-500'>
          Ни по одному адвокату нет оплат за период. Проверь ставки — при 0% выплата не начисляется.
        </div>
      ) : (
        <div className='mt-4 -mx-6 overflow-x-auto sm:mx-0'>
          <table className='w-full min-w-max text-sm'>
            <thead>
              <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
                <th className='py-2 pr-3 font-medium'>Адвокат</th>
                <th className='py-2 pr-3 text-right font-medium'>Ставка</th>
                <th className='py-2 pr-3 text-right font-medium'>Собрал по делам</th>
                <th className='py-2 pr-3 text-right font-medium'>Начислено</th>
                <th className='py-2 pr-3 text-right font-medium'>Выплачено</th>
                <th className='py-2 text-right font-medium'>Осталось отдать</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((a) => (
                <tr key={a.advocate_id} className='border-b border-gray-100 hover:bg-emerald-50/70'>
                  <td className='py-2 pr-3'>{a.full_name || <span className='text-gray-400'>без имени</span>}</td>
                  <td className='py-2 pr-3 text-right tabular-nums'>
                    {a.commission_percent > 0 ? a.commission_percent + '%' : <Badge variant='warning'>не задана</Badge>}
                  </td>
                  <td className='py-2 pr-3 text-right tabular-nums'>{money(a.collected)}</td>
                  <td className='py-2 pr-3 text-right tabular-nums'>{money(a.accrued)}</td>
                  <td className='py-2 pr-3 text-right tabular-nums'>{money(a.paid)}</td>
                  <td className={'py-2 text-right font-medium tabular-nums ' + (a.outstanding > 0 ? 'text-rose-600' : 'text-gray-500')}>
                    {money(a.outstanding)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data.unattributed_paid > 0 && (
        <div className='mt-3 text-xs text-gray-500'>
          Ещё {money(data.unattributed_paid)} выплат по статье «Адвокаты» не привязаны к человеку — это перенесённые из таблицы общие суммы.
          Они в расходах учтены, но по людям не разложены: делить их догадкой было бы выдумыванием.
        </div>
      )}
    </div>
  )
}

function Figure({ label, value, accent }: { label: string; value: string; accent?: 'bad' }) {
  return (
    <div className='rounded-md border border-gray-200 bg-white p-3'>
      <div className='text-[11px] tracking-wide text-gray-500 uppercase'>{label}</div>
      <div className={'mt-1 font-semibold tabular-nums ' + (accent === 'bad' ? 'text-rose-700' : '')}>{value}</div>
    </div>
  )
}
