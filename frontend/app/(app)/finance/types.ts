export type ExpenseKind = 'marketing' | 'direct' | 'payroll' | 'development' | 'infra' | 'admin'
export type PaymentMethod = 'card' | 'invoice' | 'company' | 'cash'
export type ExpenseStatus = 'draft' | 'posted' | 'void'
export type ExpenseOrigin = 'manual' | 'recurring' | 'derived' | 'imported'

export interface Expense {
  id: string
  spent_at: string
  amount: number
  category_code: string
  category_label: string
  payment_method: PaymentMethod
  vendor: string
  description: string
  status: ExpenseStatus
  origin: ExpenseOrigin
  rule_id?: string
  created_by?: string
  created_at: string
}

export interface ExpenseList {
  items: Expense[]
  total: number
  sum: number
}

export interface Category {
  code: string
  label: string
  kind: ExpenseKind
  // money paid to a person for work — cuts across kind, see the people panel
  is_people_pay: boolean
  is_active: boolean
  sort_order: number
}

export interface Rule {
  id: string
  name: string
  category_code: string
  category_label: string
  vendor: string
  payment_method: PaymentMethod
  amount: number
  day_of_month: number
  auto_post: boolean
  active_from: string
  active_to?: string
  is_active: boolean
}

export interface OtherIncome {
  id: string
  received_at: string
  amount: number
  source: string
  description: string
  created_at: string
}

export interface FinanceMonth {
  month: string
  income_consult: number
  income_cases: number
  income_other: number
  income_total: number
  expense_by_category: Record<string, number>
  expense_by_kind: Record<string, number>
  expense_total: number
  balance: number
  cumulative: number
  marketing_spend: number
  direct_cost: number
  gross_profit: number
  leads: number
  new_clients: number
  cohort_payers: number
  paying_clients: number
  consult_count: number
  case_payment_count: number
  cac: number
  cpl: number
  romi: number
  avg_consult_ticket: number
  avg_case_ticket: number
  margin_percent: number
  marketing_share: number
  revenue_per_client: number
  ltv: number
  ltv_to_cac: number
  lead_to_consult: number
  break_even_consults: number
  income_growth: number
}

export interface FinanceReport {
  months: FinanceMonth[]
  total: FinanceMonth
  receivable: number
}

export interface AdvocateRate {
  advocate_id: string
  full_name: string
  is_active: boolean
  commission_percent: number
}

export interface Generated {
  month: string
  recurring: number
  payouts: number
  skipped: number
}

export const KIND_LABEL: Record<ExpenseKind, string> = {
  marketing: 'Маркетинг',
  direct: 'Себестоимость',
  payroll: 'Зарплаты',
  development: 'Разработка',
  infra: 'Инфраструктура',
  admin: 'Административные',
}

export const PAYMENT_LABEL: Record<PaymentMethod, string> = {
  card: 'Карта',
  invoice: 'Счёт',
  company: 'От компании',
  cash: 'Наличные',
}

export const ORIGIN_LABEL: Record<ExpenseOrigin, string> = {
  manual: 'Вручную',
  recurring: 'По шаблону',
  derived: 'Расчёт по CRM',
  imported: 'Импорт',
}
