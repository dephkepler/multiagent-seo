// importcases is a one-off backfill: pulls the historical "проекти" tab —
// a log of case updates, not one row per case — into the cases table.
//
// The sheet has no header row and no fixed case-per-row shape: the same
// case (same leading ID) reappears across several rows as it progresses
// (negotiation → partial payment → done), interleaved with unrelated
// "Консульт"-tagged rows that are a different thing entirely (a
// consultation, already covered by import55k — importing those here too
// would double-count consultation revenue). So this tool:
//  1. keeps only rows tagged "проект" (column 2), dropping "Консульт" rows
//  2. groups the survivors by the leading ID (column 0) — one case per group
//  3. takes the fee/advocate/status from the group's most recent row (the
//     latest figure is the closest thing to "current"); paid_amount is set
//     to the fee if the case's own status is "Выполнил" (assumed settled on
//     completion), 0 otherwise — deliberately conservative: guessing a
//     partial figure from free-text notes ("half now, half later") risks
//     fabricating a number nobody can verify, undercounting is the safer
//     direction, staff correct it going forward with /pay
//  4. links each case to the client's own most recent consultation at or
//     before the case's start date, if one was already imported by import55k
//
// Usage:
//
//	go run ./cmd/importcases          # dry run, prints what it would do
//	go run ./cmd/importcases --commit # actually writes
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	domainleads "multiagent-seo/internal/domain/webleads"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/pkg/config"
)

type sheetRow struct {
	kind     string // "проект" or "Консульт" (column 2)
	status   string // column 1 — "В работе" / "Выполнил"
	date     time.Time
	admin    string
	phone    string
	name     string
	info     string // column 8, the original inquiry text
	advocate string // column 16
	fee      float64
	noteMid  string // column 11 — short progress note
	noteLong string // column 19 — longer progress note
}

func main() {
	commit := flag.Bool("commit", false, "actually write to the database (default: dry run, no writes)")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.LeadsSheets.SpreadsheetID == "" {
		log.Fatal("CF_LEADS_SHEETS_SPREADSHEET_ID is empty")
	}

	byID, err := fetchGroups(ctx, cfg)
	if err != nil {
		log.Fatalf("fetch sheet: %v", err)
	}
	rowCount := 0
	for _, rs := range byID {
		rowCount += len(rs)
	}
	fmt.Printf("fetched %d 'проект' rows across %d cases with a parseable date and id (Консульт rows skipped)\n", rowCount, len(byID))

	var order []string
	for id := range byID {
		order = append(order, id)
	}
	sort.Strings(order)

	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer database.Close()
	pool := database.Pool()

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	var inserted, skippedDup, skippedNoPhone int

	for _, id := range order {
		group := byID[id]
		sort.Slice(group, func(i, j int) bool { return group[i].date.Before(group[j].date) })
		first, last := group[0], group[len(group)-1]

		phone := domainleads.NormalizePhone(first.phone)
		if strings.TrimSpace(phone) == "" {
			skippedNoPhone++
			continue
		}

		clientID, err := upsertHistoricalClient(ctx, tx, phone, first.name, first.date)
		if err != nil {
			log.Fatalf("case %s: upsert client: %v", id, err)
		}

		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM cases WHERE client_id = @client_id AND created_at = @created_at)`,
			pgx.NamedArgs{"client_id": clientID, "created_at": atNoon(first.date)},
		).Scan(&exists); err != nil {
			log.Fatalf("case %s: check existing: %v", id, err)
		}
		if exists {
			skippedDup++
			continue
		}

		status := "in_progress"
		paid := 0.0
		if last.status == "Выполнил" {
			status = "completed"
			paid = last.fee
		}

		consultationID, _ := findConsultation(ctx, tx, clientID, first.date)

		var notes []string
		for _, r := range group {
			var parts []string
			if r.noteMid != "" {
				parts = append(parts, r.noteMid)
			}
			if r.noteLong != "" && r.noteLong != r.noteMid {
				parts = append(parts, r.noteLong)
			}
			if len(parts) > 0 {
				notes = append(notes, r.date.Format("02.01.06")+": "+strings.Join(parts, " — "))
			}
		}
		description := strings.TrimSpace(first.info)
		if len(notes) > 0 {
			description += "\n\nХід справи:\n" + strings.Join(notes, "\n")
		}
		description += fmt.Sprintf("\n\n[Імпорт з «проекти», №%s, %d записів. Сума оплати визначена консервативно: fee якщо статус «Виконав», інакше 0 — потребує звірки.]", id, len(group))

		var consultationArg any
		if consultationID != "" {
			consultationArg = consultationID
		}

		// paid_amount is a running total, and case_payments is the ledger behind
		// it. Writing the total without a ledger row makes the same money visible
		// to whatever reads the column (leadstats) and invisible to whatever reads
		// the ledger (the finance P&L), so both go in together.
		var caseID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO cases (client_id, consultation_id, advocate_name, fee, paid_amount, status, description, created_by, created_at)
			VALUES (@client_id, @consultation_id, @advocate_name, @fee, @paid_amount, @status, @description, @created_by, @created_at)
			RETURNING id`,
			pgx.NamedArgs{
				"client_id":       clientID,
				"consultation_id": consultationArg,
				"advocate_name":   last.advocate,
				"fee":             last.fee,
				"paid_amount":     paid,
				"status":          status,
				"description":     description,
				"created_by":      "import:" + strings.TrimSpace(first.admin),
				"created_at":      atNoon(first.date),
			}).Scan(&caseID); err != nil {
			log.Fatalf("case %s: insert: %v", id, err)
		}
		if paid > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO case_payments (case_id, amount, paid_at, created_by)
				VALUES (@case_id, @amount, @paid_at, @created_by)`,
				pgx.NamedArgs{
					"case_id":    caseID,
					"amount":     paid,
					"paid_at":    atNoon(first.date),
					"created_by": "import:" + strings.TrimSpace(first.admin),
				}); err != nil {
				log.Fatalf("case %s: insert payment: %v", id, err)
			}
		}
		inserted++
	}

	fmt.Println()
	fmt.Printf("cases inserted:          %d\n", inserted)
	fmt.Printf("cases already present:   %d (skipped)\n", skippedDup)
	fmt.Printf("skipped (no phone):      %d\n", skippedNoPhone)

	if !*commit {
		fmt.Println("\nDRY RUN — nothing written. Re-run with --commit to apply.")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	fmt.Println("\nCOMMITTED.")
}

func findConsultation(ctx context.Context, tx pgx.Tx, clientID string, before time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM consultations
		WHERE client_id = @client_id AND scheduled_at <= @before
		ORDER BY scheduled_at DESC LIMIT 1`,
		pgx.NamedArgs{"client_id": clientID, "before": atNoon(before).Add(24 * time.Hour)},
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// upsertHistoricalClient mirrors the one in import55k — historical
// first/last-seen bounds via LEAST/GREATEST, not now().
func upsertHistoricalClient(ctx context.Context, tx pgx.Tx, phone, name string, at time.Time) (string, error) {
	ts := atNoon(at)
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO clients (phone, name, first_seen_at, last_seen_at)
		VALUES (@phone, @name, @at, @at)
		ON CONFLICT (phone) DO UPDATE SET
			name = CASE WHEN clients.name = '' THEN EXCLUDED.name ELSE clients.name END,
			first_seen_at = LEAST(clients.first_seen_at, EXCLUDED.first_seen_at),
			last_seen_at = GREATEST(clients.last_seen_at, EXCLUDED.last_seen_at)
		RETURNING id`,
		pgx.NamedArgs{"phone": phone, "name": name, "at": ts},
	).Scan(&id)
	return id, err
}

func atNoon(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
}

func fetchGroups(ctx context.Context, cfg config.Config) (map[string][]sheetRow, error) {
	credsJSON, err := os.ReadFile(cfg.Sheets.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	creds, err := google.CredentialsFromJSON(ctx, credsJSON, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	svc, err := sheets.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}
	resp, err := svc.Spreadsheets.Values.Get(cfg.LeadsSheets.SpreadsheetID, "проекти!A1:V500").Do()
	if err != nil {
		return nil, fmt.Errorf("read проекти tab: %w", err)
	}

	col := func(r []interface{}, i int) string {
		if i < len(r) {
			if s, ok := r[i].(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}

	collector := map[string][]sheetRow{}
	for _, r := range resp.Values {
		if len(r) == 0 {
			continue
		}
		if col(r, 2) != "проект" {
			continue // skip "Консульт" rows — see package doc
		}
		id := col(r, 0)
		if id == "" {
			continue
		}
		dateStr := col(r, 3)
		t, perr := time.Parse("02.01.06", dateStr)
		if perr != nil {
			continue
		}
		fee := 0.0
		feeStr := strings.ReplaceAll(strings.ReplaceAll(col(r, 13), " ", ""), ",", ".")
		if feeStr != "" {
			if v, err := strconv.ParseFloat(feeStr, 64); err == nil {
				fee = v
			}
		}
		row := sheetRow{
			kind:     col(r, 2),
			status:   col(r, 1),
			date:     t,
			admin:    col(r, 5),
			phone:    col(r, 6),
			name:     col(r, 7),
			info:     col(r, 8),
			advocate: col(r, 16),
			fee:      fee,
			noteMid:  col(r, 11),
			noteLong: col(r, 19),
		}
		collector[id] = append(collector[id], row)
	}
	return collector, nil
}
