const MONTH_NAME = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь']

export function money(n: number): string {
  return Math.round(n).toLocaleString('ru-RU') + ' ₴'
}

// Compact form for table cells, where a full "1 234 567 ₴" per column stops fitting.
export function moneyShort(n: number): string {
  if (n === 0) return '—'
  const abs = Math.abs(n)
  if (abs >= 100000) return (n / 1000).toFixed(0) + 'k'
  return Math.round(n).toLocaleString('ru-RU')
}

// Ratios cross the wire as 0 when their denominator was 0 — an undefined value,
// not a real zero — so the caller decides when to show a dash instead.
export function percent(n: number, undefinedWhen = false): string {
  return undefinedWhen ? '—' : (n * 100).toFixed(0) + '%'
}

export function times(n: number, undefinedWhen = false): string {
  return undefinedWhen ? '—' : n.toFixed(2) + '×'
}

export function monthLabel(key: string): string {
  if (key === 'total') return 'Итого'
  const [year, month] = key.split('-')
  const idx = Number(month) - 1
  if (!MONTH_NAME[idx]) return key
  return `${MONTH_NAME[idx]} ${year.slice(2)}`
}

// Parsed by hand: new Date('2026-08-01') is UTC midnight, which renders as the
// previous day in any negative-offset timezone.
export function dateLabel(iso: string): string {
  const [, month, day] = iso.split('-')
  return month && day ? `${day}.${month}` : '—'
}

export function currentMonthKey(): string {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

export function monthBounds(monthKey: string): { from: string; to: string } {
  const [year, month] = monthKey.split('-').map(Number)
  const last = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return {
    from: `${monthKey}-01`,
    to: `${monthKey}-${String(last).padStart(2, '0')}`,
  }
}

// Report range: `count` months back from (and including) the current month.
export function rangeBack(count: number): { from: string; to: string } {
  const now = new Date()
  const start = new Date(Date.UTC(now.getFullYear(), now.getMonth() - (count - 1), 1))
  const end = new Date(Date.UTC(now.getFullYear(), now.getMonth() + 1, 0))
  return { from: iso(start), to: iso(end) }
}

// Local, not toISOString(): before 03:00 Kyiv the UTC date is still yesterday,
// and a new expense would be filed into the previous day — or previous month.
export function todayISO(): string {
  const now = new Date()
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}

function iso(d: Date): string {
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}
