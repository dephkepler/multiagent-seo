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
		{Month: "2026-07", ExpenseByCategory: map[string]float64{"google_ads": 20000}, NewClients: 1},
		{Month: "2026-08", ExpenseByCategory: map[string]float64{"google_ads": 20000}, NewClients: 39},
	}

	got := BuildReport(facts, testCategories, 0)

	if got.Total.CAC != 1000 {
		t.Errorf("total CAC = %v, want 1000 (40000/40), not the 10250 average of 20000 and 512.82", got.Total.CAC)
	}
	if got.Total.ExpenseByCategory["google_ads"] != 40000 {
		t.Errorf("total google_ads = %v, want 40000", got.Total.ExpenseByCategory["google_ads"])
	}
}
