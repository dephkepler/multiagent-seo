// The firm's timezone, not the phone's. A client abroad picking "14:00" has to
// mean 14:00 in the office, or they arrive at the wrong hour — and the slot the
// server offered was built in this zone to begin with.
const TZ = 'Europe/Kyiv'

const dayLabel = new Intl.DateTimeFormat('uk-UA', {
  weekday: 'short',
  day: 'numeric',
  month: 'short',
  timeZone: TZ,
})

const timeLabel = new Intl.DateTimeFormat('uk-UA', {
  hour: '2-digit',
  minute: '2-digit',
  timeZone: TZ,
})

const fullLabel = new Intl.DateTimeFormat('uk-UA', {
  day: 'numeric',
  month: 'long',
  hour: '2-digit',
  minute: '2-digit',
  timeZone: TZ,
})

/** Stable key for grouping slots into days, in the firm's zone. */
export function dayKey(iso: string): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: TZ }).format(new Date(iso))
}

export function formatDay(iso: string): string {
  return dayLabel.format(new Date(iso))
}

export function formatTime(iso: string): string {
  return timeLabel.format(new Date(iso))
}

export function formatDateTime(iso: string): string {
  return fullLabel.format(new Date(iso))
}

export const STATUS_LABELS: Record<string, string> = {
  requested: 'Очікує підтвердження',
  scheduled: 'Підтверджено',
  completed: 'Відбулася',
  cancelled: 'Скасовано',
  no_show: 'Не відбулася',
}
