'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Section } from './section'
import { PeoplePanel } from './people-panel'
import { SettlementPanel, type SettlementData } from './settlement-panel'
import { SectionHeader } from '@/components/ui/section-header'
import { Select } from '@/components/ui/select'
import { Drafts } from './drafts'
import { ExpenseForm, type ExpenseValues } from './expense-form'
import { Ledger, LEDGER_PAGE_SIZE, emptyFilters, type LedgerFilters } from './ledger'
import { StatTile } from '@/components/ui/stat-tile'
import { OtherIncomePanel, type OtherIncomeValues } from './other-income-panel'
import { PLTable } from './pl-table'
import { RatesPanel } from './rates-panel'
import { RulesPanel, type RuleValues } from './rules-panel'
import { money, monthBounds, monthLabel, percent, times } from './format'
import {
  KIND_LABEL,
  defaultPeriod,
  focusMonthFor,
  optionsFor,
  periodLabel,
  windowFor,
  type DataRange,
  type Period,
  type PeriodKind,
} from './period'
import type { AdvocateRate, Category, Expense, ExpenseList, FinanceMonth, FinanceReport, Generated, OtherIncome, Rule } from './types'

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
  cohort_payers: 0,
  paying_clients: 0,
  consult_count: 0,
  case_payment_count: 0,
  cac: 0,
  cpl: 0,
  romi: 0,
  avg_consult_ticket: 0,
  avg_case_ticket: 0,
  margin_percent: 0,
  marketing_share: 0,
  revenue_per_client: 0,
  ltv: 0,
  ltv_to_cac: 0,
  lead_to_consult: 0,
  break_even_consults: 0,
  income_growth: 0,
}

export default function FinancePage() {
  const qc = useQueryClient()
  const [period, setPeriod] = useState<Period>(defaultPeriod())
  // A month the user drilled into by clicking a column; null means the tiles
  // show the whole period, which is what a P&L should open with.
  const [drillMonth, setDrillMonth] = useState<string | null>(null)
  const [filters, setFilters] = useState<LedgerFilters>(emptyFilters)
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [editing, setEditing] = useState<Expense | null>(null)
  // bumped after a successful create so the add form remounts empty
  const [formNonce, setFormNonce] = useState(0)
  const formRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(filters.search.trim()), 300)
    return () => clearTimeout(t)
  }, [filters.search])

  // Any filter change but paging itself returns to the first page — page 4 of a
  // freshly narrowed list is usually empty.
  function changeFilters(next: Partial<LedgerFilters>) {
    setFilters((prev) => ({ ...prev, ...next, page: next.page ?? 1 }))
  }

  // The offered periods come from the data itself; a window relative to today
  // showed twelve empty columns with the running total repeated in each.
  const dataRange = useQuery({
    queryKey: ['finance-period'],
    queryFn: () => api<DataRange>('/finance/period'),
  })
  const available: DataRange = useMemo(() => dataRange.data ?? { has_data: false }, [dataRange.data])
  const range = useMemo(() => windowFor(period, available), [period, available])

  const report = useQuery({
    queryKey: ['finance-report', range.from, range.to],
    queryFn: () => api<FinanceReport>(`/finance/report?from=${range.from}&to=${range.to}`),
    placeholderData: keepPreviousData,
  })
  const months = report.data?.months ?? []
  // A drill-down survives only while the period still contains that month.
  const shownMonth = drillMonth && months.some((m) => m.month === drillMonth) ? drillMonth : focusMonthFor(period, available)
  const bounds = monthBounds(shownMonth)

  const categories = useQuery({
    queryKey: ['finance-categories'],
    queryFn: () => api<{ items: Category[] }>('/finance/categories'),
  })
  const ledgerWindow = filters.scope === 'period' ? range : bounds
  const ledgerParams = new URLSearchParams({
    from: ledgerWindow.from,
    to: ledgerWindow.to,
    limit: String(LEDGER_PAGE_SIZE),
    offset: String((filters.page - 1) * LEDGER_PAGE_SIZE),
  })
  if (debouncedSearch) ledgerParams.set('search', debouncedSearch)
  if (filters.category) ledgerParams.set('category', filters.category)
  if (filters.status) ledgerParams.set('status', filters.status)
  if (filters.origin) ledgerParams.set('origin', filters.origin)
  if (filters.method) ledgerParams.set('payment_method', filters.method)
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
  const settlement = useQuery({
    queryKey: ['finance-settlement', range.from, range.to],
    queryFn: () => api<SettlementData>(`/finance/settlement?from=${range.from}&to=${range.to}`),
    placeholderData: keepPreviousData,
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
  // Tiles follow the drill-down when there is one, otherwise they show the whole
  // period — the number a person opens this page for.
  const tileSource = drillMonth ? months.find((m) => m.month === drillMonth) : report.data?.total
  const current = tileSource ?? EMPTY_MONTH
  const draftItems = drafts.data?.items ?? []

  // the same money the P&L splits by purpose, summed by who received it
  const peopleTotal = categoryItems.filter((c) => c.is_people_pay).reduce((sum, c) => sum + (current.expense_by_category[c.code] ?? 0), 0)

  function startEdit(expense: Expense) {
    setEditing(expense)
    formRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className='space-y-4'>
      <SectionHeader
        title='Финансы'
        as='h1'
        action={
          <div className='flex flex-wrap items-center gap-2'>
            <Select
              value={period.kind}
              disabled={dataRange.isLoading || dataRange.isError}
              onChange={(e) => {
                const kind = e.target.value as PeriodKind
                // Jump to the newest option of the new kind, which for this data is
                // the most recent month/quarter/year that actually has rows.
                const first = optionsFor(kind, available)[0]?.value ?? ''
                setPeriod({ kind, value: first })
                setDrillMonth(null)
              }}
              className='w-[130px]'
            >
              {(Object.keys(KIND_LABEL) as PeriodKind[]).map((k) => (
                <option key={k} value={k}>
                  {KIND_LABEL[k]}
                </option>
              ))}
            </Select>
            {period.kind !== 'all' && (
              <Select
                value={period.value}
                disabled={dataRange.isLoading || dataRange.isError}
                onChange={(e) => {
                  setPeriod({ kind: period.kind, value: e.target.value })
                  setDrillMonth(null)
                }}
                className='w-[170px]'
              >
                {optionsFor(period.kind, available).map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            )}
            <span className='text-sm text-gray-500'>{periodLabel(period, available)}</span>
          </div>
        }
      />

      {dataRange.isLoading && <div className='text-sm text-gray-500'>Загрузка периода…</div>}
      {dataRange.isError && (
        <div className='text-sm text-rose-600'>Не удалось загрузить доступный период — выбор периода может быть неполным.</div>
      )}

      {report.isLoading ? (
        <div className='text-sm text-gray-500'>Загрузка…</div>
      ) : (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5'>
          <StatTile
            label='Доход'
            value={money(current.income_total)}
            accent='good'
            hint={drillMonth ? monthLabel(shownMonth) : periodLabel(period, available)}
          />
          <StatTile label='Расход' value={money(current.expense_total)} />
          <StatTile label='Баланс' value={money(current.balance)} accent={current.balance < 0 ? 'bad' : 'good'} />
          <StatTile
            label='Нар. итог'
            value={money(current.cumulative)}
            accent={current.cumulative < 0 ? 'bad' : 'good'}
            hint='на конец месяца, за всё время'
          />
          <StatTile
            label='CAC'
            value={current.cac ? money(current.cac) : '—'}
            hint={`${current.cohort_payers} из ${current.new_clients} заплатили`}
          />
          <StatTile label='ROMI' value={times(current.romi, current.marketing_spend === 0)} hint='на 1 ₴ рекламы' />
          <StatTile
            label='Маржа'
            value={percent(current.margin_percent, current.income_total === 0)}
            accent={current.margin_percent < 0 ? 'bad' : 'good'}
            hint='баланс / доход'
          />
          <StatTile
            label='Средний чек'
            value={current.avg_consult_ticket ? money(current.avg_consult_ticket) : '—'}
            hint={`${current.consult_count} консультаций`}
          />
          <StatTile
            label='LTV / CAC'
            value={times(current.ltv_to_cac, current.cac === 0)}
            accent={current.ltv_to_cac > 0 && current.ltv_to_cac < 1 ? 'bad' : undefined}
            hint={current.ltv ? `LTV ${money(current.ltv)}` : '<1 — клиент не окупается'}
          />
          <StatTile
            label='Дебиторка'
            value={report.data?.receivable ? money(report.data.receivable) : '—'}
            accent={report.data?.receivable ? 'bad' : undefined}
            hint='долг по делам, за всё время'
          />
        </div>
      )}

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

      <Section
        title='P&L по месяцам'
        summary={
          <>
            {report.isFetching && <span className='text-xs text-gray-400'>обновление…</span>}
            <span className='text-xs text-gray-500'>баланс за период</span>
            <span
              className={report.data && report.data.total.balance < 0 ? 'font-semibold text-rose-700' : 'font-semibold text-emerald-700'}
            >
              {money(report.data?.total.balance ?? 0)}
            </span>
          </>
        }
      >
        {(report.isLoading || categories.isLoading) && <div className='text-sm text-gray-500'>Загрузка…</div>}
        {report.isError && <div className='text-sm text-rose-600'>Не удалось загрузить отчёт.</div>}
        {categories.isError && (
          <div className='text-sm text-rose-600'>Не удалось загрузить категории — разбивка по статьям неполная, итоги ниже верные.</div>
        )}
        {report.data && !categories.isLoading && (
          <PLTable
            report={report.data}
            categories={categoryItems}
            activeMonth={drillMonth ?? ''}
            onPickMonth={(month) => setDrillMonth((prev) => (prev === month ? null : month))}
          />
        )}
      </Section>

      <Section
        title='Расходы на людей'
        summary={
          <>
            <span className='text-xs text-gray-500'>{drillMonth ? monthLabel(shownMonth) : 'за период'}</span>
            <span className='font-semibold text-gray-800'>{money(peopleTotal)}</span>
            <span className='text-xs text-gray-500'>
              {percent(peopleTotal / (current.expense_total || 1), current.expense_total === 0)} расходов
            </span>
          </>
        }
        defaultOpen={false}
      >
        <PeoplePanel
          categories={categoryItems}
          month={current}
          monthLabel={drillMonth ? monthLabel(shownMonth) : periodLabel(period, available)}
        />
      </Section>

      <Section
        title='Расчёты с адвокатами'
        summary={
          <>
            <span className='text-xs text-gray-500'>осталось отдать</span>
            <span
              className={
                settlement.data && settlement.data.total_outstanding > 0 ? 'font-semibold text-rose-700' : 'font-semibold text-gray-700'
              }
            >
              {money(settlement.data?.total_outstanding ?? 0)}
            </span>
          </>
        }
        defaultOpen={false}
      >
        <SettlementPanel data={settlement.data} loading={settlement.isPending} />
      </Section>

      <Section
        title={editing ? 'Изменить расход' : 'Расходы'}
        summary={
          <>
            <span className='text-xs text-gray-500'>проведено за период</span>
            <span className='font-semibold text-gray-800'>{money(report.data?.total.expense_total ?? 0)}</span>
          </>
        }
      >
        <div ref={formRef} className='scroll-mt-4' />
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
            filters={filters}
            monthLabel={monthLabel(shownMonth)}
            periodLabel={periodLabel(period, available)}
            onChange={changeFilters}
            onReset={() => setFilters(emptyFilters)}
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
      </Section>

      <Section
        title='Автоматические расходы'
        defaultOpen={false}
        summary={
          <>
            <span className='text-xs text-gray-500'>шаблонов</span>
            <span className='font-semibold text-gray-800'>{rules.data?.items.length ?? 0}</span>
            {(rules.data?.items.length ?? 0) > 0 && (
              <span className='text-xs text-gray-500'>
                на {money((rules.data?.items ?? []).filter((r) => r.is_active).reduce((sum, r) => sum + r.amount, 0))} в месяц
              </span>
            )}
          </>
        }
      >
        {
          <RulesPanel
            rules={rules.data?.items ?? []}
            loading={rules.isLoading}
            categories={activeCategories}
            pending={createRule.isPending || updateRule.isPending}
            onCreate={(values) => createRule.mutateAsync(values).catch(() => undefined)}
            onUpdate={(id, values) => updateRule.mutateAsync({ id, values }).catch(() => undefined)}
            onDelete={(rule) => {
              if (window.confirm(`Удалить шаблон «${rule.name}»?`)) deleteRule.mutate(rule.id)
            }}
          />
        }
      </Section>

      <Section
        title='Ставки адвокатов'
        defaultOpen={false}
        summary={
          <>
            <span className='text-xs text-gray-500'>со ставкой</span>
            <span className='font-semibold text-gray-800'>
              {(rates.data?.items ?? []).filter((r) => r.commission_percent > 0).length} из {rates.data?.items.length ?? 0}
            </span>
          </>
        }
      >
        {
          <RatesPanel
            rates={rates.data?.items ?? []}
            loading={rates.isLoading}
            pendingId={saveRate.isPending ? (saveRate.variables?.id ?? null) : null}
            onSave={(id, percent) => saveRate.mutate({ id, percent })}
          />
        }
      </Section>

      <Section
        title='Прочие доходы'
        defaultOpen={false}
        summary={
          <>
            <span className='text-xs text-gray-500'>за период</span>
            <span className='font-semibold text-gray-800'>{money(report.data?.total.income_other ?? 0)}</span>
          </>
        }
      >
        {
          <OtherIncomePanel
            items={otherIncome.data?.items ?? []}
            loading={otherIncome.isLoading}
            pending={createIncome.isPending}
            onCreate={(values) => createIncome.mutateAsync(values).catch(() => undefined)}
            onDelete={(income) => {
              if (window.confirm(`Удалить доход на ${money(income.amount)}?`)) deleteIncome.mutate(income.id)
            }}
          />
        }
      </Section>
    </div>
  )
}
