// importexpenses backfills the hand-kept spreadsheet P&L (June 2024 — January
// 2025) into the expenses ledger. The sheet is human typing, so this tool never
// writes blind: it reconciles what it parsed against the two totals the sheet
// states for every month, prints every correction it had to apply, and refuses
// to commit a month whose numbers do not add up.
//
// Every row is written with an external ref ("sheet:<tab>:<line>"), so
// re-running the import inserts nothing the second time.
//
// Usage:
//
//	go run ./cmd/importexpenses                # dry run against the bundled data
//	go run ./cmd/importexpenses --commit       # write the reconciling months
//	go run ./cmd/importexpenses --commit --force  # write even the ones that don't
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/expenseimport"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
)

func main() {
	file := flag.String("file", "data/expenses-2024-2025.csv", "CSV export of the sheet's monthly expense registers")
	totals := flag.String("totals", "data/expenses-2024-2025-totals.csv", "CSV of the totals the sheet itself states per month")
	commit := flag.Bool("commit", false, "actually write to the database (default: dry run)")
	force := flag.Bool("force", false, "commit months that fail reconciliation too")
	flag.Parse()

	result, diffs, err := load(*file, *totals)
	if err != nil {
		log.Fatal(err)
	}

	report(result, diffs)

	failed := map[string]bool{}
	for _, d := range diffs {
		if !d.OK() {
			failed[d.Tab] = true
		}
	}

	if !*commit {
		fmt.Printf("\nDRY RUN — nothing written. %d rows ready. Re-run with --commit to apply.\n", len(result.Expenses))
		return
	}
	if len(failed) > 0 && !*force {
		fmt.Printf("\n%d month(s) do not reconcile: fix the CSV, or re-run with --force to import them as they are.\n", len(failed))
		os.Exit(1)
	}

	inserted, skipped, err := write(*result, failed, *force)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nDone. %d inserted, %d already present (external ref taken).\n", inserted, skipped)
}

func load(file, totals string) (*expenseimport.Result, []expenseimport.Diff, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	result, err := expenseimport.Parse(f)
	if err != nil {
		return nil, nil, err
	}

	var expected []expenseimport.Expected
	if totals != "" {
		t, err := os.Open(totals)
		if err != nil {
			return nil, nil, fmt.Errorf("open %s: %w", totals, err)
		}
		defer t.Close()
		if expected, err = expenseimport.ParseExpected(t); err != nil {
			return nil, nil, err
		}
	}
	return &result, expenseimport.Reconcile(result.ByTab, expected), nil
}

func report(result *expenseimport.Result, diffs []expenseimport.Diff) {
	fmt.Printf("Parsed %d rows, rejected %d.\n\n", len(result.Expenses), len(result.Rejected))

	fmt.Println("Reconciliation (parsed vs the sheet's own two totals)")
	fmt.Printf("%-12s %12s %12s %10s %12s %10s\n", "month", "parsed", "summary", "Δ", "breakdown", "Δ")
	for _, d := range diffs {
		mark := "OK"
		if !d.OK() {
			mark = "MISMATCH"
		}
		fmt.Printf("%-12s %12.0f %12.0f %10.0f %12.0f %10.0f  %s\n",
			d.Tab, d.Parsed, d.Summary, d.SummaryDelta(), d.Breakdown, d.BreakdownDelta(), mark)
	}

	if len(result.Rejected) > 0 {
		fmt.Println("\nRejected rows (nothing was invented for these — add them by hand if they are real):")
		for _, r := range result.Rejected {
			fmt.Printf("  line %d [%s] %s — %s\n", r.Line, r.Tab, r.Reason, r.Raw)
		}
	}

	if len(result.Notes) > 0 {
		fmt.Printf("\nCorrections applied (%d):\n", len(result.Notes))
		for _, n := range result.Notes {
			fmt.Printf("  line %d: %s\n", n.Line, n.Message)
		}
	}
}

func write(result expenseimport.Result, failed map[string]bool, force bool) (int, int, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, 0, fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		return 0, 0, fmt.Errorf("connect database: %w", err)
	}
	defer database.Close()

	repo := postgres.NewFinanceRepository(database.Pool())
	var inserted, skipped int
	for _, e := range result.Expenses {
		if !force && failed[tabOf(e.ExternalRef)] {
			continue
		}
		ok, err := repo.InsertGenerated(ctx, e)
		if err != nil {
			return inserted, skipped, fmt.Errorf("insert %s: %w", e.ExternalRef, err)
		}
		if ok {
			inserted++
			continue
		}
		skipped++
	}
	return inserted, skipped, nil
}

// external refs read "sheet:<tab>:<line>"; the tab is what reconciliation gates on
func tabOf(externalRef string) string {
	parts := strings.Split(externalRef, ":")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}
