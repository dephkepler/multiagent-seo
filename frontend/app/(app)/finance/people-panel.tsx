'use client'

import { money, percent } from '@/lib/format'
import type { Category, FinanceMonth } from './types'

interface Props {
  categories: Category[]
  // period totals; the drilled-into month when one is selected
  month: FinanceMonth
  monthLabel: string
}

// The P&L groups spend by what the work was for — the right shape for a P&L, and
// the shape the company's own spreadsheet used. The cost of that: money paid to
// people is scattered across marketing, development and delivery, so the
// "Зарплаты" line shows one assistant while most of the budget is people.
// This block answers the other question — who got paid, and how much in total —
// without re-classifying anything.
export function PeoplePanel({ categories, month, monthLabel }: Props) {
  const people = categories
    .filter((c) => c.is_people_pay)
    .map((c) => ({ label: c.label, amount: month.expense_by_category[c.code] ?? 0 }))
    .sort((a, b) => b.amount - a.amount)

  const total = people.reduce((sum, p) => sum + p.amount, 0)

  if (people.length === 0) {
    return (
      <div className='text-sm text-gray-500'>Ни одна статья не отмечена как выплата человеку. Отметка ставится на статье расходов.</div>
    )
  }

  return (
    <div>
      <div className='text-sm text-gray-500'>
        {monthLabel}: людям ушло <span className='font-medium text-gray-800'>{money(total)}</span> —{' '}
        {percent(total / (month.expense_total || 1), month.expense_total === 0)} всех расходов. В P&L эти же деньги разложены по назначению
        работы, поэтому строка «Зарплаты» там показывает только штатную выплату.
      </div>

      <ul className='mt-3 divide-y divide-gray-100'>
        {people.map((p) => (
          <li key={p.label} className='flex items-center gap-3 py-2 hover:bg-emerald-50/70'>
            <span className='min-w-0 flex-1 truncate'>{p.label}</span>
            <span className='w-16 text-right text-xs text-gray-500 tabular-nums'>{percent(p.amount / (total || 1), total === 0)}</span>
            <span className='w-28 text-right font-medium tabular-nums'>{p.amount ? money(p.amount) : '—'}</span>
          </li>
        ))}
        <li className='flex items-center gap-3 py-2 font-semibold'>
          <span className='min-w-0 flex-1'>Всего людям</span>
          <span className='w-16 text-right text-xs text-gray-500 tabular-nums'>
            {percent(total / (month.expense_total || 1), month.expense_total === 0)}
          </span>
          <span className='w-28 text-right tabular-nums'>{money(total)}</span>
        </li>
      </ul>
    </div>
  )
}
