// sheets-check probes the configured Google Sheets setup end-to-end:
// fetches metadata (list of tabs), then runs a LookupKeywords for the topic
// passed as the first argument (or a canned one). Useful for verifying
// share permissions, tab name, and column layout.
//
// Usage:
//
//	go run ./cmd/sheets-check
//	go run ./cmd/sheets-check "my topic"
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	sheetsv4 "google.golang.org/api/sheets/v4"

	"contentflow/internal/config"
	"contentflow/internal/sheets"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg, err := config.NewConfig()
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	if cfg.Sheets.CredentialsFile == "" || cfg.Sheets.SpreadsheetID == "" {
		log.Error("sheets not configured",
			"hint", "set CF_SHEETS_SPREADSHEET_ID in contentflow/.env")
		os.Exit(1)
	}

	ctx := context.Background()

	// Step 1: list all tabs in the spreadsheet so the user can see what's there.
	data, err := os.ReadFile(cfg.Sheets.CredentialsFile)
	if err != nil {
		log.Error("read credentials", "err", err)
		os.Exit(1)
	}
	creds, err := google.CredentialsFromJSON(ctx, data, sheetsv4.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Error("parse credentials", "err", err)
		os.Exit(1)
	}
	svc, err := sheetsv4.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		log.Error("sheets service", "err", err)
		os.Exit(1)
	}

	meta, err := svc.Spreadsheets.Get(cfg.Sheets.SpreadsheetID).
		Fields("properties.title,sheets.properties(title,gridProperties)").
		Context(ctx).Do()
	if err != nil {
		log.Error("fetch metadata", "err", err)
		os.Exit(1)
	}

	fmt.Printf("Spreadsheet title: %q\n", meta.Properties.Title)
	fmt.Printf("Configured tab:    %q (cols %s,%s, header=%v)\n\n",
		cfg.Sheets.Sheet, cfg.Sheets.TopicColumn, cfg.Sheets.KeywordColumn, cfg.Sheets.HeaderRow)
	fmt.Println("Tabs found in spreadsheet:")
	for _, sh := range meta.Sheets {
		p := sh.Properties
		fmt.Printf("  - %q (%d rows × %d cols)\n",
			p.Title, p.GridProperties.RowCount, p.GridProperties.ColumnCount)
	}
	fmt.Println()

	// Step 2: dump the first 5 rows across the first 10 columns of the
	// configured tab (best-effort; ignore errors) so you can eyeball
	// the layout and pick the right topic/keyword columns.
	dumpRange := fmt.Sprintf("%s!A1:C15", cfg.Sheets.Sheet)
	if resp, derr := svc.Spreadsheets.Values.Get(cfg.Sheets.SpreadsheetID, dumpRange).
		Context(ctx).Do(); derr == nil {
		fmt.Printf("First rows of %q (A1:J5):\n", cfg.Sheets.Sheet)
		for i, row := range resp.Values {
			fmt.Printf("  row %d:", i+1)
			for j, v := range row {
				fmt.Printf("  [%c]=%q", 'A'+j, fmt.Sprint(v))
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Step 3: run the lookup via the real client, same code path as the app.
	client, err := sheets.New(ctx, cfg.Sheets, log)
	if err != nil {
		log.Error("init sheets client", "err", err)
		os.Exit(1)
	}

	topic := "как выбрать ноутбук"
	if len(os.Args) > 1 {
		topic = os.Args[1]
	}

	res, err := client.Lookup(ctx, topic)
	if err != nil {
		log.Error("lookup", "topic", topic, "err", err)
		os.Exit(1)
	}

	fmt.Printf("Lookup topic: %q\n", topic)
	if res.Title != "" {
		fmt.Printf("Title:        %s\n", res.Title)
	}
	fmt.Printf("Matches:      %d\n", len(res.Keywords))
	for _, kw := range res.Keywords {
		fmt.Printf("  - %s\n", kw)
	}
	if len(res.Keywords) == 0 {
		fmt.Println("\n(no matches — if the tab name is wrong, set CF_SHEETS_SHEET in .env)")
	}
}
