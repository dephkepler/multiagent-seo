export interface MyCasePayment {
  id: string
  amount: number
  paid_at: string
}

export interface MyCase {
  id: string
  client_id: string
  client_name: string
  client_phone: string
  category: string
  status: string
  description: string
  fee: number
  paid: number
  owed: number
  created_at: string
  payments: MyCasePayment[]
}

export interface MyCaseList {
  items: MyCase[]
  total_fee: number
  total_paid: number
  total_owed: number
}

export interface MyClient {
  id: string
  name: string
  phone: string
  cases: number
  fee: number
  paid: number
  owed: number
  last_case_at: string
}

export interface MyClientList {
  items: MyClient[]
}

export interface MyConsultation {
  id: string
  scheduled_at: string
  price: number
  status: string
  case_note: string
}

export interface ClientNote {
  id: string
  text: string
  created_by: string
  created_at: string
}

export interface MyClientCard {
  client: MyClient
  cases: MyCase[]
  consultations: MyConsultation[]
  notes: ClientNote[]
}

export interface MyMonthMoney {
  month: string
  collected: number
  accrued: number
}

export interface MySettlement {
  advocate_id: string
  full_name: string
  commission_percent: number
  collected: number
  accrued: number
  paid: number
  outstanding: number
  months: MyMonthMoney[]
  paid_is_partial: boolean
}

export interface MyStatusCount {
  status: string
  count: number
}

export interface MyStats {
  cases: number
  clients: number
  by_status: MyStatusCount[]
  fee_total: number
  paid_total: number
  client_debt: number
  avg_fee: number
  months: MyMonthMoney[]
  first_case_at?: string
  last_payment_at?: string
}

export const CASE_STATUS_LABEL: Record<string, string> = {
  in_progress: 'В работе',
  completed: 'Завершено',
  cancelled: 'Отменено',
}

// Only these two are an advocate's to set: cancelling a case writes off what
// the client owes, which stays an admin decision (see the API's own refusal).
export const SETTABLE_STATUS = ['in_progress', 'completed'] as const

export const CONSULT_STATUS_LABEL: Record<string, string> = {
  scheduled: 'Запланирована',
  completed: 'Проведена',
  cancelled: 'Отменена',
}
