package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

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

func NewCredentialSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (linkbuilding.CredentialSource, error) {
	src, err := newSource(ctx, credentialsFile, spreadsheetID, log)
	if err != nil {
		return nil, err
	}
	return src, nil
}

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
	if log == nil {
		log = slog.Default()
	}

	credsJSON, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	creds, err := google.CredentialsFromJSON(ctx, credsJSON, sheets.SpreadsheetsScope)
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

	valueRanges := make([]*sheets.ValueRange, 0, len(results)*2)
	for _, r := range results {
		valueRanges = append(valueRanges, &sheets.ValueRange{
			Range: resultRange(sheet, r.Row),
			Values: [][]any{{
				r.Topic,
				r.OutboundDomains,
				suitableCell(r.Suitable),
			}},
		})
		if !r.Suitable {
			valueRanges = append(valueRanges, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!H%d", sheet, r.Row),
				Values: [][]any{{""}},
			})
		}
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             valueRanges,
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
	aRange := fmt.Sprintf("%s!A:D", sheet)
	aResp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, aRange).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", aRange, err)
	}
	aVerdicts := parseAVerdicts(aResp.Values)

	eRange := fmt.Sprintf("%s!E:I", sheet)
	eResp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, eRange).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", eRange, err)
	}
	out, rejectedUnknown, rejectedNotSuitable := parseECredentialsJoin(eResp.Values, aVerdicts)

	s.log.InfoContext(ctx, "sheets credentials list",
		"sheet", sheet,
		"a_qualified", len(aVerdicts),
		"e_rows_scanned", len(eResp.Values),
		"credentials", len(out),
		"rejected_url_not_in_a", rejectedUnknown,
		"rejected_not_suitable", rejectedNotSuitable,
	)
	return out, nil
}

func (s *websiteSource) WriteLoginStatus(ctx context.Context, sheet string, results []linkbuilding.LoginResult) error {
	if len(results) == 0 {
		return nil
	}

	valueRanges := make([]*sheets.ValueRange, 0, len(results))
	for _, r := range results {
		valueRanges = append(valueRanges, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!H%d", sheet, r.Row),
			Values: [][]any{{r.Status}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             valueRanges,
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

	ranges := make([]string, 0, len(results))
	for _, r := range results {
		ranges = append(ranges, fmt.Sprintf("%s!I%d", sheet, r.Row))
	}
	existing, err := s.svc.Spreadsheets.Values.BatchGet(s.spreadsheetID).Ranges(ranges...).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read existing placement in %s: %w", sheet, err)
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
	valueRanges := make([]*sheets.ValueRange, 0, len(results))
	for _, r := range results {
		entry := fmt.Sprintf("[%s] %s", now, r.Status)
		if old := existingByRow[r.Row]; old != "" {
			entry = entry + "\n" + old
		}
		valueRanges = append(valueRanges, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!I%d", sheet, r.Row),
			Values: [][]any{{entry}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             valueRanges,
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
	aResp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, fmt.Sprintf("%s!A:D", sheet)).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch range %s!A:D: %w", sheet, err)
	}
	aVerdicts := parseAVerdicts(aResp.Values)

	eResp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, fmt.Sprintf("%s!E:H", sheet)).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch range %s!E:H: %w", sheet, err)
	}
	rows := staleEStatusRows(eResp.Values, aVerdicts)
	if len(rows) == 0 {
		return nil
	}

	valueRanges := make([]*sheets.ValueRange, 0, len(rows))
	for _, row := range rows {
		valueRanges = append(valueRanges, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!H%d", sheet, row),
			Values: [][]any{{""}},
		})
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             valueRanges,
	}
	if _, err := s.svc.Spreadsheets.Values.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("clear stale statuses in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets stale statuses cleared", "sheet", sheet, "cleared", len(rows))
	return nil
}

type qualVerdict struct {
	Topic    string
	Suitable bool
}

func parseAVerdicts(values [][]any) map[string]qualVerdict {
	out := make(map[string]qualVerdict, len(values))
	for _, row := range values {
		url := cell(row, 0)
		if !strings.HasPrefix(strings.ToLower(url), "http") {
			continue
		}
		topic := cell(row, 1)
		suitable := strings.ToLower(cell(row, 3))
		out[normalizeURL(url)] = qualVerdict{
			Topic:    topic,
			Suitable: suitable == "yes",
		}
	}
	return out
}

func parseECredentialsJoin(values [][]any, aVerdicts map[string]qualVerdict) (out []linkbuilding.SiteCredential, rejectedUnknown, rejectedNotSuitable int) {
	for i, row := range values {
		base := cell(row, 0)
		login := cell(row, 1)
		password := cell(row, 2)
		loginStatus := cell(row, 3)
		placementStatus := cell(row, 4)
		if !strings.HasPrefix(strings.ToLower(base), "http") || login == "" || password == "" {
			continue
		}
		v, ok := aVerdicts[normalizeURL(base)]
		if !ok {
			rejectedUnknown++
			continue
		}
		if !v.Suitable || v.Topic == "" {
			rejectedNotSuitable++
			continue
		}
		out = append(out, linkbuilding.SiteCredential{
			Row:             i + 1,
			BaseURL:         base,
			Login:           login,
			Password:        password,
			Topic:           v.Topic,
			LoginStatus:     loginStatus,
			PlacementStatus: placementStatus,
		})
	}
	return out, rejectedUnknown, rejectedNotSuitable
}

func cell(row []any, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[idx]))
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "/")
	return s
}

func staleEStatusRows(values [][]any, aVerdicts map[string]qualVerdict) []int {
	var rows []int
	for i, row := range values {
		base := cell(row, 0)
		status := cell(row, 3)
		if !strings.HasPrefix(strings.ToLower(base), "http") || status == "" {
			continue
		}
		v, ok := aVerdicts[normalizeURL(base)]
		if ok && v.Suitable {
			continue
		}
		rows = append(rows, i+1)
	}
	return rows
}
