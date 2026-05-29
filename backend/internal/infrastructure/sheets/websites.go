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

var _ linkbuilding.WebsiteSource = (*websiteSource)(nil)

func NewWebsiteSource(ctx context.Context, credentialsFile, spreadsheetID string, log *slog.Logger) (linkbuilding.WebsiteSource, error) {
	if credentialsFile == "" || spreadsheetID == "" {
		return nil, fmt.Errorf("sheets: credentialsFile and spreadsheetId are required")
	}

	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	// Read-write scope so WriteResults can update the qualification columns;
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
		if i == 0 {
			continue
		}
		if len(row) == 0 {
			continue
		}
		url := strings.TrimSpace(fmt.Sprint(row[0]))
		if url == "" {
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

	data := make([]*sheets.ValueRange, 0, len(results))
	for _, r := range results {
		data = append(data, &sheets.ValueRange{
			Range: fmt.Sprintf("%s!B%d:D%d", sheet, r.Row, r.Row),
			Values: [][]any{{
				r.Topic,
				r.OutboundDomains,
				suitableCell(r.Suitable),
			}},
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
		return fmt.Errorf("batch update results in %s: %w", sheet, err)
	}

	s.log.DebugContext(ctx, "sheets websites write",
		"sheet", sheet,
		"results", len(results),
	)
	return nil
}

func suitableCell(suitable bool) string {
	if suitable {
		return "yes"
	}
	return "no"
}
