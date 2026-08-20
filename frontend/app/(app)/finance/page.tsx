'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Card } from '@/components/ui/card'
import { Select } from '@/components/ui/select'
import { SectionHeader } from '@/components/ui/section-header'
import { Button } from '@/components/ui/button'
import { Drafts } from './drafts'
import { ExpenseForm, type ExpenseValues } from './expense-form'
import { Ledger } from './ledger'
import { MetricTile } from './metric-tile'
import { OtherIncomePanel, type OtherIncomeValues } from './other-income-panel'
import { PLTable } from './pl-table'
import { RatesPanel } from './rates-panel'
import { RulesPanel, type RuleValues } from './rules-panel'
import { currentMonthKey, money, monthBounds, monthLabel, rangeBack } from './format'
import type { AdvocateRate, Category, Expense, ExpenseList, FinanceMonth, FinanceReport, Generated, OtherIncome, Rule } from './types'

const RANGE_OPTIONS = [
  { months: 6, label: '6 месяцев' },
  { months: 12, label: '12 месяцев' },
  { months: 24, label: '24 месяца' },
]

const EMPTY_MONTH: FinanceMonth = {
  month: '',
  income_consult: 0,
  income_cases: 0,
  income_other: 0,
  income_total: 0,
  expense_by_category: {},
  expense_by_kind: {},
  expense_total: 0,
  balance: 0,
  cumulative: 0,
  marketing_spend: 0,
  direct_cost: 0,
  gross_profit: 0,
  leads: 0,
  new_clients: 0,
  cac: 0,
  cpl: 0,
  romi: 0,
}

export default function FinancePage() {
  const qc = useQueryClient()
  const [monthsBack, setMonthsBack] = useState(12)
  const [activeMonth, setActiveMonth] = useState(currentMonthKey())
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [categoryFilter, setCategoryFilter] = useState('')
  const [editing, setEditing] = useState<Expense | null>(null)
  // bumped after a successful create so the add form remounts empty
  const [formNonce, setFormNonce] = useState(0)
  const formRef = useRef<HTMLDivElement>(null)
  const [showRules, setShowRules] = useState(false)
  const [showRates, setShowRates] = useState(false)
  const [showIncome, setShowIncome] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search.trim()), 300)
    return () => clearTimeout(t)
  }, [search])

  const range = useMemo(() => rangeBack(monthsBack), [monthsBack])

  const report = useQuery({
    queryKey: ['finance-report', range.from, range.to],
    queryFn: () => api<FinanceReport>(`/finance/report?from=${range.from}&to=${range.to}`),
    placeholderData: keepPreviousData,
  })
  // The picked month is clamped to the loaded range: switching 24 months -> 6
  // otherwise leaves activeMonth outside `months`, and every tile reads 0 while
  // the ledger below still lists that month's real expenses.
  const months = report.data?.months ?? []
  const shownMonth =
    months.length === 0 || months.some((m) => m.month === activeMonth) ? activeMonth : (months[months.length - 1]?.month ?? activeMonth)
  const bounds = monthBounds(shownMonth)

  const categories = useQuery({
    queryKey: ['finance-categories'],
    queryFn: () => api<{ items: Category[] }>('/finance/categories'),
  })
  const ledgerParams = new URLSearchParams({ from: bounds.from, to: bounds.to, limit: '200' })
  if (debouncedSearch) ledgerParams.set('search', debouncedSearch)
  if (categoryFilter) ledgerParams.set('category', categoryFilter)
  const expenses = useQuery({
    queryKey: ['finance-expenses', ledgerParams.toString()],
    queryFn: () => api<ExpenseList>(`/finance/expenses?${ledgerParams}`),
    placeholderData: keepPreviousData,
  })
  // Drafts are not scoped to the selected month: a payout generated for last
  // month must not disappear from the confirm list when the month rolls over.
  const drafts = useQuery({
    queryKey: ['finance-drafts'],
    queryFn: () => api<ExpenseList>('/finance/expenses?status=draft&limit=100'),
  })
  const rules = useQuery({
    queryKey: ['finance-rules'],
    queryFn: () => api<{ items: Rule[] }>('/finance/rules'),
  })
  const rates = useQuery({
    queryKey: ['finance-rates'],
    queryFn: () => api<{ items: AdvocateRate[] }>('/finance/advocate-rates'),
  })
  const otherIncome = useQuery({
    queryKey: ['finance-income', bounds.from, bounds.to],
    queryFn: () => api<{ items: OtherIncome[]; sum: number }>(`/finance/income?from=${bounds.from}&to=${bounds.to}`),
  })

  function refreshMoney() {
    qc.invalidateQueries({ queryKey: ['finance-report'] })
    qc.invalidateQueries({ queryKey: ['finance-expenses'] })
    qc.invalidateQueries({ queryKey: ['finance-drafts'] })
  }
  const fail = (e: Error) => toast.error(e.message)

  const createExpense = useMutation({
    mutationFn: (values: ExpenseValues) => api('/finance/expenses', { method: 'POST', body: JSON.stringify(values) }),
    onSuccess: () => {
      refreshMoney()
      setFormNonce((n) => n + 1)
      toast.success('Расход добавлен')
    },
    onError: fail,
  })
  const updateExpense = useMutation({
    mutationFn: ({ id, values }: { id: string; values: ExpenseValues }) =>
      api(`/finance/expenses/${id}`, { method: 'PATCH', body: JSON.stringify(values) }),
    onSuccess: () => {
      refreshMoney()
      setEditing(null)
      toast.success('Сохранено')
    },
    onError: fail,
  })
  const confirmExpense = useMutation({
    mutationFn: (id: string) => api(`/finance/expenses/${id}/confirm`, { method: 'POST' }),
    onSuccess: () => {
      refreshMoney()
      toast.success('Проведено')
    },
    onError: fail,
  })
  const deleteExpense = useMutation({
    mutationFn: (id: string) => api(`/finance/expenses/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      refreshMoney()
      setEditing(null)
      toast.success('Удалено')
    },
    onError: fail,
  })
  const generate = useMutation({
    mutationFn: () => api<Generated>('/finance/generate', { method: 'POST', body: JSON.stringify({ month: shownMonth }) }),
    onSuccess: (result) => {
      refreshMoney()
      // an auto_post template posts straight to the ledger, so "created drafts" would be a lie
      const created = result.recurring + result.payouts
      if (created === 0) {
        toast.success(result.skipped > 0 ? `Всё уже начислено (${result.skipped})` : 'Новых начислений нет')
        return
      }
      toast.success(`Начислено: ${created} (шаблоны ${result.recurring}, выплаты ${result.payouts})`)
    },
    onError: fail,
  })

  const createRule = useMutation({
    mutationFn: (values: RuleValues) => api('/finance/rules', { method: 'POST', body: JSON.stringify(values) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-rules'] })
      toast.success('Шаблон создан')
    },
    onError: fail,
  })
  const updateRule = useMutation({
    mutationFn: ({ id, values }: { id: string; values: RuleValues }) =>
      api(`/finance/rules/${id}`, { method: 'PATCH', body: JSON.stringify(values) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-rules'] })
      toast.success('Шаблон сохранён')
    },
    onError: fail,
  })
  const deleteRule = useMutation({
    mutationFn: (id: string) => api(`/finance/rules/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-rules'] })
      toast.success('Шаблон удалён')
    },
    onError: fail,
  })
  const saveRate = useMutation({
    mutationFn: ({ id, percent }: { id: string; percent: number }) =>
      api(`/finance/advocate-rates/${id}`, { method: 'PATCH', body: JSON.stringify({ commission_percent: percent }) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-rates'] })
      toast.success('Ставка сохранена')
    },
    onError: fail,
  })
  const createIncome = useMutation({
    mutationFn: (values: OtherIncomeValues) => api('/finance/income', { method: 'POST', body: JSON.stringify(values) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-income'] })
      qc.invalidateQueries({ queryKey: ['finance-report'] })
      toast.success('Доход добавлен')
    },
    onError: fail,
  })
  const deleteIncome = useMutation({
    mutationFn: (id: string) => api(`/finance/income/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['finance-income'] })
      qc.invalidateQueries({ queryKey: ['finance-report'] })
      toast.success('Удалено')
    },
    onError: fail,
  })

  const categoryItems = categories.data?.items ?? []
  const activeCategories = categoryItems.filter((c) => c.is_active)
  const current = months.find((m) => m.month === shownMonth) ?? EMPTY_MONTH
  const draftItems = drafts.data?.items ?? []

  function startEdit(expense: Expense) {
    setEditing(expense)
    formRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h1 className='text-xl font-semibold'>Финансы</h1>
        <div className='flex flex-wrap items-center gap-2'>
          <Select value={shownMonth} onChange={(e) => setActiveMonth(e.target.value)} className='w-[150px]'>
            {months.length === 0 && <option value={shownMonth}>{monthLabel(shownMonth)}</option>}
            {[...months].reverse().map((m) => (
              <option key={m.month} value={m.month}>
                {monthLabel(m.month)}
              </option>
            ))}
          </Select>
          <Select value={String(monthsBack)} onChange={(e) => setMonthsBack(Number(e.target.value))} className='w-[140px]'>
            {RANGE_OPTIONS.map((o) => (
              <option key={o.months} value={o.months}>
                {o.label}
              </option>
            ))}
          </Select>
        </div>
      </div>

      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
        <MetricTile label='Доход' value={money(current.income_total)} accent='good' hint={monthLabel(shownMonth)} />
        <MetricTile label='Расход' value={money(current.expense_total)} />
        <MetricTile label='Баланс' value={money(current.balance)} accent={current.balance < 0 ? 'bad' : 'good'} />
        <MetricTile
          label='Нар. итог'
          value={money(current.cumulative)}
          accent={current.cumulative < 0 ? 'bad' : 'good'}
          hint='с начала периода'
        />
        <MetricTile label='CAC' value={current.cac ? money(current.cac) : '—'} hint={`${current.new_clients} новых`} />
        <MetricTile label='ROMI' value={current.marketing_spend > 0 ? current.romi.toFixed(2) + '×' : '—'} hint='на 1 ₴ рекламы' />
      </div>

      <Drafts
        drafts={draftItems}
        month={shownMonth}
        generating={generate.isPending}
        pendingId={confirmExpense.isPending ? (confirmExpense.variables as string) : null}
        busy={confirmExpense.isPending}
        onGenerate={() => generate.mutate()}
        onConfirm={(id) => confirmExpense.mutate(id)}
        onEdit={startEdit}
        onDelete={(expense) => {
          if (window.confirm(`Удалить черновик на ${money(expense.amount)}?`)) deleteExpense.mutate(expense.id)
        }}
      />

      <Card>
        <SectionHeader
          title='P&L по месяцам'
          action={report.isFetching ? <span className='text-xs text-gray-400'>обновление…</span> : undefined}
        />
        {(report.isLoading || categories.isLoading) && <div className='text-sm text-gray-500'>Загрузка…</div>}
        {report.isError && <div className='text-sm text-rose-600'>Не удалось загрузить отчёт.</div>}
        {categories.isError && (
          <div className='text-sm text-rose-600'>Не удалось загрузить категории — разбивка по статьям неполная, итоги ниже верные.</div>
        )}
        {report.data && !categories.isLoading && (
          <PLTable report={report.data} categories={categoryItems} activeMonth={shownMonth} onPickMonth={setActiveMonth} />
        )}
      </Card>

      <Card>
        <div ref={formRef} className='scroll-mt-4' />
        <SectionHeader title={editing ? 'Изменить расход' : `Расходы: ${monthLabel(shownMonth)}`} />
        {editing ? (
          <ExpenseForm
            key={editing.id}
            categories={activeCategories}
            initial={editing}
            submitLabel='Сохранить'
            pending={updateExpense.isPending}
            onSubmit={(values) => updateExpense.mutate({ id: editing.id, values })}
            onCancel={() => setEditing(null)}
          />
        ) : (
          <ExpenseForm
            key={'new-' + formNonce}
            categories={activeCategories}
            submitLabel='Добавить расход'
            pending={createExpense.isPending}
            onSubmit={(values) => createExpense.mutate(values)}
          />
        )}

        <div className='mt-4 border-t border-gray-100 pt-4'>
          <Ledger
            list={expenses.data}
            loading={expenses.isPending}
            fetching={expenses.isFetching}
            categories={categoryItems}
            categoryFilter={categoryFilter}
            search={search}
            onCategoryFilter={setCategoryFilter}
            onSearch={setSearch}
            onEdit={startEdit}
            onDelete={(expense) => {
              if (
                window.confirm(
                  expense.origin === 'manual'
                    ? `Удалить расход на ${money(expense.amount)}?`
                    : `Отменить начисление на ${money(expense.amount)}? Строка останется в истории, но перестанет считаться — и генератор её не создаст заново.`
                )
              )
                deleteExpense.mutate(expense.id)
            }}
          />
        </div>
      </Card>

      <Card>
        <SectionHeader
          title='Автоматические расходы'
          action={
            <Button variant='ghost' size='sm' onClick={() => setShowRules((v) => !v)}>
              {showRules ? 'Свернуть' : `Показать (${rules.data?.items.length ?? 0})`}
            </Button>
          }
        />
        {showRules && (
          <RulesPanel
            rules={rules.data?.items ?? []}
            categories={activeCategories}
            pending={createRule.isPending || updateRule.isPending}
            onCreate={(values) => createRule.mutateAsync(values).catch(() => undefined)}
            onUpdate={(id, values) => updateRule.mutateAsync({ id, values }).catch(() => undefined)}
            onDelete={(rule) => {
              if (window.confirm(`Удалить шаблон «${rule.name}»?`)) deleteRule.mutate(rule.id)
            }}
          />
        )}
      </Card>

      <Card>
        <SectionHeader
          title='Ставки адвокатов'
          action={
            <Button variant='ghost' size='sm' onClick={() => setShowRates((v) => !v)}>
              {showRates ? 'Свернуть' : `Показать (${rates.data?.items.length ?? 0})`}
            </Button>
          }
        />
        {showRates && (
          <RatesPanel
            rates={rates.data?.items ?? []}
            pendingId={saveRate.isPending ? (saveRate.variables?.id ?? null) : null}
            onSave={(id, percent) => saveRate.mutate({ id, percent })}
          />
        )}
      </Card>

      <Card>
        <SectionHeader
          title='Прочие доходы'
          action={
            <Button variant='ghost' size='sm' onClick={() => setShowIncome((v) => !v)}>
              {showIncome ? 'Свернуть' : `Показать (${otherIncome.data?.items.length ?? 0})`}
            </Button>
          }
        />
        {showIncome && (
          <OtherIncomePanel
            items={otherIncome.data?.items ?? []}
            pending={createIncome.isPending}
            onCreate={(values) => createIncome.mutateAsync(values).catch(() => undefined)}
            onDelete={(income) => {
              if (window.confirm(`Удалить доход на ${money(income.amount)}?`)) deleteIncome.mutate(income.id)
            }}
          />
        )}
      </Card>
    </div>
  )
}
