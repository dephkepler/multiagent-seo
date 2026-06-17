package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/emailscrape"
)

type emailSource struct {
	svc           *sheets.Service
	spreadsheetID string
	log           *slog.Logger
}

func NewEmailSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (emailscrape.SiteSource, error) {
	if log == nil {
		log = slog.Default()
	}
	svc, err := newSheetsService(ctx, credentialsFile, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return &emailSource{svc: svc, spreadsheetID: spreadsheetID, log: log}, nil
}

// List reads column A (URL) and C (status). Rows whose latest status is a
// success ("found …") are skipped; "no emails"/"error" rows are retried so a
// re-run re-attempts sites that didn't yield an address.
func (s *emailSource) List(ctx context.Context, sheet string) ([]emailscrape.Site, error) {
	rangeStr := fmt.Sprintf("%s!A:C", sheet)
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, rangeStr).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	var out []emailscrape.Site
	skipped := 0
	for i, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		url := strings.TrimSpace(fmt.Sprint(row[0]))
		if !strings.HasPrefix(strings.ToLower(url), "http") {
			continue
		}
		status := ""
		if len(row) >= 3 {
			status = fmt.Sprint(row[2])
		}
		if isFoundStatus(status) {
			skipped++
			continue
		}
		out = append(out, emailscrape.Site{Row: i + 1, URL: url})
	}

	s.log.DebugContext(ctx, "sheets email list",
		"sheet", sheet,
		"rows_scanned", len(resp.Values),
		"pending", len(out),
		"skipped_found", skipped,
	)
	return out, nil
}

// isFoundStatus reports whether the latest status line (column C is append-only,
// newest first) marks a successful scrape, which we don't re-run.
func isFoundStatus(cell string) bool {
	firstLine := cell
	if i := strings.IndexByte(cell, '\n'); i >= 0 {
		firstLine = cell[:i]
	}
	return strings.Contains(strings.ToLower(firstLine), "found")
}

// WriteResults overwrites column B with the found emails and prepends a
// timestamped line to the append-only status column C.
func (s *emailSource) WriteResults(ctx context.Context, sheet string, results []emailscrape.Result) error {
	if len(results) == 0 {
		return nil
	}

	statusRanges := make([]string, 0, len(results))
	for _, r := range results {
		statusRanges = append(statusRanges, fmt.Sprintf("%s!C%d", sheet, r.Row))
	}
	existing, err := s.svc.Spreadsheets.Values.BatchGet(s.spreadsheetID).Ranges(statusRanges...).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read existing status in %s: %w", sheet, err)
	}
	existingByRow := make(map[int]string, len(results))
	for i, vr := range existing.ValueRanges {
		if i >= len(results) {
			break
		}
		if len(vr.Values) > 0 && len(vr.Values[0]) > 0 {
			existingByRow[results[i].Row] = strings.TrimSpace(fmt.Sprint(vr.Values[0][0]))
		}
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	valueRanges := make([]*sheets.ValueRange, 0, len(results)*2)
	for _, r := range results {
		entry := fmt.Sprintf("[%s] %s", now, r.Status)
		if old := existingByRow[r.Row]; old != "" {
			entry = entry + "\n" + old
		}
		valueRanges = append(valueRanges,
			&sheets.ValueRange{
				Range:  fmt.Sprintf("%s!B%d", sheet, r.Row),
				Values: [][]any{{strings.Join(r.Emails, ", ")}},
			},
			&sheets.ValueRange{
				Range:  fmt.Sprintf("%s!C%d", sheet, r.Row),
				Values: [][]any{{entry}},
			},
		)
	}

	req := &sheets.BatchUpdateValuesRequest{ValueInputOption: "RAW", Data: valueRanges}
	if _, err := s.svc.Spreadsheets.Values.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("batch update emails in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets email write", "sheet", sheet, "results", len(results))
	return nil
}
