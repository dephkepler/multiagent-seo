// incometopup closes the gap between what the CRM can account for and what the
// company's own monthly records say came in.
//
// This is not an import. Both source tabs are fully imported and they simply
// end — the 55k tab around 2024-12-10, проекти at 2024-11-14. January 2025's
// income was never written down row by row anywhere, only as a monthly total,
// so no better importer can recover it. What can be done honestly is to book
// the difference as one labelled row per month, which makes the month's total
// true while saying out loud that the detail is gone.
//
// It only ever adds money the CRM is missing. A month where the CRM shows MORE
// than the records is the opposite problem (usually double counting) and is
// reported, never "fixed" — a negative row would bury it.
//
// Usage:
//
//	go run ./cmd/incometopup                     # dry run, prints the comparison
//	go run ./cmd/incometopup --commit            # book the missing months
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"multiagent-seo/internal/domain/finance"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
)

func main() {
	totals := flag.String("totals", "data/income-2024-2025-totals.csv", "CSV of the monthly income totals the company itself recorded (month,total)")
	from := flag.String("from", "2024-06-01", "start date, YYYY-MM-DD")
	to := flag.String("to", "2025-01-31", "end date, YYYY-MM-DD")
	commit := flag.Bool("commit", false, "actually write the top-up rows (default: dry run)")
	flag.Parse()

	expected, err := readTotals(*totals)
	if err != nil {
		log.Fatal(err)
	}
	fromDate, err := time.Parse("2006-01-02", *from)
	if err != nil {
		log.Fatalf("--from: %v", err)
	}
	toDate, err := time.Parse("2006-01-02", *to)
	if err != nil {
		log.Fatalf("--to: %v", err)
	}

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer database.Close()

	repo := postgres.NewFinanceRepository(database.Pool())
	facts, err := repo.MonthlyFacts(ctx, fromDate, toDate)
	if err != nil {
		log.Fatalf("monthly facts: %v", err)
	}

	plan := finance.PlanIncomeTopUps(facts, expected)
	report(plan)

	if !*commit {
		fmt.Println("\nDRY RUN — nothing written. Re-run with --commit to book the missing months.")
		return
	}

	var booked, skipped int
	for _, t := range plan {
		if t.Amount <= 0 {
			continue
		}
		inserted, err := repo.InsertOtherIncomeGenerated(ctx, finance.OtherIncome{
			// last day of the month: the money arrived during it, and no more
			// precise date survives anywhere
			ReceivedAt:  endOfMonth(t.Month),
			Amount:      t.Amount,
			Source:      "сводная таблица",
			Description: fmt.Sprintf("Добор до итога за %s: CRM видит %.0f ₴ из %.0f ₴, детализация утрачена", t.Month, t.CRM, t.Expected),
			ExternalRef: t.ExternalRef,
			CreatedBy:   "reconcile",
		})
		if err != nil {
			log.Fatalf("book %s: %v", t.Month, err)
		}
		if inserted {
			booked++
			continue
		}
		skipped++
	}
	fmt.Printf("\nDone. %d month(s) booked, %d already present.\n", booked, skipped)
}

func report(plan []finance.IncomeTopUp) {
	fmt.Printf("%-10s %12s %12s %12s  %s\n", "month", "CRM", "records", "Δ", "")
	var missing, overshoot float64
	for _, t := range plan {
		gap := t.Expected - t.CRM
		note := ""
		switch {
		case t.Amount > 0:
			note = "будет добрано"
			missing += t.Amount
		case gap < -0.01:
			note = "CRM показывает БОЛЬШЕ — это не добирается, разбирайся с задвоением"
			overshoot += -gap
		default:
			note = "сходится"
		}
		fmt.Printf("%-10s %12.0f %12.0f %12.0f  %s\n", t.Month, t.CRM, t.Expected, gap, note)
	}
	fmt.Printf("\nвсего к добору: %.0f ₴", missing)
	if overshoot > 0 {
		fmt.Printf("; превышение, требующее разбора: %.0f ₴", overshoot)
	}
	fmt.Println()
}

// Header: month,total
func readTotals(path string) (map[string]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = 2
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	out := make(map[string]float64, len(records)-1)
	for i, rec := range records[1:] {
		month := strings.TrimSpace(rec[0])
		cleaned := strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(strings.TrimSpace(rec[1]))
		total, err := strconv.ParseFloat(cleaned, 64)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: total %q is not a number", path, i+2, rec[1])
		}
		out[month] = total
	}
	return out, nil
}

func endOfMonth(month string) time.Time {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}
	}
	return t.AddDate(0, 1, -1)
}
