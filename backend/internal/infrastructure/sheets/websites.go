package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/linkbuilding"
)

type websiteSource struct {
	svc           *sheets.Service
	spreadsheetID string
	log           *slog.Logger
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
	if log == nil {
		log = slog.Default()
	}
	svc, err := newSheetsService(ctx, credentialsFile, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return &websiteSource{
		svc:           svc,
		spreadsheetID: spreadsheetID,
		log:           log,
	}, nil
}

func (s *websiteSource) ListCredentials(ctx context.Context, sheet string) ([]linkbuilding.SiteCredential, error) {
	rangeStr := colRange(sheet, colCredBase, colPlacement)
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, rangeStr).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}
	out := parseCredentials(resp.Values)

	s.log.InfoContext(ctx, "sheets credentials list",
		"sheet", sheet,
		"rows_scanned", len(resp.Values),
		"credentials", len(out),
	)
	return out, nil
}

func (s *websiteSource) WritePlacementStatus(ctx context.Context, sheet string, results []linkbuilding.PlacementResult) error {
	if len(results) == 0 {
		return nil
	}

	ranges := make([]string, 0, len(results))
	for _, r := range results {
		ranges = append(ranges, colCell(sheet, colPlacement, r.Row))
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

	now := time.Now().UTC()
	stamp := now.Format("2006-01-02 15:04:05")
	date := now.Format("2006-01-02")
	valueRanges := make([]*sheets.ValueRange, 0, len(results)*3)
	for _, r := range results {
		entry := fmt.Sprintf("[%s] %s", stamp, r.Status)
		if old := existingByRow[r.Row]; old != "" {
			entry = entry + "\n" + old
		}
		valueRanges = append(valueRanges,
			&sheets.ValueRange{
				Range:  colCell(sheet, colPlacement, r.Row),
				Values: [][]any{{entry}},
			},
			&sheets.ValueRange{
				Range:  colCell(sheet, colLoginStatus, r.Row),
				Values: [][]any{{linkbuilding.ShortStatus(r.OK, r.Outcome, date)}},
			},
		)
		if r.Rights != "" {
			valueRanges = append(valueRanges, &sheets.ValueRange{
				Range:  colCell(sheet, colRights, r.Row),
				Values: [][]any{{r.Rights}},
			})
		}
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

func parseCredentials(values [][]any) []linkbuilding.SiteCredential {
	var out []linkbuilding.SiteCredential
	for i, row := range values {
		base := cell(row, 0)
		login := cell(row, 1)
		password := cell(row, 2)
		loginStatus := cell(row, 3)
		placementStatus := cell(row, 4)
		if !strings.HasPrefix(strings.ToLower(base), "http") || login == "" || password == "" {
			continue
		}
		out = append(out, linkbuilding.SiteCredential{
			Row:             i + 1,
			BaseURL:         base,
			Login:           login,
			Password:        password,
			LoginStatus:     loginStatus,
			PlacementStatus: placementStatus,
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
