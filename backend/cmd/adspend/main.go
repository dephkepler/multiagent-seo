// adspend prints what GA4 reports as Google Ads cost next to what the expenses
// ledger holds for the google_ads category, on the same service account that
// already reads sessions — no Ads API credentials involved.
//
// DIAGNOSTIC ONLY, and the reason is worth keeping: on this property the GA4
// figure is not money. It is non-additive across query windows (December alone
// reads 323 626, the same December inside a December+January range reads
// 796 004), and no single scale factor maps it onto payments actually made —
// the ratios come out at 12.5 / 19.2 / 23.5 for December / January / February
// 2025. GA4 refuses advertiserAdCost without an ads-scoped session dimension,
// and that re-attribution is what moves the number. So nothing here is ever
// written to the ledger; automating ad spend needs the Google Ads API proper
// (developer token + OAuth + customer ID).
//
// Usage:
//
//	go run ./cmd/adspend
//	go run ./cmd/adspend --from 2024-12-01 --to 2025-04-30
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/ga4"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
)

func main() {
	fromFlag := flag.String("from", "2024-06-01", "start date, YYYY-MM-DD")
	toFlag := flag.String("to", time.Now().Format("2006-01-02"), "end date, YYYY-MM-DD")
	flag.Parse()

	from, err := time.Parse("2006-01-02", *fromFlag)
	if err != nil {
		log.Fatalf("--from: %v", err)
	}
	to, err := time.Parse("2006-01-02", *toFlag)
	if err != nil {
		log.Fatalf("--to: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.GA4.PropertyID == "" {
		log.Fatal("CF_GA4_PROPERTY_ID is not set — nothing to read")
	}

	ctx := context.Background()
	client, err := ga4.New(ctx, cfg.Sheets.CredentialsFile, cfg.GA4.PropertyID)
	if err != nil {
		log.Fatalf("ga4: %v", err)
	}

	spend, err := client.AdSpend(ctx, from, to)
	if err != nil {
		log.Fatalf("ad spend: %v", err)
	}

	ledger := ledgerByMonth(ctx, cfg, from, to)

	fmt.Printf("%-10s %16s %10s %16s\n", "month", "GA4 raw cost", "clicks", "ledger (posted)")
	for _, s := range spend {
		month := s.Month
		if month == "" {
			month = "(undated)"
		}
		fmt.Printf("%-10s %16.2f %10d %16.2f\n", month, s.Cost, s.Clicks, ledger[s.Month])
	}
	fmt.Println("\nDIAGNOSTIC ONLY — the GA4 column is not money: it is non-additive across")
	fmt.Println("query windows and no scale factor maps it onto real payments. Nothing is")
	fmt.Println("written to the ledger from here; real automation needs the Google Ads API.")
}

func ledgerByMonth(ctx context.Context, cfg config.Config, from, to time.Time) map[string]float64 {
	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		fmt.Printf("(no database: %v — ledger column will read 0)\n\n", err)
		return map[string]float64{}
	}
	defer database.Close()

	facts, err := postgres.NewFinanceRepository(database.Pool()).MonthlyFacts(ctx, from, to)
	if err != nil {
		fmt.Printf("(monthly facts failed: %v — ledger column will read 0)\n\n", err)
		return map[string]float64{}
	}

	out := make(map[string]float64, len(facts))
	for _, f := range facts {
		out[f.Month] = f.ExpenseByCategory["google_ads"]
	}
	return out
}
