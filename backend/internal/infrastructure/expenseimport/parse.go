// Package expenseimport backfills the hand-kept spreadsheet P&L into the
// expenses ledger. The sheet is six years of human typing, so parsing is
// deliberately lenient and every correction is reported instead of hidden.
package expenseimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"multiagent-seo/internal/domain/finance"
)

// Tab is the month sheet a row came from ("2024-08"); it decides the year when
// the row's own date is missing or mistyped, and it is the unit reconciliation
// runs on, because the sheet grouped a payment by the month it belonged to
// rather than the day it left the account.
type Row struct {
	Line        int
	Tab         string
	Date        string
	Description string
	Amount      string
	Method      string
	Category    string
}

type Rejected struct {
	Line   int
	Tab    string
	Raw    string
	Reason string
}

// Note is a correction that was applied rather than rejected — a year fixed
// from the tab, a missing day filed to the 1st.
type Note struct {
	Line    int
	Message string
}

type Result struct {
	Expenses []finance.Expense
	Rejected []Rejected
	Notes    []Note
	// ByTab is what the parsed rows sum to per tab, for reconciliation.
	ByTab map[string]float64
}

// The sheet's own labels, as typed, mapped onto the seeded category codes.
// "контекст зп" is the PPC contractor's retainer, which the sheet folded into
// its "Реклама Гугл" line but kept as its own ledger label.
var categoryCodes = map[string]string{
	"реклама гугл":   "google_ads",
	"контекст зп":    "ppc_specialist",
	"копирайтинг":    "copywriting",
	"смм":            "smm",
	"адвокаты":       "advocates",
	"помощник зп":    "assistant",
	"верстка":        "layout",
	"программист":    "developer",
	"дизайн":         "design",
	"телефония":      "telephony",
	"сайт":           "website",
	"компания":       "company",
	"админ":          "admin_misc",
	"админ. расходы": "admin_misc",
}

var paymentMethods = map[string]finance.PaymentMethod{
	"":            finance.PaymentCard,
	"карта":       finance.PaymentCard,
	"счет":        finance.PaymentInvoice,
	"счёт":        finance.PaymentInvoice,
	"от компании": finance.PaymentCompany,
	"наличные":    finance.PaymentCash,
	"нал":         finance.PaymentCash,
}

// Parse reads the CSV export of the sheet's per-month expense registers.
// Header: tab,date,description,amount,method,category
func Parse(r io.Reader) (Result, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = 6
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return Result{}, fmt.Errorf("expenseimport: read csv: %w", err)
	}
	if len(records) == 0 {
		return Result{}, fmt.Errorf("expenseimport: empty file")
	}

	out := Result{ByTab: map[string]float64{}}
	for i, rec := range records[1:] {
		line := i + 2 // 1-based, and the header is line 1
		row := Row{
			Line:        line,
			Tab:         strings.TrimSpace(rec[0]),
			Date:        strings.TrimSpace(rec[1]),
			Description: strings.TrimSpace(rec[2]),
			Amount:      strings.TrimSpace(rec[3]),
			Method:      strings.TrimSpace(rec[4]),
			Category:    strings.TrimSpace(rec[5]),
		}
		expense, notes, reason := convert(row)
		if reason != "" {
			out.Rejected = append(out.Rejected, Rejected{Line: line, Tab: row.Tab, Raw: strings.Join(rec, ";"), Reason: reason})
			continue
		}
		out.Notes = append(out.Notes, notes...)
		out.Expenses = append(out.Expenses, expense)
		out.ByTab[row.Tab] += expense.Amount
	}
	return out, nil
}

func convert(row Row) (finance.Expense, []Note, string) {
	tabMonth, err := time.Parse("2006-01", tabKey(row.Tab))
	if err != nil {
		return finance.Expense{}, nil, fmt.Sprintf("tab %q is not a YYYY-MM month", row.Tab)
	}

	amount, err := parseAmount(row.Amount)
	if err != nil {
		return finance.Expense{}, nil, err.Error()
	}

	code, ok := categoryCodes[strings.ToLower(row.Category)]
	if !ok {
		return finance.Expense{}, nil, fmt.Sprintf("unknown category %q", row.Category)
	}

	method, ok := paymentMethods[strings.ToLower(row.Method)]
	if !ok {
		return finance.Expense{}, nil, fmt.Sprintf("unknown payment method %q", row.Method)
	}

	spentAt, notes := parseDate(row.Date, tabMonth, row.Line)

	return finance.Expense{
		SpentAt:       spentAt,
		Amount:        amount,
		CategoryCode:  code,
		PaymentMethod: method,
		Description:   row.Description,
		Status:        finance.StatusPosted,
		Origin:        finance.OriginImported,
		ExternalRef:   finance.ImportRef(row.Tab, row.Line),
		CreatedBy:     "import",
	}, notes, ""
}

// A tab may cover two months ("2024-06-07" — the sheet's first register ran
// June into July); the first one decides the year.
func tabKey(tab string) string {
	parts := strings.Split(tab, "-")
	if len(parts) < 2 {
		return tab
	}
	return parts[0] + "-" + parts[1]
}

// Dates in the sheet run dd/mm/yy, dd/mm/yyyy, dd/mm, or nothing at all, and
// the year is sometimes a typo ("07/11/04" for November 2024). The day and
// month are taken as written; the year always comes from the tab, which is the
// one thing that cannot be mistyped row by row.
func parseDate(raw string, tabMonth time.Time, line int) (time.Time, []Note) {
	if raw == "" {
		return tabMonth, []Note{{Line: line, Message: "no date in the sheet, filed to the 1st of " + tabMonth.Format("2006-01")}}
	}

	parts := strings.Split(raw, "/")
	day, dayErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	if dayErr != nil || day < 1 || day > 31 {
		return tabMonth, []Note{{Line: line, Message: fmt.Sprintf("unreadable date %q, filed to the 1st of %s", raw, tabMonth.Format("2006-01"))}}
	}

	month := tabMonth.Month()
	var notes []Note
	if len(parts) > 1 {
		if m, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && m >= 1 && m <= 12 {
			month = time.Month(m)
		} else {
			notes = append(notes, Note{Line: line, Message: fmt.Sprintf("unreadable month in %q, took it from the tab", raw)})
		}
	}

	// A payment dated in the next month is normal in this sheet (advocate
	// payouts for August were paid in September) and is kept as written: the
	// P&L is cash-basis, so it belongs to the month the money left.
	year := tabMonth.Year()
	if month < tabMonth.Month() {
		year++
	}
	if len(parts) > 2 {
		if y, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
			written := y
			if written < 100 {
				written += 2000
			}
			if written != year {
				notes = append(notes, Note{Line: line, Message: fmt.Sprintf("year in %q reads as %d, took %d from the tab", raw, written, year)})
			}
		}
	}

	spentAt := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	// A day the month doesn't have (31/09) rolls over in time.Date; that is a
	// silent shift, so say it out loud.
	if spentAt.Day() != day || spentAt.Month() != month {
		notes = append(notes, Note{Line: line, Message: fmt.Sprintf("date %q does not exist, normalized to %s", raw, spentAt.Format("2006-01-02"))})
	}
	return spentAt, notes
}

// Amounts arrive as "1 300", "42 274", "1300,50" — thousands separated by
// spaces (including non-breaking ones, straight from the spreadsheet).
func parseAmount(raw string) (float64, error) {
	cleaned := strings.NewReplacer(" ", "", " ", "", " ", "", "₴", "", ",", ".").Replace(raw)
	if cleaned == "" {
		return 0, fmt.Errorf("no amount")
	}
	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not a number", raw)
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount %q is not positive", raw)
	}
	return amount, nil
}
