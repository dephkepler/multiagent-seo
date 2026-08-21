// The page used to offer "6 / 12 / 24 months back from today". With the firm's
// history sitting in 2024–2025 and today in 2026, the default window was twelve
// empty columns with the same running total repeated in each — which reads as
// broken data rather than an empty period. So periods are built from the range
// the data actually covers, and nothing empty is ever offered.

export type PeriodKind = 'all' | 'year' | 'quarter' | 'month'

export interface Period {
  kind: PeriodKind
  // '' for 'all'; '2025' for a year; '2025-Q2' for a quarter; '2025-04' for a month
  value: string
}

export interface DataRange {
  has_data: boolean
  first_month?: string
  last_month?: string
}

const MONTH_NAME = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь']
const QUARTER_LABEL = ['I', 'II', 'III', 'IV']

export const KIND_LABEL: Record<PeriodKind, string> = {
  all: 'Всё время',
  year: 'Год',
  quarter: 'Квартал',
  month: 'Месяц',
}

function parseMonth(key: string): { year: number; month: number } {
  const [y, m] = key.split('-').map(Number)
  return { year: y, month: m }
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// Every month between first and last inclusive, newest first — the order a
// finance page is read in.
export function monthsInRange(range: DataRange): string[] {
  if (!range.has_data || !range.first_month || !range.last_month) return []
  const first = parseMonth(range.first_month)
  const last = parseMonth(range.last_month)
  const out: string[] = []
  for (let y = last.year, m = last.month; y > first.year || (y === first.year && m >= first.month);) {
    out.push(`${y}-${pad(m)}`)
    m -= 1
    if (m === 0) {
      m = 12
      y -= 1
    }
  }
  return out
}

export function yearsInRange(range: DataRange): string[] {
  const years = new Set(monthsInRange(range).map((m) => m.slice(0, 4)))
  return [...years].sort().reverse()
}

export function quartersInRange(range: DataRange): string[] {
  const quarters = new Set(
    monthsInRange(range).map((key) => {
      const { year, month } = parseMonth(key)
      return `${year}-Q${Math.ceil(month / 3)}`
    })
  )
  return [...quarters].sort().reverse()
}

export function optionsFor(kind: PeriodKind, range: DataRange): { value: string; label: string }[] {
  switch (kind) {
    case 'year':
      return yearsInRange(range).map((y) => ({ value: y, label: y }))
    case 'quarter':
      return quartersInRange(range).map((q) => ({ value: q, label: periodLabel({ kind: 'quarter', value: q }) }))
    case 'month':
      return monthsInRange(range).map((m) => ({ value: m, label: periodLabel({ kind: 'month', value: m }) }))
    default:
      return []
  }
}

export function periodLabel(period: Period, range?: DataRange): string {
  switch (period.kind) {
    case 'year':
      return period.value
    case 'quarter': {
      const [year, q] = period.value.split('-Q')
      return `${QUARTER_LABEL[Number(q) - 1]} кв. ${year}`
    }
    case 'month': {
      const { year, month } = parseMonth(period.value)
      return `${MONTH_NAME[month - 1]} ${String(year).slice(2)}`
    }
    default:
      if (range?.has_data && range.first_month && range.last_month) {
        return `Всё время (${periodLabel({ kind: 'month', value: range.first_month })} — ${periodLabel({ kind: 'month', value: range.last_month })})`
      }
      return 'Всё время'
  }
}

function lastDay(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

// A window the report can be asked for. Falls back to the current year only when
// there is no data at all — with data, every kind resolves inside it.
export function windowFor(period: Period, range: DataRange): { from: string; to: string } {
  const thisYear = new Date().getFullYear()

  if (period.kind === 'all' || !period.value) {
    if (!range.has_data || !range.first_month || !range.last_month) {
      return { from: `${thisYear}-01-01`, to: `${thisYear}-12-31` }
    }
    const first = parseMonth(range.first_month)
    const last = parseMonth(range.last_month)
    return {
      from: `${first.year}-${pad(first.month)}-01`,
      to: `${last.year}-${pad(last.month)}-${pad(lastDay(last.year, last.month))}`,
    }
  }

  if (period.kind === 'year') {
    const year = Number(period.value)
    return { from: `${year}-01-01`, to: `${year}-12-31` }
  }

  if (period.kind === 'quarter') {
    const [yearStr, q] = period.value.split('-Q')
    const year = Number(yearStr)
    const startMonth = (Number(q) - 1) * 3 + 1
    const endMonth = startMonth + 2
    return {
      from: `${year}-${pad(startMonth)}-01`,
      to: `${year}-${pad(endMonth)}-${pad(lastDay(year, endMonth))}`,
    }
  }

  const { year, month } = parseMonth(period.value)
  return { from: `${year}-${pad(month)}-01`, to: `${year}-${pad(month)}-${pad(lastDay(year, month))}` }
}

// The month the ledger and the accrual generator act on: whichever month the
// user drilled into, else the last month the period covers.
export function focusMonthFor(period: Period, range: DataRange): string {
  if (period.kind === 'month' && period.value) return period.value
  const window = windowFor(period, range)
  return window.to.slice(0, 7)
}

// Opens on everything there is, which for this firm is eight months of history —
// not on a window relative to today that contains nothing.
export function defaultPeriod(): Period {
  return { kind: 'all', value: '' }
}
