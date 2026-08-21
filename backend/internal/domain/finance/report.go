package finance

// MonthFacts is one month as the repository reads it: income straight from
// consultations/case_payments/other_income, expenses already summed per
// category, plus the two counters the acquisition metrics need.
type MonthFacts struct {
	Month          string // MonthKey format, "2026-08"
	ConsultRevenue float64
	// how many priced, completed consultations made up ConsultRevenue — the
	// divisor for the average ticket, which a sum alone can't give
	ConsultCount      int
	CasePaid          float64
	CasePaymentCount  int
	OtherIncome       float64
	ExpenseByCategory map[string]float64
	Leads             int
	// clients whose first-ever lead landed in this month
	NewClients int
	// of that cohort, how many ever paid anything — the honest CAC denominator.
	// Counting form-fillers instead understated CAC roughly eightfold on the
	// real data: 658 clients acquired, 76 who ever paid.
	CohortPayers int
	// everything that cohort has ever paid, with no upper date bound: a client
	// acquired in March who pays in June belongs to March's acquisition cost.
	// This is what keeps LTV independent of the period's own revenue.
	CohortRevenue float64
	// distinct clients who paid anything IN this month
	PayingClients int
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

	Leads            int
	NewClients       int
	CohortPayers     int
	PayingClients    int
	CohortRevenue    float64
	ConsultCount     int
	CasePaymentCount int
	// 0 when the denominator is 0 — an undefined ratio is not a real zero, but
	// the alternative (NaN/Inf) crosses JSON as null and breaks the page.
	CAC  float64
	CPL  float64
	ROMI float64

	// Derived numbers the spreadsheet never had. All of them are 0 on a zero
	// denominator, same rule as the ratios above.
	AvgConsultTicket float64
	AvgCaseTicket    float64
	// MarginPercent is balance as a share of income — the sign matters more than
	// the magnitude in a month that lost money.
	MarginPercent float64
	// MarketingShare is marketing spend as a share of all spend, which is what
	// makes a heavy month legible: 599 967 ₴ means little, "39% of it is ads" a lot.
	MarketingShare float64
	// RevenuePerClient is this period's revenue over the clients who actually
	// paid in it — a plain average ticket per client, not a lifetime value.
	RevenuePerClient float64
	// LTV is what a client acquired in this period has paid to date, across
	// their whole life, over the cohort members who paid at all.
	LTV float64
	// LtvToCac compares that lifetime figure against what acquiring the cohort
	// cost. It must NOT reduce to ROMI: an earlier version divided period
	// revenue by new clients and marketing by the same new clients, so
	// LTV/CAC was algebraically ROMI+1 — the same number under two names.
	LtvToCac float64
	// LeadToConsult is the share of this month's leads that turned into a paid
	// consultation. Not a cohort conversion (a lead converts weeks later — see
	// leadstats.Funnel for that), just this month's ratio.
	LeadToConsult float64
	// BreakEvenConsults is how many average-ticket consultations the month's
	// spend would have needed to pay for itself.
	BreakEvenConsults float64
	// IncomeGrowth is income against the previous month in the report, as a
	// share: +0.35 is a 35% rise. 0 for the first month, which has nothing to
	// compare against.
	IncomeGrowth float64
}

// Period is the span the data actually covers. The page cannot guess it: a
// default window of "the last 12 months" showed twelve empty columns and a
// running total repeated in every one of them, because the firm's history sits
// well in the past. Offering only periods that contain something is the fix.
type Period struct {
	// First/LastMonth bound the months that carry MONEY, which is what "all
	// time" should show: dense columns instead of a wall of empty ones.
	FirstMonth string // MonthKey, "" when there is nothing at all
	LastMonth  string
	// LastActivityMonth also counts leads, which keep arriving long after the
	// last recorded payment — those months are worth being able to select
	// (they say "leads came, nothing was booked"), just not worth defaulting to.
	LastActivityMonth string
	HasData           bool
}

type Report struct {
	Months []MonthReport
	// Receivable is a live figure across every case still owing (fee minus
	// paid), deliberately not scoped to the report window: money owed from a
	// case opened last year is still owed today.
	Receivable float64
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
			CohortPayers:      f.CohortPayers,
			PayingClients:     f.PayingClients,
			CohortRevenue:     f.CohortRevenue,
			ConsultCount:      f.ConsultCount,
			CasePaymentCount:  f.CasePaymentCount,
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

		// per client who actually paid, not per person who filled in a form
		m.CAC = ratio(m.MarketingSpend, float64(f.CohortPayers))
		m.CPL = ratio(m.MarketingSpend, float64(m.Leads))
		m.ROMI = ratio(m.IncomeTotal-m.MarketingSpend, m.MarketingSpend)
		deriveExtras(&m, f)
		if len(months) > 0 {
			m.IncomeGrowth = ratio(m.IncomeTotal-months[len(months)-1].IncomeTotal, months[len(months)-1].IncomeTotal)
		}

		total.IncomeConsult += f.ConsultRevenue
		total.IncomeCases += f.CasePaid
		total.IncomeOther += f.OtherIncome
		total.ExpenseTotal += m.ExpenseTotal
		total.Leads += f.Leads
		total.NewClients += f.NewClients
		total.ConsultCount += f.ConsultCount
		total.CasePaymentCount += f.CasePaymentCount
		total.CohortPayers += f.CohortPayers
		total.PayingClients += f.PayingClients
		total.CohortRevenue += f.CohortRevenue

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
	total.CAC = ratio(total.MarketingSpend, float64(total.CohortPayers))
	total.CPL = ratio(total.MarketingSpend, float64(total.Leads))
	total.ROMI = ratio(total.IncomeTotal-total.MarketingSpend, total.MarketingSpend)
	deriveExtras(&total, MonthFacts{
		ConsultRevenue:   total.IncomeConsult,
		ConsultCount:     total.ConsultCount,
		CasePaid:         total.IncomeCases,
		CasePaymentCount: total.CasePaymentCount,
		Leads:            total.Leads,
		NewClients:       total.NewClients,
		CohortPayers:     total.CohortPayers,
		CohortRevenue:    total.CohortRevenue,
		PayingClients:    total.PayingClients,
	})
	// growth over a whole period has no previous period to compare against
	total.IncomeGrowth = 0

	return Report{Months: months, Total: total}
}

// deriveExtras fills the numbers that are pure arithmetic over what is already
// in the row — kept in one place so the "Итого" column computes them from its
// own sums instead of averaging the months.
func deriveExtras(m *MonthReport, f MonthFacts) {
	m.AvgConsultTicket = ratio(f.ConsultRevenue, float64(f.ConsultCount))
	m.AvgCaseTicket = ratio(f.CasePaid, float64(f.CasePaymentCount))
	m.MarginPercent = ratio(m.Balance, m.IncomeTotal)
	m.MarketingShare = ratio(m.MarketingSpend, m.ExpenseTotal)
	m.RevenuePerClient = ratio(m.IncomeTotal, float64(f.PayingClients))
	m.LTV = ratio(f.CohortRevenue, float64(f.CohortPayers))
	m.LtvToCac = ratio(m.LTV, m.CAC)
	m.LeadToConsult = ratio(float64(f.ConsultCount), float64(f.Leads))
	m.BreakEvenConsults = ratio(m.ExpenseTotal, m.AvgConsultTicket)
}

func ratio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return roundMoney(numerator / denominator)
}
