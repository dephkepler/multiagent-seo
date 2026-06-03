package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/linkbuilding"
)

type websiteSource struct {
	svc           *sheets.Service
	spreadsheetID string
	log           *slog.Logger
}

func NewWebsiteSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (linkbuilding.WebsiteSource, error) {
	src, err := newSource(ctx, credentialsFile, spreadsheetID, log)
	if err != nil {
		return nil, err
	}
	return src, nil
}

// NewCredentialSource builds the same Google Sheets adapter for the Flow 2
// login inventory: it reads the credential columns and writes the login status.
func NewCredentialSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (linkbuilding.CredentialSource, error) {
	src, err := newSource(ctx, credentialsFile, spreadsheetID, log)
	if err != nil {
		return nil, err
	}
	return src, nil
}

// NewPlacementSink builds the same Google Sheets adapter as a Flow 3 placement
// sink — it writes per-donor "placed / failed" rows back to the I column.
func NewPlacementSink(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (linkbuilding.PlacementSink, error) {
	src, err := newSource(ctx, credentialsFile, spreadsheetID, log)
	if err != nil {
		return nil, err
	}
	return src, nil
}

func newSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (*websiteSource, error) {
	if credentialsFile == "" || spreadsheetID == "" {
		return nil, fmt.Errorf("sheets: credentialsFile and spreadsheetId are required")
	}

	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	// Read-write scope so the result/status writes can update the sheet;
	// the keyword client (client.go) stays read-only.
	creds, err := google.CredentialsFromJSON(ctx, data, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	svc, err := sheets.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &websiteSource{
		svc:           svc,
		spreadsheetID: spreadsheetID,
		log:           log,
	}, nil
}

func (s *websiteSource) List(ctx context.Context, sheet string) ([]linkbuilding.Website, error) {
	rangeStr := fmt.Sprintf("%s!A:A", sheet)

	resp, err := s.svc.Spreadsheets.Values.
		Get(s.spreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	var out []linkbuilding.Website
	for i, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		url := strings.TrimSpace(fmt.Sprint(row[0]))
		// Only rows whose first cell is a URL count — this skips blanks and a
		// header row like "URL"/"Website" without assuming row 1 is a header
		// (the donor list often starts at row 1 with no header).
		if !strings.HasPrefix(strings.ToLower(url), "http") {
			continue
		}
		out = append(out, linkbuilding.Website{
			Row: i + 1,
			URL: url,
		})
	}

	s.log.DebugContext(ctx, "sheets websites list",
		"sheet", sheet,
		"rows_scanned", len(resp.Values),
		"websites", len(out),
	)
	return out, nil
}

func (s *websiteSource) WriteResults(ctx context.Context, sheet string, results []linkbuilding.Result) error {
	if len(results) == 0 {
		return nil
	}

	data := make([]*sheets.ValueRange, 0, len(results)*2)
	for _, r := range results {
		data = append(data, &sheets.ValueRange{
			Range: resultRange(sheet, r.Row),
			Values: [][]any{{
				r.Topic,
				r.OutboundDomains,
				suitableCell(r.Suitable),
			}},
		})
		// On a fresh "not suitable" verdict, blank H and I so a stale Flow 2
		// "login ok" or Flow 3 "placed: ..." from a previous campaign doesn't
		// linger on a row we just decided is out of scope. Suitable rows leave
		// H/I untouched — those statuses are managed by Flow 2/3 themselves.
		if !r.Suitable {
			data = append(data, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!H%d:I%d", sheet, r.Row, r.Row),
				Values: [][]any{{"", ""}},
			})
		}
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             data,
	}

	_, err := s.svc.Spreadsheets.Values.
		BatchUpdate(s.spreadsheetID, req).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("batch update results in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets websites write",
		"sheet", sheet,
		"results", len(results),
	)
	return nil
}

// resultRange is the A1 range for a single result's qualification columns (B:D).
func resultRange(sheet string, row int) string {
	return fmt.Sprintf("%s!B%d:D%d", sheet, row, row)
}

func suitableCell(suitable bool) string {
	if suitable {
		return "yes"
	}
	return "no"
}

func (s *websiteSource) ListCredentials(ctx context.Context, sheet string) ([]linkbuilding.SiteCredential, error) {
	// B:H covers the Flow 1 topic (B) — surfaced for optional per-campaign
	// topic filtering downstream — through suitability (D), credentials (E/F/G),
	// and the prior Flow 2 login status (H). Column C (outbound count) is read
	// but discarded.
	rangeStr := fmt.Sprintf("%s!B:H", sheet)

	resp, err := s.svc.Spreadsheets.Values.
		Get(s.spreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	out := parseCredentialRows(resp.Values)
	// Bumped from Debug to Info plus emit the actual row numbers that passed
	// the suitable=yes filter — lets the user verify against the sheet exactly
	// which rows we considered, without having to scroll the placement log.
	rows := make([]int, 0, len(out))
	for _, c := range out {
		rows = append(rows, c.Row)
	}
	s.log.InfoContext(ctx, "sheets credentials list",
		"sheet", sheet,
		"rows_scanned", len(resp.Values),
		"credentials", len(out),
		"included_rows", rows,
	)
	return out, nil
}

func (s *websiteSource) WriteLoginStatus(ctx context.Context, sheet string, results []linkbuilding.LoginResult) error {
	if len(results) == 0 {
		return nil
	}

	data := make([]*sheets.ValueRange, 0, len(results))
	for _, r := range results {
		data = append(data, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!H%d", sheet, r.Row),
			Values: [][]any{{r.Status}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             data,
	}

	_, err := s.svc.Spreadsheets.Values.
		BatchUpdate(s.spreadsheetID, req).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("batch update login status in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets login status write",
		"sheet", sheet,
		"results", len(results),
	)
	return nil
}

func (s *websiteSource) WritePlacementStatus(ctx context.Context, sheet string, results []linkbuilding.PlacementResult) error {
	if len(results) == 0 {
		return nil
	}

	data := make([]*sheets.ValueRange, 0, len(results))
	for _, r := range results {
		data = append(data, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!I%d", sheet, r.Row),
			Values: [][]any{{r.Status}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             data,
	}
	if _, err := s.svc.Spreadsheets.Values.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("batch update placement status in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets placement status write",
		"sheet", sheet,
		"results", len(results),
	)
	return nil
}

func (s *websiteSource) ClearStaleStatuses(ctx context.Context, sheet string) error {
	rangeStr := fmt.Sprintf("%s!D:H", sheet)

	resp, err := s.svc.Spreadsheets.Values.
		Get(s.spreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	rows := staleStatusRows(resp.Values)
	if len(rows) == 0 {
		return nil
	}

	data := make([]*sheets.ValueRange, 0, len(rows))
	for _, row := range rows {
		data = append(data, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!H%d", sheet, row),
			Values: [][]any{{""}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             data,
	}
	if _, err := s.svc.Spreadsheets.Values.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("clear stale statuses in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets stale statuses cleared", "sheet", sheet, "cleared", len(rows))
	return nil
}

// parseCredentialRows turns rows from the B:H range into credentials. Column
// offsets: 0=topic (B), 1=outbound (C, unused), 2=suitable (D), 3=base URL (E),
// 4=login (F), 5=password (G), 6=prior login status (H). Only rows the
// qualification marked suitable ("yes") and that carry a URL + login + password
// are kept; the header row and unsuitable/incomplete rows are skipped. Row is
// the 1-based sheet line so the status can be written back.
func parseCredentialRows(values [][]any) []linkbuilding.SiteCredential {
	var out []linkbuilding.SiteCredential
	for i, row := range values {
		topic := cell(row, 0)
		suitable := strings.ToLower(cell(row, 2))
		base := cell(row, 3)
		login := cell(row, 4)
		password := cell(row, 5)
		loginStatus := cell(row, 6)
		if suitable != "yes" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(base), "http") || login == "" || password == "" {
			continue
		}
		out = append(out, linkbuilding.SiteCredential{
			Row:         i + 1,
			BaseURL:     base,
			Login:       login,
			Password:    password,
			Topic:       topic,
			LoginStatus: loginStatus,
		})
	}
	return out
}

func cell(row []any, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[idx]))
}

// staleStatusRows returns the 1-based rows of the D:H range whose status (col H,
// offset 4) is stale and should be blanked: a real site row (E is a URL) that is
// no longer suitable (col D, offset 0) yet still carries a leftover status. The
// header and blank rows (E not a URL) and suitable rows are left untouched.
func staleStatusRows(values [][]any) []int {
	var rows []int
	for i, row := range values {
		suitable := strings.ToLower(cell(row, 0))
		base := strings.ToLower(cell(row, 1))
		status := cell(row, 4)
		if strings.HasPrefix(base, "http") && suitable != "yes" && status != "" {
			rows = append(rows, i+1)
		}
	}
	return rows
}
