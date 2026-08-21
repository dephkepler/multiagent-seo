'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { dateLabel, money } from './format'
import {
  ORIGIN_LABEL,
  PAYMENT_LABEL,
  type Category,
  type Expense,
  type ExpenseList,
  type ExpenseOrigin,
  type ExpenseStatus,
  type PaymentMethod,
} from './types'

export interface LedgerFilters {
  // 'month' is the picked month, 'period' the whole report window — asking "what
  // did we spend on ads all year" was impossible while the ledger was pinned to
  // one month.
  scope: 'month' | 'period'
  category: string
  status: ExpenseStatus | ''
  method: PaymentMethod | ''
  origin: ExpenseOrigin | ''
  search: string
  page: number
}

export const emptyFilters: LedgerFilters = {
  scope: 'month',
  category: '',
  status: '',
  method: '',
  origin: '',
  search: '',
  page: 1,
}

export const LEDGER_PAGE_SIZE = 50

interface Props {
  list?: ExpenseList
  loading: boolean
  fetching: boolean
  categories: Category[]
  filters: LedgerFilters
  periodLabel: string
  monthLabel: string
  onChange: (next: Partial<LedgerFilters>) => void
  onReset: () => void
  onEdit: (expense: Expense) => void
  onDelete: (expense: Expense) => void
}

export function Ledger({
  list,
  loading,
  fetching,
  categories,
  filters,
  periodLabel,
  monthLabel,
  onChange,
  onReset,
  onEdit,
  onDelete,
}: Props) {
  const items = list?.items ?? []
  const active =
    (filters.category ? 1 : 0) + (filters.status ? 1 : 0) + (filters.method ? 1 : 0) + (filters.origin ? 1 : 0) + (filters.search ? 1 : 0)
  const pages = list ? Math.max(1, Math.ceil(list.total / LEDGER_PAGE_SIZE)) : 1

  return (
    <div>
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
        <Input
          value={filters.search}
          onChange={(e) => onChange({ search: e.target.value })}
          placeholder='Поиск по контрагенту или описанию'
        />
        <Select value={filters.scope} onChange={(e) => onChange({ scope: e.target.value as LedgerFilters['scope'] })}>
          <option value='month'>{monthLabel}</option>
          <option value='period'>{periodLabel}</option>
        </Select>
        <Select value={filters.category} onChange={(e) => onChange({ category: e.target.value })}>
          <option value=''>Все статьи</option>
          {categories.map((c) => (
            <option key={c.code} value={c.code}>
              {c.label}
            </option>
          ))}
        </Select>
        <Select value={filters.status} onChange={(e) => onChange({ status: e.target.value as LedgerFilters['status'] })}>
          <option value=''>Любой статус</option>
          <option value='posted'>Проведённые</option>
          <option value='draft'>Черновики</option>
          <option value='void'>Отменённые</option>
        </Select>
        <Select value={filters.method} onChange={(e) => onChange({ method: e.target.value as LedgerFilters['method'] })}>
          <option value=''>Любая оплата</option>
          {(Object.keys(PAYMENT_LABEL) as PaymentMethod[]).map((m) => (
            <option key={m} value={m}>
              {PAYMENT_LABEL[m]}
            </option>
          ))}
        </Select>
        <Select value={filters.origin} onChange={(e) => onChange({ origin: e.target.value as LedgerFilters['origin'] })}>
          <option value=''>Любое происхождение</option>
          {(Object.keys(ORIGIN_LABEL) as ExpenseOrigin[]).map((o) => (
            <option key={o} value={o}>
              {ORIGIN_LABEL[o]}
            </option>
          ))}
        </Select>
        <div className='flex items-center gap-2 sm:col-span-2'>
          {active > 0 && (
            <Button variant='secondary' size='sm' onClick={onReset}>
              Сбросить ({active})
            </Button>
          )}
          {list && (
            <div className='ml-auto text-sm text-gray-500'>
              {fetching && <span className='mr-2 text-xs text-gray-400'>обновление…</span>}
              {list.total} записей · проведено <span className='font-medium text-gray-700'>{money(list.sum)}</span>
            </div>
          )}
        </div>
      </div>

      {loading && <div className='mt-4 text-sm text-gray-500'>Загрузка…</div>}
      {!loading && items.length === 0 && (
        <div className='mt-4 text-sm text-gray-500'>
          {/* "за этот месяц" was hardcoded regardless of scope/filters — false claim when
              viewing the whole period, or when filters (not the month) are why the list is empty. */}
          {active > 0
            ? 'По выбранным фильтрам ничего не найдено.'
            : filters.scope === 'period'
              ? 'За этот период расходов нет.'
              : 'За этот месяц расходов нет.'}
        </div>
      )}

      {items.length > 0 && (
        <div className={fetching ? 'opacity-50 transition-opacity' : undefined}>
          {/* mobile: cards — a 7-column table on a phone is unreadable even with horizontal scroll */}
          <ul className='mt-3 grid gap-2 md:hidden'>
            {items.map((e) => (
              <li key={e.id} className='rounded-md border border-gray-200 bg-white p-3'>
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <div className='truncate font-medium'>{e.vendor || e.description || e.category_label}</div>
                    {e.description && e.vendor && <div className='mt-0.5 truncate text-xs text-gray-500'>{e.description}</div>}
                  </div>
                  <div className='shrink-0 text-right font-semibold tabular-nums'>{money(e.amount)}</div>
                </div>
                <div className='mt-1.5 flex flex-wrap items-center gap-1'>
                  <span className='text-xs text-gray-400'>{dateLabel(e.spent_at)}</span>
                  <Badge variant='neutral'>{e.category_label}</Badge>
                  <Badge variant='neutral'>{PAYMENT_LABEL[e.payment_method]}</Badge>
                  {e.status === 'draft' && <Badge variant='warning'>Черновик</Badge>}
                  {e.status === 'void' && <Badge variant='danger'>Отменён</Badge>}
                  {e.origin !== 'manual' && <Badge variant='info'>{ORIGIN_LABEL[e.origin]}</Badge>}
                </div>
                <div className='mt-2 flex gap-2'>
                  <Button size='sm' variant='secondary' onClick={() => onEdit(e)} className='flex-1'>
                    Изменить
                  </Button>
                  <Button size='sm' variant='ghost' className='flex-1 text-rose-600' onClick={() => onDelete(e)}>
                    Удалить
                  </Button>
                </div>
              </li>
            ))}
          </ul>

          <div className='mt-3 hidden overflow-x-auto md:block'>
            <table className='w-full text-sm'>
              <thead>
                <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
                  <th className='py-2 pr-3 font-medium'>Дата</th>
                  <th className='py-2 pr-3 font-medium'>Контрагент</th>
                  <th className='py-2 pr-3 font-medium'>Описание</th>
                  <th className='py-2 pr-3 font-medium'>Категория</th>
                  <th className='py-2 pr-3 font-medium'>Оплата</th>
                  <th className='py-2 pr-3 text-right font-medium'>Сумма</th>
                  <th className='py-2 font-medium'></th>
                </tr>
              </thead>
              <tbody>
                {items.map((e) => (
                  <tr key={e.id} className='border-b border-gray-100 hover:bg-emerald-50/70'>
                    <td className='py-2 pr-3 whitespace-nowrap text-gray-500'>{dateLabel(e.spent_at)}</td>
                    <td className='py-2 pr-3'>{e.vendor || '—'}</td>
                    <td className='max-w-[280px] truncate py-2 pr-3 text-gray-600'>{e.description || '—'}</td>
                    <td className='py-2 pr-3'>
                      <div className='flex flex-wrap items-center gap-1'>
                        <Badge variant='neutral'>{e.category_label}</Badge>
                        {e.status === 'draft' && <Badge variant='warning'>Черновик</Badge>}
                        {e.status === 'void' && <Badge variant='danger'>Отменён</Badge>}
                        {e.origin !== 'manual' && <Badge variant='info'>{ORIGIN_LABEL[e.origin]}</Badge>}
                      </div>
                    </td>
                    <td className='py-2 pr-3 whitespace-nowrap text-gray-500'>{PAYMENT_LABEL[e.payment_method]}</td>
                    <td className='py-2 pr-3 text-right font-medium tabular-nums whitespace-nowrap'>{money(e.amount)}</td>
                    <td className='py-2 text-right whitespace-nowrap'>
                      <Button size='sm' variant='ghost' onClick={() => onEdit(e)}>
                        Изменить
                      </Button>
                      <Button size='sm' variant='ghost' className='text-rose-600' onClick={() => onDelete(e)}>
                        Удалить
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {list && pages > 1 && (
            <div className='mt-3 flex flex-wrap items-center gap-2'>
              <Button variant='secondary' size='sm' disabled={filters.page <= 1} onClick={() => onChange({ page: filters.page - 1 })}>
                Назад
              </Button>
              <span className='text-sm text-gray-500'>
                стр. {filters.page} из {pages}
              </span>
              <Button variant='secondary' size='sm' disabled={filters.page >= pages} onClick={() => onChange({ page: filters.page + 1 })}>
                Дальше
              </Button>
              <span className='text-xs text-gray-400'>сумма считается по всем {list.total} записям</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
