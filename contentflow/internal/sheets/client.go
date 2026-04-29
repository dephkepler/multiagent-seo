// Package sheets reads a keyword cluster for an article topic from a Google
// Sheets table. The table has two required columns (topic, keyword) and one
// optional column (title). One topic may repeat across many rows — each row
// contributes a keyword. The title (if present) is taken from the first
// matching row.
//
// Expected layout (configurable):
//
//	| A: topic          | B: keyword                  | C: title (optional)            |
//	| high flyer casino | high flyer casino           | High Flyer Casino Review: ...  |
//	| high flyer casino | high flyer casino ontario   | High Flyer Casino Review: ...  |
//	| canplay casino    | canplay casino              | CanPlay Casino Review: ...     |
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

	"contentflow/internal/config"
)

// Result is the data pulled for a given topic: every matching keyword plus an
// optional suggested article title (from the title column if configured).
type Result struct {
	Keywords []string
	Title    string
}

// Client looks up a keyword cluster for an article topic.
type Client interface {
	// Lookup returns Result.Keywords with every keyword whose topic column
	// matches (case-insensitive, trimmed) and Result.Title with the first
	// non-empty title among matching rows (if the title column is configured).
	// Empty Keywords with nil error means "no cluster for this topic".
	Lookup(ctx context.Context, topic string) (Result, error)
}

type client struct {
	svc *sheets.Service
	cfg config.SheetsConfig
	log *slog.Logger
}

// New builds a live Google Sheets client. Returns an error when credentials
// or spreadsheet ID are missing so callers can fall back to the mock.
func New(ctx context.Context, cfg config.SheetsConfig, log *slog.Logger) (Client, error) {
	if cfg.CredentialsFile == "" || cfg.SpreadsheetID == "" {
		return nil, fmt.Errorf("sheets: credentialsFile and spreadsheetId are required")
	}

	data, err := os.ReadFile(cfg.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	creds, err := google.CredentialsFromJSON(ctx, data, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	svc, err := sheets.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &client{svc: svc, cfg: cfg, log: log}, nil
}

// Lookup reads the topic/keyword/title columns for the configured sheet and
// returns all matching entries.
func (c *client) Lookup(ctx context.Context, topic string) (Result, error) {
	topic = normalize(topic)
	if topic == "" {
		return Result{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build a range that spans topic..title (or topic..keyword when title is disabled).
	endCol := c.cfg.KeywordColumn
	wantTitle := c.cfg.TitleColumn != ""
	if wantTitle {
		endCol = c.cfg.TitleColumn
	}
	rangeStr := fmt.Sprintf("%s!%s:%s", c.cfg.Sheet, c.cfg.TopicColumn, endCol)

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.cfg.SpreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return Result{}, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	titleIdx := columnOffset(c.cfg.TopicColumn, c.cfg.TitleColumn)

	var out Result
	for i, row := range resp.Values {
		if c.cfg.HeaderRow && i == 0 {
			continue
		}
		if len(row) < 2 {
			continue
		}
		if normalize(fmt.Sprint(row[0])) != topic {
			continue
		}
		kw := strings.TrimSpace(fmt.Sprint(row[1]))
		if kw == "" {
			continue
		}
		out.Keywords = append(out.Keywords, kw)

		if wantTitle && out.Title == "" && titleIdx >= 0 && len(row) > titleIdx {
			if t := strings.TrimSpace(fmt.Sprint(row[titleIdx])); t != "" {
				out.Title = t
			}
		}
	}

	c.log.Debug("sheets lookup",
		"topic", topic,
		"matches", len(out.Keywords),
		"has_title", out.Title != "",
		"rows_scanned", len(resp.Values),
	)
	return out, nil
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// columnOffset returns the zero-based offset of col relative to base. Both
// must be single-letter column references in the same row; returns -1 when
// col is empty or parsing fails.
func columnOffset(base, col string) int {
	if base == "" || col == "" {
		return -1
	}
	b, c := strings.ToUpper(base), strings.ToUpper(col)
	if len(b) != 1 || len(c) != 1 {
		return -1
	}
	return int(c[0]) - int(b[0])
}
