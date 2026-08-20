package expenseimport

import (
	"strings"
	"testing"
	"time"

	"multiagent-seo/internal/domain/finance"
)

const header = "tab,date,description,amount,method,category\n"

func parseCSV(t *testing.T, body string) Result {
	t.Helper()
	got, err := Parse(strings.NewReader(header + body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func TestParseMapsSheetLabels(t *testing.T) {
	got := parseCSV(t, "2024-08,08/08,Реклама Гугл,28 927,карта,реклама гугл\n")

	if len(got.Expenses) != 1 {
		t.Fatalf("want 1 expense, got %+v (rejected: %+v)", got.Expenses, got.Rejected)
	}
	e := got.Expenses[0]
	if e.Amount != 28927 {
		t.Errorf("amount = %v, want 28927 (spaces are thousands separators)", e.Amount)
	}
	if e.CategoryCode != "google_ads" {
		t.Errorf("category = %q, want google_ads", e.CategoryCode)
	}
	if e.PaymentMethod != finance.PaymentCard {
		t.Errorf("method = %q, want card", e.PaymentMethod)
	}
	if e.Status != finance.StatusPosted || e.Origin != finance.OriginImported {
		t.Errorf("status/origin = %q/%q, want posted/imported", e.Status, e.Origin)
	}
	if want := "sheet:2024-08:2"; e.ExternalRef != want {
		t.Errorf("external ref = %q, want %q — re-running the import must not double-charge", e.ExternalRef, want)
	}
	if !e.SpentAt.Equal(time.Date(2024, time.August, 8, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("spentAt = %v, want 2024-08-08", e.SpentAt)
	}
}

// The sheet's own labels for the same thing: "счет" without the ё, an empty
// method (most of December), "от компании" for the company account.
func TestParsePaymentMethods(t *testing.T) {
	got := parseCSV(t, strings.Join([]string{
		"2024-10,04/10/24,Бинотель,1013,счет,телефония",
		"2024-12,10/12,Верстальщик,2000,,верстка",
		"2024-11,04/11/24,Бинотель,1100,от компании,телефония",
	}, "\n")+"\n")

	if len(got.Expenses) != 3 {
		t.Fatalf("want 3, got %d (rejected: %+v)", len(got.Expenses), got.Rejected)
	}
	want := []finance.PaymentMethod{finance.PaymentInvoice, finance.PaymentCard, finance.PaymentCompany}
	for i, w := range want {
		if got.Expenses[i].PaymentMethod != w {
			t.Errorf("row %d method = %q, want %q", i, got.Expenses[i].PaymentMethod, w)
		}
	}
}

// "07/11/04" is November 2024 with a typo'd year — the tab is the one thing that
// cannot be mistyped row by row, so it wins, and the correction is reported.
func TestParseTakesTheYearFromTheTab(t *testing.T) {
	got := parseCSV(t, "2024-11,07/11/04,Водафон,300,карта,телефония\n")

	if len(got.Expenses) != 1 {
		t.Fatalf("want 1, got %+v", got.Rejected)
	}
	if !got.Expenses[0].SpentAt.Equal(time.Date(2024, time.November, 7, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("spentAt = %v, want 2024-11-07", got.Expenses[0].SpentAt)
	}
	if len(got.Notes) != 1 || !strings.Contains(got.Notes[0].Message, "2004") {
		t.Errorf("the year correction must be reported, got %+v", got.Notes)
	}
}

// Advocate payouts for one month were paid in the next; a cash-basis P&L keeps
// them where the money actually left.
func TestParseKeepsAPaymentDatedInTheNextMonth(t *testing.T) {
	got := parseCSV(t, "2024-08,04/09,адвокаты за август,7700,карта,адвокаты\n")

	if len(got.Expenses) != 1 {
		t.Fatalf("want 1, got %+v", got.Rejected)
	}
	if !got.Expenses[0].SpentAt.Equal(time.Date(2024, time.September, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("spentAt = %v, want 2024-09-04", got.Expenses[0].SpentAt)
	}
	if got.ByTab["2024-08"] != 7700 {
		t.Errorf("ByTab = %v, want the row counted under its own tab for reconciliation", got.ByTab)
	}
}

func TestParseDatelessRowGoesToTheFirstAndSaysSo(t *testing.T) {
	got := parseCSV(t, "2025-01,,Алсана,9140,,контекст зп\n")

	if len(got.Expenses) != 1 {
		t.Fatalf("want 1, got %+v", got.Rejected)
	}
	if !got.Expenses[0].SpentAt.Equal(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("spentAt = %v, want 2025-01-01", got.Expenses[0].SpentAt)
	}
	if len(got.Notes) != 1 {
		t.Errorf("a filed-to-the-1st row must be reported, got %+v", got.Notes)
	}
}

// Nothing is invented: a blank amount, an unknown label or an unreadable tab is
// reported for a human, never guessed.
func TestParseRejectsInsteadOfGuessing(t *testing.T) {
	got := parseCSV(t, strings.Join([]string{
		"2024-10,14/10/24,Реклама Гугл,,счет,реклама гугл",
		"2024-10,14/10/24,Что-то,100,карта,новая статья",
		"2024-10,14/10/24,Отрицательная,-100,карта,телефония",
		"2024-10,14/10/24,Не число,абв,карта,телефония",
		"октябрь,14/10/24,Плохой таб,100,карта,телефония",
		"2024-10,14/10/24,Плохой метод,100,биткоин,телефония",
	}, "\n")+"\n")

	if len(got.Expenses) != 0 {
		t.Fatalf("nothing should have been imported, got %+v", got.Expenses)
	}
	if len(got.Rejected) != 6 {
		t.Fatalf("want 6 rejects, got %d: %+v", len(got.Rejected), got.Rejected)
	}
	for _, want := range []string{"no amount", "unknown category", "not positive", "not a number", "not a YYYY-MM month", "unknown payment method"} {
		found := false
		for _, r := range got.Rejected {
			if strings.Contains(r.Reason, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no reject explained by %q: %+v", want, got.Rejected)
		}
	}
}

func TestParseNormalizesADayTheMonthDoesNotHave(t *testing.T) {
	got := parseCSV(t, "2024-11,31/11/24,Опечатка,100,карта,телефония\n")

	if len(got.Expenses) != 1 {
		t.Fatalf("want 1, got %+v", got.Rejected)
	}
	if got.Expenses[0].SpentAt.Day() == 31 {
		t.Error("31 November should have been normalized")
	}
	if len(got.Notes) == 0 {
		t.Error("a silently shifted date must be reported")
	}
}

func TestReconcileFlagsOnlyRealMismatches(t *testing.T) {
	byTab := map[string]float64{
		"2024-08": 121522,
		"2024-09": 89478,
		"2024-12": 72750,
	}
	expected := []Expected{
		{Tab: "2024-08", Summary: 121522, Breakdown: 121522},
		{Tab: "2024-09", Summary: 90578, Breakdown: 90578},
		// the sheet's own two totals disagree for December; matching either is fine
		{Tab: "2024-12", Summary: 71250, Breakdown: 72750},
		{Tab: "2025-01", Summary: 73090, Breakdown: 73090},
	}

	diffs := Reconcile(byTab, expected)
	if len(diffs) != 4 {
		t.Fatalf("want a row per month including the one with no parsed rows, got %+v", diffs)
	}
	byName := map[string]Diff{}
	for _, d := range diffs {
		byName[d.Tab] = d
	}
	if !byName["2024-08"].OK() {
		t.Error("2024-08 matches exactly and must pass")
	}
	if byName["2024-09"].OK() || byName["2024-09"].SummaryDelta() != -1100 {
		t.Errorf("2024-09 = %+v, want a -1100 mismatch", byName["2024-09"])
	}
	if !byName["2024-12"].OK() {
		t.Error("2024-12 matches the breakdown total, which is enough")
	}
	if byName["2025-01"].OK() || byName["2025-01"].Parsed != 0 {
		t.Errorf("a month with no parsed rows must not silently pass: %+v", byName["2025-01"])
	}
	if diffs[0].Tab > diffs[1].Tab {
		t.Error("diffs must be ordered by month")
	}
}
