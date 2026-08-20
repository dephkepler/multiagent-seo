package finance

// MonthFacts is one month as the repository reads it: income straight from
// consultations/case_payments/other_income, expenses already summed per
// category, plus the two counters the acquisition metrics need.
type MonthFacts struct {
	Month             string // MonthKey format, "2026-08"
	ConsultRevenue    float64
	CasePaid          float64
	OtherIncome       float64
	ExpenseByCategory map[string]float64
	Leads             int
	// clients whose first-ever lead landed in this month — the CAC denominator
	NewClients int
}

type MonthReport struct {
	Month         string
	IncomeConsult float64
	IncomeCases   float64
	IncomeOther   float64
	IncomeTotal   float64

	ExpenseByCategory map[string]float64
	ExpenseByKind     map[Kind]float64
	ExpenseTotal      float64

	Balance float64
	// running balance across the whole report — the spreadsheet's "Нар. итог"
	Cumulative float64

	MarketingSpend float64
	DirectCost     float64
	GrossProfit    float64

	Leads      int
	NewClients int
	// 0 when the denominator is 0 — an undefined ratio is not a real zero, but
	// the alternative (NaN/Inf) crosses JSON as null and breaks the page.
	CAC  float64
	CPL  float64
	ROMI float64
}

type Report struct {
	Months []MonthReport
	// Total is the "Итого" column: sums for money, and ratios recomputed from
	// those sums — averaging per-month CAC/ROMI would weight a quiet month the
	// same as a loud one.
	Total MonthReport
}

// BuildReport expects facts in ascending month order. startingBalance carries
// the cumulative result of everything before the first month, so a report
// that starts mid-history still shows a true running total.
func BuildReport(facts []MonthFacts, categories []Category, startingBalance float64) Report {
	kindOf := make(map[string]Kind, len(categories))
	for _, c := range categories {
		kindOf[c.Code] = c.Kind
	}

	months := make([]MonthReport, 0, len(facts))
	cumulative := startingBalance
	total := MonthReport{
		Month:             "total",
		ExpenseByCategory: map[string]float64{},
		ExpenseByKind:     map[Kind]float64{},
	}

	for _, f := range facts {
		m := MonthReport{
			Month:             f.Month,
			IncomeConsult:     f.ConsultRevenue,
			IncomeCases:       f.CasePaid,
			IncomeOther:       f.OtherIncome,
			ExpenseByCategory: make(map[string]float64, len(f.ExpenseByCategory)),
			ExpenseByKind:     map[Kind]float64{},
			Leads:             f.Leads,
			NewClients:        f.NewClients,
		}
		m.IncomeTotal = roundMoney(f.ConsultRevenue + f.CasePaid + f.OtherIncome)

		for code, amount := range f.ExpenseByCategory {
			m.ExpenseByCategory[code] = roundMoney(amount)
			m.ExpenseTotal += amount
			kind := kindOf[code]
			m.ExpenseByKind[kind] += amount

			total.ExpenseByCategory[code] += amount
			total.ExpenseByKind[kind] += amount
		}
		m.ExpenseTotal = roundMoney(m.ExpenseTotal)
		m.MarketingSpend = roundMoney(m.ExpenseByKind[KindMarketing])
		m.DirectCost = roundMoney(m.ExpenseByKind[KindDirect])
		m.GrossProfit = roundMoney(m.IncomeTotal - m.DirectCost)

		m.Balance = roundMoney(m.IncomeTotal - m.ExpenseTotal)
		cumulative = roundMoney(cumulative + m.Balance)
		m.Cumulative = cumulative

		m.CAC = ratio(m.MarketingSpend, float64(m.NewClients))
		m.CPL = ratio(m.MarketingSpend, float64(m.Leads))
		m.ROMI = ratio(m.IncomeTotal-m.MarketingSpend, m.MarketingSpend)

		total.IncomeConsult += f.ConsultRevenue
		total.IncomeCases += f.CasePaid
		total.IncomeOther += f.OtherIncome
		total.ExpenseTotal += m.ExpenseTotal
		total.Leads += f.Leads
		total.NewClients += f.NewClients

		months = append(months, m)
	}

	// the maps accumulate raw floats, so they need the same rounding as the
	// scalars beside them or expense_by_kind reaches the API as 1234.5600000000002
	for code, amount := range total.ExpenseByCategory {
		total.ExpenseByCategory[code] = roundMoney(amount)
	}
	for kind, amount := range total.ExpenseByKind {
		total.ExpenseByKind[kind] = roundMoney(amount)
	}
	total.IncomeConsult = roundMoney(total.IncomeConsult)
	total.IncomeCases = roundMoney(total.IncomeCases)
	total.IncomeOther = roundMoney(total.IncomeOther)
	total.IncomeTotal = roundMoney(total.IncomeConsult + total.IncomeCases + total.IncomeOther)
	total.ExpenseTotal = roundMoney(total.ExpenseTotal)
	total.MarketingSpend = roundMoney(total.ExpenseByKind[KindMarketing])
	total.DirectCost = roundMoney(total.ExpenseByKind[KindDirect])
	total.GrossProfit = roundMoney(total.IncomeTotal - total.DirectCost)
	total.Balance = roundMoney(total.IncomeTotal - total.ExpenseTotal)
	total.Cumulative = cumulative
	total.CAC = ratio(total.MarketingSpend, float64(total.NewClients))
	total.CPL = ratio(total.MarketingSpend, float64(total.Leads))
	total.ROMI = ratio(total.IncomeTotal-total.MarketingSpend, total.MarketingSpend)

	return Report{Months: months, Total: total}
}

func ratio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return roundMoney(numerator / denominator)
}
