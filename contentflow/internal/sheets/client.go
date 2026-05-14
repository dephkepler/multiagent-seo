// Package sheets reads a keyword cluster + H1 for an article topic from a
// Google Sheets table. Each row describes one article: the topic column is
// matched against the requested keyword, the keyword column holds a
// comma-separated list of target keywords, and the H1 column (optional)
// gives the article heading the LLM must use.
//
// Expected layout (column letters are configurable):
//
//	| A: Title (lookup key)              | B: Keywords (comma-separated)                                                 | C: H1 (optional)                   |
//	| Android Game Development Services  | Android Game Development, Android Game Development Services, ...              | Android Game Development Services  |
//	| game performance testing services  | game performance testing, game performance testing services, video game ...   | Game Performance Testing Services  |
//
// One row per article. If a topic appears in more than one row (legacy data),
// keywords from every matching row are merged and the first non-empty H1 wins.
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

// Result is the data pulled for a given topic: the full list of target
// keywords (split out of the comma-separated cell) plus an optional H1.
// The Title field is kept (rather than renamed to H1) so the prompt layer
// stays untouched — semantically it now holds the article's H1 heading.
type Result struct {
	Keywords []string
	Title    string // article H1 (from the H1 column)
}

// Client looks up a keyword cluster for an article topic.
type Client interface {
	// Lookup matches topic against the topic column (case-insensitive, trimmed),
	// splits the keyword column by commas into Result.Keywords, and puts the
	// H1 column value into Result.Title.
	// Empty Keywords with nil error means "no row for this topic".
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

	seen := make(map[string]struct{})
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

		for kw := range strings.SplitSeq(fmt.Sprint(row[1]), ",") {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			key := normalize(kw)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out.Keywords = append(out.Keywords, kw)
		}

		if wantTitle && out.Title == "" && titleIdx >= 0 && len(row) > titleIdx {
			if t := strings.TrimSpace(fmt.Sprint(row[titleIdx])); t != "" {
				out.Title = t
			}
		}
	}

	if len(out.Keywords) == 0 {
		c.log.Warn("sheets lookup: no match",
			"sheet", c.cfg.Sheet,
			"topic_normalized", topic,
			"rows_scanned", len(resp.Values),
			"range", rangeStr,
		)
	} else {
		c.log.Info("sheets lookup",
			"sheet", c.cfg.Sheet,
			"topic", topic,
			"keywords", len(out.Keywords),
			"has_h1", out.Title != "",
			"rows_scanned", len(resp.Values),
		)
	}
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
