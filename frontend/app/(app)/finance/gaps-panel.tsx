'use client'

import { Badge } from '@/components/ui/badge'
import { money } from './format'

export interface DataGap {
  kind: string
  count: number
  amount: number
}

// Each gap is a to-do, so each carries what to do about it. The wording lives
// here and not in the backend on purpose: the query counts rows, a person needs
// to know which conversation to have.
const GAPS: Record<string, { label: string; advice: string; tone: 'bad' | 'warn' | 'info' }> = {
  unresolved_consultations: {
    label: 'Записаны, но не закрыты',
    advice:
      'Цена согласована, консультация ни проведена, ни отменена. Поставь статус — деньги либо появятся в доходе, либо честно уйдут в потери.',
    tone: 'bad',
  },
  no_show_priced: {
    label: 'Неявки с согласованной ценой',
    advice: 'В оферте есть ответственность за неявку — по этим суммам можно попробовать взыскать. В доход не идут.',
    tone: 'warn',
  },
  cancelled_priced: {
    label: 'Отменённые с согласованной ценой',
    advice: 'Встреча не состоялась, в доход не идёт. Это справка о потерянном спросе, а не пропавшие деньги.',
    tone: 'info',
  },
  zero_priced_completed: {
    label: 'Проведены без цены',
    advice: 'Портят средний чек и точку безубыточности: либо проставь цену, либо смени статус.',
    tone: 'warn',
  },
  future_consultations: {
    label: 'Датированы будущим',
    advice: 'Запись с датой вперёд добавит в отчёт колонку того года, как только её отметят проведённой.',
    tone: 'warn',
  },
  unlinked_cases: {
    label: 'Дела без адвоката из ростера',
    advice: 'Выплата по проценту для них не считается — привяжи адвоката или заведи его в ростер под тем же именем.',
    tone: 'warn',
  },
  duplicate_advocates: {
    label: 'Похоже, дубль в ростере адвокатов',
    advice: 'Имя одного адвоката содержится в имени другого — выплаты одного человека расщепляются на две строки.',
    tone: 'warn',
  },
}

const TONE: Record<'bad' | 'warn' | 'info', 'danger' | 'warning' | 'neutral'> = {
  bad: 'danger',
  warn: 'warning',
  info: 'neutral',
}

export function gapsAtStake(items: DataGap[]): number {
  // Cancelled consultations are not money at stake — the meeting did not happen.
  return items.filter((g) => g.kind !== 'cancelled_priced').reduce((sum, g) => sum + g.amount, 0)
}

interface Props {
  items: DataGap[]
  loading: boolean
}

export function GapsPanel({ items, loading }: Props) {
  if (loading) return <div className='text-sm text-gray-500'>Загрузка…</div>
  if (items.length === 0) {
    return <div className='text-sm text-gray-500'>Ничего не висит — все записи закрыты статусами.</div>
  }

  const sorted = [...items].sort((a, b) => b.amount - a.amount || b.count - a.count)

  return (
    <ul className='grid gap-2'>
      {sorted.map((gap) => {
        const meta = GAPS[gap.kind] ?? { label: gap.kind, advice: '', tone: 'info' as const }
        return (
          <li key={gap.kind} className='rounded-md border border-gray-200 bg-white p-3 hover:border-emerald-300 hover:bg-emerald-50/40'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div className='flex items-center gap-2'>
                <Badge variant={TONE[meta.tone]}>{gap.count}</Badge>
                <span className='font-medium'>{meta.label}</span>
              </div>
              <span className='font-semibold tabular-nums'>
                {gap.amount ? money(gap.amount) : <span className='text-gray-400'>сумма неизвестна</span>}
              </span>
            </div>
            {meta.advice && <div className='mt-1 text-xs text-gray-500'>{meta.advice}</div>}
          </li>
        )
      })}
    </ul>
  )
}
