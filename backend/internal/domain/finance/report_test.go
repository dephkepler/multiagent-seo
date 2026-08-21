package finance

import (
	"testing"
)

var testCategories = []Category{
	{Code: "google_ads", Kind: KindMarketing},
	{Code: "copywriting", Kind: KindMarketing},
	{Code: "advocates", Kind: KindDirect},
	{Code: "website", Kind: KindInfra},
}

func TestBuildReportCumulativeContinuesFromStartingBalance(t *testing.T) {
	facts := []MonthFacts{
		{
			Month:             "2026-07",
			ConsultRevenue:    10000,
			ExpenseByCategory: map[string]float64{"google_ads": 15000},
		},
		{
			Month:             "2026-08",
			ConsultRevenue:    20000,
			CasePaid:          30000,
			ExpenseByCategory: map[string]float64{"google_ads": 20000, "advocates": 5000},
		},
	}

	got := BuildReport(facts, testCategories, -100000)

	if got.Months[0].Balance != -5000 {
		t.Errorf("july balance = %v, want -5000", got.Months[0].Balance)
	}
	if got.Months[0].Cumulative != -105000 {
		t.Errorf("july cumulative = %v, want -105000", got.Months[0].Cumulative)
	}
	if got.Months[1].Balance != 25000 {
		t.Errorf("august balance = %v, want 25000", got.Months[1].Balance)
	}
	if got.Months[1].Cumulative != -80000 {
		t.Errorf("august cumulative = %v, want -80000", got.Months[1].Cumulative)
	}
	if got.Total.Cumulative != -80000 {
		t.Errorf("total cumulative = %v, want the last month's", got.Total.Cumulative)
	}
	if got.Total.Balance != 20000 {
		t.Errorf("total balance = %v, want 20000", got.Total.Balance)
	}
}

func TestBuildReportSplitsByKind(t *testing.T) {
	facts := []MonthFacts{{
		Month:          "2026-08",
		ConsultRevenue: 50000,
		ExpenseByCategory: map[string]float64{
			"google_ads":  20000,
			"copywriting": 5000,
			"advocates":   8000,
			"website":     642,
		},
	}}

	m := BuildReport(facts, testCategories, 0).Months[0]

	if m.MarketingSpend != 25000 {
		t.Errorf("marketing = %v, want 25000", m.MarketingSpend)
	}
	if m.DirectCost != 8000 {
		t.Errorf("direct = %v, want 8000", m.DirectCost)
	}
	if m.GrossProfit != 42000 {
		t.Errorf("gross profit = %v, want 42000", m.GrossProfit)
	}
	if m.ExpenseByKind[KindInfra] != 642 {
		t.Errorf("infra = %v, want 642", m.ExpenseByKind[KindInfra])
	}
}

func TestBuildReportAcquisitionMetrics(t *testing.T) {
	facts := []MonthFacts{{
		Month:             "2026-08",
		ConsultRevenue:    60000,
		ExpenseByCategory: map[string]float64{"google_ads": 20000},
		Leads:             100,
		NewClients:        25,
		CohortPayers:      25,
	}}

	m := BuildReport(facts, testCategories, 0).Months[0]

	if m.CAC != 800 {
		t.Errorf("CAC = %v, want 800", m.CAC)
	}
	if m.CPL != 200 {
		t.Errorf("CPL = %v, want 200", m.CPL)
	}
	if m.ROMI != 2 {
		t.Errorf("ROMI = %v, want 2", m.ROMI)
	}
}

// A month with spend but no conversions must not blow up the page with NaN/Inf.
func TestBuildReportZeroDenominators(t *testing.T) {
	facts := []MonthFacts{{
		Month:             "2026-08",
		ExpenseByCategory: map[string]float64{"google_ads": 20000},
	}}

	m := BuildReport(facts, testCategories, 0).Months[0]

	if m.CAC != 0 || m.CPL != 0 {
		t.Errorf("CAC/CPL = %v/%v, want 0/0 with no leads or clients", m.CAC, m.CPL)
	}

	noSpend := BuildReport([]MonthFacts{{Month: "2026-08", ConsultRevenue: 5000, Leads: 3}}, testCategories, 0).Months[0]
	if noSpend.ROMI != 0 {
		t.Errorf("ROMI = %v, want 0 with no marketing spend", noSpend.ROMI)
	}
}

// Total ratios come from the summed money, not from averaging the monthly ones:
// a 1-client month at 20k CAC must not weigh the same as a 40-client month.
func TestBuildReportTotalRatiosRecomputedFromSums(t *testing.T) {
	facts := []MonthFacts{
		{Month: "2026-07", ExpenseByCategory: map[string]float64{"google_ads": 20000}, NewClients: 1, CohortPayers: 1},
		{Month: "2026-08", ExpenseByCategory: map[string]float64{"google_ads": 20000}, NewClients: 39, CohortPayers: 39},
	}

	got := BuildReport(facts, testCategories, 0)

	if got.Total.CAC != 1000 {
		t.Errorf("total CAC = %v, want 1000 (40000/40), not the 10250 average of 20000 and 512.82", got.Total.CAC)
	}
	if got.Total.ExpenseByCategory["google_ads"] != 40000 {
		t.Errorf("total google_ads = %v, want 40000", got.Total.ExpenseByCategory["google_ads"])
	}
}

func TestBuildReportDerivedNumbers(t *testing.T) {
	facts := []MonthFacts{
		{
			Month:             "2026-07",
			ConsultRevenue:    10000,
			ConsultCount:      10,
			ExpenseByCategory: map[string]float64{"google_ads": 8000, "advocates": 2000},
			Leads:             50,
			NewClients:        10,
		},
		{
			Month:             "2026-08",
			ConsultRevenue:    20000,
			ConsultCount:      20,
			CasePaid:          30000,
			CasePaymentCount:  3,
			ExpenseByCategory: map[string]float64{"google_ads": 20000, "advocates": 5000},
			Leads:             100,
			NewClients:        100,
			// only 25 of those 100 ever paid, and they have paid 125 000 to date
			CohortPayers:  25,
			CohortRevenue: 125000,
			PayingClients: 25,
		},
	}

	got := BuildReport(facts, testCategories, 0)
	july, august := got.Months[0], got.Months[1]

	if august.AvgConsultTicket != 1000 {
		t.Errorf("avg consult = %v, want 1000", august.AvgConsultTicket)
	}
	if august.AvgCaseTicket != 10000 {
		t.Errorf("avg case = %v, want 10000", august.AvgCaseTicket)
	}
	if august.MarginPercent != 0.5 {
		t.Errorf("margin = %v, want 0.5 (50000 income, 25000 spend)", august.MarginPercent)
	}
	if august.MarketingShare != 0.8 {
		t.Errorf("marketing share = %v, want 0.8 (20000 of 25000)", august.MarketingShare)
	}
	if august.RevenuePerClient != 2000 {
		t.Errorf("revenue per client = %v, want 2000 (50000 over 25 who paid)", august.RevenuePerClient)
	}
	if august.CAC != 800 {
		t.Errorf("CAC = %v, want 800 — marketing over the 25 who paid, not the 100 who filled a form", august.CAC)
	}
	if august.LTV != 5000 {
		t.Errorf("LTV = %v, want 5000 (125000 lifetime over 25 payers)", august.LTV)
	}
	if august.LtvToCac != 6.25 {
		t.Errorf("LTV/CAC = %v, want 6.25", august.LtvToCac)
	}
	// The defect this replaced: LTV/CAC was period income over new clients
	// divided by marketing over the same new clients, which cancels to
	// income/marketing — ROMI+1 under another name. It must not come back.
	if august.LtvToCac == august.ROMI+1 {
		t.Errorf("LTV/CAC (%v) collapsed back into ROMI+1 (%v) — it is measuring nothing of its own",
			august.LtvToCac, august.ROMI+1)
	}
	if august.LeadToConsult != 0.2 {
		t.Errorf("lead→consult = %v, want 0.2 (20 of 100)", august.LeadToConsult)
	}
	if august.BreakEvenConsults != 25 {
		t.Errorf("break-even = %v, want 25 (25000 spend / 1000 ticket)", august.BreakEvenConsults)
	}
	if august.IncomeGrowth != 4 {
		t.Errorf("growth = %v, want 4 (10000 -> 50000)", august.IncomeGrowth)
	}
	if july.IncomeGrowth != 0 {
		t.Errorf("first month growth = %v, want 0 — nothing to compare against", july.IncomeGrowth)
	}

	// The total column derives from its own sums, not from averaging the months:
	// 30000 consultation revenue over 30 held consultations.
	if got.Total.AvgConsultTicket != 1000 {
		t.Errorf("total avg consult = %v, want 1000", got.Total.AvgConsultTicket)
	}
	if got.Total.IncomeGrowth != 0 {
		t.Errorf("total growth = %v, want 0 — a period has no previous period here", got.Total.IncomeGrowth)
	}
}

// A month with income but nothing to divide by must not produce NaN or Inf,
// which cross JSON as null and blank the page.
func TestBuildReportDerivedZeroDenominators(t *testing.T) {
	facts := []MonthFacts{{Month: "2026-08", ConsultRevenue: 5000}}

	m := BuildReport(facts, testCategories, 0).Months[0]

	for name, v := range map[string]float64{
		"avg consult":  m.AvgConsultTicket,
		"ltv":          m.LTV,
		"avg case":     m.AvgCaseTicket,
		"marketing":    m.MarketingShare,
		"per client":   m.RevenuePerClient,
		"ltv/cac":      m.LtvToCac,
		"lead→consult": m.LeadToConsult,
		"break-even":   m.BreakEvenConsults,
	} {
		if v != 0 {
			t.Errorf("%s = %v, want 0", name, v)
		}
	}
	if m.MarginPercent != 1 {
		t.Errorf("margin = %v, want 1 — 5000 income and no spend is a 100%% margin", m.MarginPercent)
	}
}
