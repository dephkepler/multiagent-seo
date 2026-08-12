// import55k is a one-off backfill: pulls the historical "55k" manual-CRM
// tab (Aug 2024 – Jan 2025, pre-dates the bot) into the same leads/clients/
// consultations tables the live bot writes to, so the admin dashboard can
// show history alongside data the bot collects going forward.
//
// Read-only against Sheets, dry-run by default against Postgres. Safe to
// re-run: leads dedupe on message_id (existing ON CONFLICT DO NOTHING),
// consultations dedupe on (client_id, scheduled_at), clients dedupe on
// phone with historical-aware first/last-seen bounds (not "now()", unlike
// the live ResolveClient path — that would mislabel 2024 clients as "seen
// today").
//
// Usage:
//
//	go run ./cmd/import55k              # dry run, prints what it would do
//	go run ./cmd/import55k --commit     # actually writes
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
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

type row struct {
	num      int
	date     time.Time
	hasDate  bool
	admin    string
	advocate string
	clStatus string
	kcStatus string
	amount   float64
	name     string
	phone    string
	info     string
	source   string // "где нашли"
	kcTime   string // "ВРЕМЯ к-ц"
	kcDate   string // "Дт/к-ц"
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
		log.Fatal("CF_LEADS_SHEETS_SPREADSHEET_ID is empty — same spreadsheet the bot uses, needed to find the 55k tab")
	}

	rows, err := fetchRows(ctx, cfg)
	if err != nil {
		log.Fatalf("fetch sheet: %v", err)
	}
	fmt.Printf("fetched %d rows with a parseable date (out of the full 55k tab)\n", len(rows))

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
	defer tx.Rollback(ctx) // no-op after a successful commit below

	stats := runImport(ctx, tx, rows)

	fmt.Println()
	fmt.Printf("clients touched:        %d\n", stats.clients)
	fmt.Printf("leads inserted:         %d (already present, skipped: %d)\n", stats.leadsInserted, stats.leadsSkippedDup)
	fmt.Printf("leads skipped (no phone): %d\n", stats.leadsSkippedNoPhone)
	fmt.Printf("consultations inserted: %d (already present, skipped: %d)\n", stats.consultInserted, stats.consultSkippedDup)

	if !*commit {
		fmt.Println("\nDRY RUN — nothing written. Re-run with --commit to apply.")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	fmt.Println("\nCOMMITTED.")
}

type importStats struct {
	clients             int
	leadsInserted       int
	leadsSkippedDup     int
	leadsSkippedNoPhone int
	consultInserted     int
	consultSkippedDup   int
}

func runImport(ctx context.Context, tx pgx.Tx, rows []row) importStats {
	var stats importStats
	seenPhones := map[string]bool{}

	for _, r := range rows {
		phone := domainleads.NormalizePhone(r.phone)
		if strings.TrimSpace(phone) == "" {
			stats.leadsSkippedNoPhone++
			continue
		}

		clientID, err := upsertHistoricalClient(ctx, tx, phone, r.name, r.date)
		if err != nil {
			log.Fatalf("row %d: upsert client: %v", r.num, err)
		}
		if !seenPhones[phone] {
			seenPhones[phone] = true
			stats.clients++
		}

		messageID := fmt.Sprintf("55k-import-%d@abalis.local", r.num)
		message := buildLeadMessage(r)
		tag, err := tx.Exec(ctx, `
			INSERT INTO leads (message_id, received_at, from_email, subject, name, phone, message, page, raw_body, client_id, sheet_synced_at, created_at)
			VALUES (@message_id, @received_at, '', '', @name, @phone, @message, '', '', @client_id, @received_at, @received_at)
			ON CONFLICT (message_id) DO NOTHING`,
			pgx.NamedArgs{
				"message_id":  messageID,
				"received_at": atNoon(r.date),
				"name":        r.name,
				"phone":       phone,
				"message":     message,
				"client_id":   clientID,
			})
		if err != nil {
			log.Fatalf("row %d: insert lead: %v", r.num, err)
		}
		if tag.RowsAffected() > 0 {
			stats.leadsInserted++
		} else {
			stats.leadsSkippedDup++
		}

		if r.kcStatus == "" {
			continue // never got as far as a scheduled consultation
		}
		scheduledAt := consultationTime(r)
		caseNote := buildCaseNote(r)
		status := mapConsultationStatus(r.kcStatus)
		var existingID string
		err = tx.QueryRow(ctx,
			`SELECT id FROM consultations WHERE client_id = @client_id AND scheduled_at = @scheduled_at`,
			pgx.NamedArgs{"client_id": clientID, "scheduled_at": scheduledAt},
		).Scan(&existingID)
		switch {
		case err == nil:
			// Already imported (safe re-run) — bring its status up to date in
			// case this run has a corrected sheet read, but don't touch
			// anything else (case_note etc. was fine the first time).
			if _, err := tx.Exec(ctx, `UPDATE consultations SET status = @status WHERE id = @id`,
				pgx.NamedArgs{"status": status, "id": existingID}); err != nil {
				log.Fatalf("row %d: update consultation status: %v", r.num, err)
			}
			stats.consultSkippedDup++
		case errors.Is(err, pgx.ErrNoRows):
			createdBy := "import:" + strings.TrimSpace(r.admin)
			if _, err := tx.Exec(ctx, `
				INSERT INTO consultations (client_id, scheduled_at, price, case_note, created_by, created_at, status)
				VALUES (@client_id, @scheduled_at, @price, @case_note, @created_by, @created_at, @status)`,
				pgx.NamedArgs{
					"client_id":    clientID,
					"scheduled_at": scheduledAt,
					"price":        r.amount,
					"case_note":    caseNote,
					"created_by":   createdBy,
					"created_at":   atNoon(r.date),
					"status":       status,
				}); err != nil {
				log.Fatalf("row %d: insert consultation: %v", r.num, err)
			}
			stats.consultInserted++
		default:
			log.Fatalf("row %d: check existing consultation: %v", r.num, err)
		}
	}
	return stats
}

// mapConsultationStatus translates the sheet's free-text "Статус к-ц" into
// the four-value status the live schema uses. "Не состоялась" and "х
// прозвон" (missed call trying to reach the client) both mean the slot
// never actually happened — no_show is the closest of the four buckets,
// though the sheet doesn't distinguish "client didn't show" from "we
// couldn't reach them to begin with". Anything else in-flight (В работе,
// Ожидает, blank) is left at the DB default, scheduled.
func mapConsultationStatus(raw string) string {
	switch raw {
	case "Отмена":
		return "cancelled"
	case "Выполнил":
		return "completed"
	case "Не состоялась", "х прозвон":
		return "no_show"
	default:
		return "scheduled"
	}
}

// upsertHistoricalClient mirrors LeadRepository.ResolveClient's dedupe-by-phone
// logic, but keeps first/last-seen anchored to the row's own historical date
// instead of now() — the live path is for real-time leads, not backfills.
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

// consultationTime uses the sheet's own "Дт/к-ц" + "ВРЕМЯ к-ц" when present
// (reliably filled for "Выполнил" rows); falls back to the lead's own date
// at noon otherwise — most "Отмена"/"Не состоялась" rows never had a slot
// recorded at all, so there is nothing more precise to fall back to.
func consultationTime(r row) time.Time {
	if r.kcDate != "" {
		if d, err := time.Parse("02.01.06", r.kcDate); err == nil {
			hh, mm := 12, 0
			if r.kcTime != "" {
				if t, err := time.Parse("15:04", r.kcTime); err == nil {
					hh, mm = t.Hour(), t.Minute()
				}
			}
			return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.UTC)
		}
	}
	return atNoon(r.date)
}

func buildLeadMessage(r row) string {
	var b strings.Builder
	if r.info != "" {
		b.WriteString(r.info)
	}
	if r.clStatus != "" {
		fmt.Fprintf(&b, "\n\nСтатус клиента (импорт 55k): %s", r.clStatus)
	}
	if r.source != "" {
		fmt.Fprintf(&b, "\nГде нашли (импорт 55k): %s", r.source)
	}
	return strings.TrimSpace(b.String())
}

func buildCaseNote(r row) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[Импорт 55k, №%d]", r.num))
	parts = append(parts, "Статус к-ц: "+r.kcStatus)
	if r.clStatus != "" {
		parts = append(parts, "Статус клт: "+r.clStatus)
	}
	if r.advocate != "" {
		parts = append(parts, "Адвокат: "+r.advocate)
	}
	if r.kcDate == "" {
		parts = append(parts, "(точное время консультации неизвестно, дата приблизительная)")
	}
	return strings.Join(parts, ". ")
}

func fetchRows(ctx context.Context, cfg config.Config) ([]row, error) {
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
	resp, err := svc.Spreadsheets.Values.Get(cfg.LeadsSheets.SpreadsheetID, "55k!A1:X1096").Do()
	if err != nil {
		return nil, fmt.Errorf("read 55k tab: %w", err)
	}

	col := func(r []interface{}, i int) string {
		if i < len(r) {
			if s, ok := r[i].(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}

	var out []row
	for i, r := range resp.Values[1:] {
		if len(r) == 0 {
			continue
		}
		rr := row{
			num:      i + 2, // 1-indexed + header row
			admin:    col(r, 5),
			advocate: col(r, 17),
			clStatus: col(r, 2),
			kcStatus: col(r, 1),
			name:     col(r, 7),
			phone:    col(r, 6),
			info:     col(r, 8),
			source:   col(r, 9),
			kcTime:   col(r, 18),
			kcDate:   col(r, 19),
		}
		if dateStr := col(r, 3); dateStr != "" {
			if t, err := time.Parse("02.01.06", dateStr); err == nil {
				rr.date, rr.hasDate = t, true
			} else if t, err := time.Parse("02.01.2006", dateStr); err == nil {
				rr.date, rr.hasDate = t, true
			}
		}
		if !rr.hasDate {
			continue
		}
		sumStr := strings.ReplaceAll(strings.ReplaceAll(col(r, 13), " ", ""), ",", ".")
		if sumStr != "" {
			if v, err := strconv.ParseFloat(sumStr, 64); err == nil && v > 0 {
				rr.amount = v
			}
		}
		out = append(out, rr)
	}
	return out, nil
}
