'use client'

import { useMemo } from 'react'
import { cx } from '@/lib/cx'
import { moneyShort, monthLabel, percent, times } from './format'
import { KIND_LABEL, type Category, type ExpenseKind, type FinanceReport } from './types'

const KIND_ORDER: ExpenseKind[] = ['marketing', 'direct', 'payroll', 'development', 'infra', 'admin']

interface Props {
  report: FinanceReport
  categories: Category[]
  activeMonth: string
  onPickMonth: (month: string) => void
}

export function PLTable({ report, categories, activeMonth, onPickMonth }: Props) {
  const months = report.months
  const activeIndex = months.findIndex((m) => m.month === activeMonth)

  // Only categories with actual spend in the window — the full vocabulary would
  // add a dozen empty rows on a page whose whole job is showing where money went.
  const rows = useMemo(() => {
    const spent = new Set<string>()
    for (const m of months) {
      for (const [code, amount] of Object.entries(m.expense_by_category)) {
        if (amount) spent.add(code)
      }
    }
    return KIND_ORDER.flatMap((kind) =>
      categories
        .filter((c) => c.kind === kind && spent.has(c.code))
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((c) => ({ kind, category: c }))
    )
  }, [months, categories])

  // ROMI of 0 is a real break-even result; only "no marketing spend at all" is undefined.
  const noSpend = months.map((m) => m.marketing_spend === 0)
  const colSpan = months.length + 2

  return (
    <div className='-mx-6 overflow-x-auto sm:mx-0'>
      <table className='w-full min-w-max border-separate border-spacing-0 text-sm'>
        <thead>
          <tr>
            <Th sticky>Статья</Th>
            {months.map((m) => (
              <Th key={m.month} align='right'>
                <button
                  type='button'
                  onClick={() => onPickMonth(m.month)}
                  className={cx(
                    'min-h-[40px] rounded px-2.5 py-2 whitespace-nowrap',
                    m.month === activeMonth ? 'bg-emerald-100 font-semibold text-emerald-800' : 'hover:bg-gray-100'
                  )}
                >
                  {monthLabel(m.month)}
                </button>
              </Th>
            ))}
            <Th align='right'>Итого</Th>
          </tr>
        </thead>

        <tbody>
          <GroupRow label='Доходы' colSpan={colSpan} />
          <Row
            label='Консультации'
            values={months.map((m) => m.income_consult)}
            total={report.total.income_consult}
            activeIndex={activeIndex}
          />
          <Row
            label='Дела (оплаты)'
            values={months.map((m) => m.income_cases)}
            total={report.total.income_cases}
            activeIndex={activeIndex}
          />
          <Row label='Прочее' values={months.map((m) => m.income_other)} total={report.total.income_other} activeIndex={activeIndex} />
          <Row
            label='Итого доходов'
            values={months.map((m) => m.income_total)}
            total={report.total.income_total}
            activeIndex={activeIndex}
            strong
          />

          <GroupRow label='Расходы' colSpan={colSpan} />
          {rows.map(({ kind, category }) => (
            <Row
              key={category.code}
              label={category.label}
              hint={KIND_LABEL[kind]}
              values={months.map((m) => m.expense_by_category[category.code] ?? 0)}
              total={report.total.expense_by_category[category.code] ?? 0}
              activeIndex={activeIndex}
            />
          ))}
          <Row
            label='Итого расходов'
            values={months.map((m) => m.expense_total)}
            total={report.total.expense_total}
            activeIndex={activeIndex}
            strong
          />

          <GroupRow label='Результат' colSpan={colSpan} />
          <Row label='Баланс' values={months.map((m) => m.balance)} total={report.total.balance} activeIndex={activeIndex} strong signed />
          <Row
            label='Нарастающий итог'
            hint='на конец месяца, за всё время'
            values={months.map((m) => m.cumulative)}
            total={report.total.cumulative}
            activeIndex={activeIndex}
            signed
          />
          <Row
            label='Маржа'
            hint='баланс / доход'
            values={months.map((m) => m.margin_percent)}
            total={report.total.margin_percent}
            activeIndex={activeIndex}
            share
            undefinedAt={months.map((m) => m.income_total === 0)}
            totalUndefined={report.total.income_total === 0}
          />
          <Row
            label='Валовая прибыль'
            hint='доход минус себестоимость'
            values={months.map((m) => m.gross_profit)}
            total={report.total.gross_profit}
            activeIndex={activeIndex}
            signed
          />

          <GroupRow label='Привлечение' colSpan={colSpan} />
          <Row label='Лиды' values={months.map((m) => m.leads)} total={report.total.leads} activeIndex={activeIndex} plain />
          <Row
            label='Новых клиентов'
            hint='первая заявка в этом месяце'
            values={months.map((m) => m.new_clients)}
            total={report.total.new_clients}
            activeIndex={activeIndex}
            plain
          />
          <Row
            label='Из них заплатили'
            hint='когда-либо — знаменатель CAC'
            values={months.map((m) => m.cohort_payers)}
            total={report.total.cohort_payers}
            activeIndex={activeIndex}
            plain
          />
          <Row
            label='CAC'
            hint='маркетинг / клиент, который заплатил'
            values={months.map((m) => m.cac)}
            total={report.total.cac}
            activeIndex={activeIndex}
          />
          <Row label='CPL' hint='маркетинг / лид' values={months.map((m) => m.cpl)} total={report.total.cpl} activeIndex={activeIndex} />
          <Row
            label='ROMI'
            hint='(доход − маркетинг) / маркетинг'
            values={months.map((m) => m.romi)}
            total={report.total.romi}
            activeIndex={activeIndex}
            ratio
            undefinedAt={noSpend}
            totalUndefined={report.total.marketing_spend === 0}
          />
          <Row
            label='Доля маркетинга'
            hint='в расходах'
            values={months.map((m) => m.marketing_share)}
            total={report.total.marketing_share}
            activeIndex={activeIndex}
            share
            undefinedAt={months.map((m) => m.expense_total === 0)}
            totalUndefined={report.total.expense_total === 0}
          />
          <Row
            label='Лид → консультация'
            values={months.map((m) => m.lead_to_consult)}
            total={report.total.lead_to_consult}
            activeIndex={activeIndex}
            share
            undefinedAt={months.map((m) => m.leads === 0)}
            totalUndefined={report.total.leads === 0}
          />
          <Row
            label='Доход на клиента'
            hint='за период / платящих клиентов'
            values={months.map((m) => m.revenue_per_client)}
            total={report.total.revenue_per_client}
            activeIndex={activeIndex}
          />
          <Row
            label='LTV'
            hint='сколько принёс клиент этого месяца за всю жизнь'
            values={months.map((m) => m.ltv)}
            total={report.total.ltv}
            activeIndex={activeIndex}
          />
          <Row
            label='LTV / CAC'
            hint='<1 — привлечение не окупается'
            values={months.map((m) => m.ltv_to_cac)}
            total={report.total.ltv_to_cac}
            activeIndex={activeIndex}
            ratio
            undefinedAt={months.map((m) => m.cac === 0)}
            totalUndefined={report.total.cac === 0}
          />

          <GroupRow label='Средние и порог' colSpan={colSpan} />
          <Row
            label='Средний чек консультации'
            values={months.map((m) => m.avg_consult_ticket)}
            total={report.total.avg_consult_ticket}
            activeIndex={activeIndex}
          />
          <Row
            label='Консультаций проведено'
            values={months.map((m) => m.consult_count)}
            total={report.total.consult_count}
            activeIndex={activeIndex}
            plain
          />
          <Row
            label='Средний чек по делу'
            hint='на одну оплату'
            values={months.map((m) => m.avg_case_ticket)}
            total={report.total.avg_case_ticket}
            activeIndex={activeIndex}
          />
          <Row
            label='Оплат по делам'
            values={months.map((m) => m.case_payment_count)}
            total={report.total.case_payment_count}
            activeIndex={activeIndex}
            plain
          />
          <Row
            label='Точка безубыточности'
            hint='консультаций нужно, чтобы покрыть расходы'
            values={months.map((m) => m.break_even_consults)}
            total={report.total.break_even_consults}
            activeIndex={activeIndex}
            plainRounded
            undefinedAt={months.map((m) => m.avg_consult_ticket === 0)}
            totalUndefined={report.total.avg_consult_ticket === 0}
          />
          <Row
            label='Рост дохода'
            hint='к предыдущему месяцу'
            values={months.map((m) => m.income_growth)}
            total={0}
            activeIndex={activeIndex}
            share
            signed
            undefinedAt={months.map((m, i) => i === 0 || months[i - 1].income_total === 0)}
            totalUndefined
          />
        </tbody>
      </table>
    </div>
  )
}

function Th({ children, align, sticky }: { children: React.ReactNode; align?: 'right'; sticky?: boolean }) {
  return (
    <th
      className={cx(
        'border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500',
        align === 'right' ? 'text-right' : 'text-left',
        sticky && 'sticky left-0 z-10 min-w-[150px]'
      )}
    >
      {children}
    </th>
  )
}

// The label sits in its own sticky cell with the filler spanning the rest: a
// single colSpan cell is as wide as the scroll content, so sticky does nothing
// and the group heading scrolls away from the rows it heads.
function GroupRow({ label, colSpan }: { label: string; colSpan: number }) {
  return (
    <tr>
      <td className='sticky left-0 z-10 border-b border-gray-200 bg-gray-50 px-3 py-1.5 text-[11px] font-semibold tracking-wide text-gray-500 uppercase'>
        {label}
      </td>
      <td colSpan={colSpan - 1} className='border-b border-gray-200 bg-gray-50' />
    </tr>
  )
}

interface RowProps {
  label: string
  hint?: string
  values: number[]
  total: number
  activeIndex: number
  strong?: boolean
  signed?: boolean
  plain?: boolean
  plainRounded?: boolean
  ratio?: boolean
  share?: boolean
  undefinedAt?: boolean[]
  totalUndefined?: boolean
}

function Row({
  label,
  hint,
  values,
  total,
  activeIndex,
  strong,
  signed,
  plain,
  plainRounded,
  ratio,
  share,
  undefinedAt,
  totalUndefined,
}: RowProps) {
  const fmt = (v: number, undef: boolean) => {
    if (plain) return v === 0 ? '—' : String(v)
    if (plainRounded) return undef || v === 0 ? '—' : String(Math.round(v))
    if (ratio) return times(v, undef)
    if (share) return percent(v, undef)
    return moneyShort(v)
  }
  return (
    <tr className='group cursor-default'>
      <td
        className={cx(
          'sticky left-0 z-10 border-b border-gray-100 bg-white px-3 py-1.5 group-hover:bg-emerald-100 group-hover:font-medium',
          strong && 'font-semibold'
        )}
      >
        {label}
        {hint && <span className='block text-[11px] text-gray-400 lg:ml-1 lg:inline'>{hint}</span>}
      </td>
      {values.map((v, i) => (
        <td
          key={i}
          className={cx(
            'border-b border-gray-100 px-3 py-1.5 text-right tabular-nums group-hover:bg-emerald-50',
            i === activeIndex && 'bg-emerald-50',
            strong && 'font-semibold',
            signed && v < 0 && 'text-rose-600',
            signed && v > 0 && 'text-emerald-700'
          )}
        >
          {fmt(v, undefinedAt?.[i] ?? false)}
        </td>
      ))}
      <td
        className={cx(
          'border-b border-gray-100 bg-gray-50 px-3 py-1.5 text-right font-medium tabular-nums group-hover:bg-emerald-100',
          signed && total < 0 && 'text-rose-600',
          signed && total > 0 && 'text-emerald-700'
        )}
      >
        {fmt(total, totalUndefined ?? false)}
      </td>
    </tr>
  )
}
