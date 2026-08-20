'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { dateLabel, money } from './format'
import { ORIGIN_LABEL, PAYMENT_LABEL, type Category, type Expense, type ExpenseList } from './types'

interface Props {
  list?: ExpenseList
  loading: boolean
  fetching: boolean
  categories: Category[]
  categoryFilter: string
  search: string
  onCategoryFilter: (code: string) => void
  onSearch: (value: string) => void
  onEdit: (expense: Expense) => void
  onDelete: (expense: Expense) => void
}

export function Ledger({
  list,
  loading,
  fetching,
  categories,
  categoryFilter,
  search,
  onCategoryFilter,
  onSearch,
  onEdit,
  onDelete,
}: Props) {
  const items = list?.items ?? []

  return (
    <div>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <Input
          value={search}
          onChange={(e) => onSearch(e.target.value)}
          placeholder='Поиск по контрагенту или описанию'
          className='sm:max-w-xs'
        />
        <Select value={categoryFilter} onChange={(e) => onCategoryFilter(e.target.value)} className='sm:max-w-[220px]'>
          <option value=''>Все категории</option>
          {categories.map((c) => (
            <option key={c.code} value={c.code}>
              {c.label}
            </option>
          ))}
        </Select>
        {list && (
          <div className='text-sm text-gray-500 sm:ml-auto'>
            {fetching && <span className='mr-2 text-xs text-gray-400'>обновление…</span>}
            {list.total} записей · проведено <span className='font-medium text-gray-700'>{money(list.sum)}</span>
          </div>
        )}
      </div>

      {loading && <div className='mt-4 text-sm text-gray-500'>Загрузка…</div>}
      {!loading && items.length === 0 && <div className='mt-4 text-sm text-gray-500'>За этот месяц расходов нет.</div>}

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
                  <tr key={e.id} className='border-b border-gray-100 hover:bg-gray-50'>
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

          {list && list.total > items.length && (
            <div className='mt-3 text-xs text-gray-500'>
              Показаны первые {items.length} из {list.total}. Сузьте период или фильтр, чтобы увидеть остальные — сумма выше считается по
              всем {list.total}.
            </div>
          )}
        </div>
      )}
    </div>
  )
}
