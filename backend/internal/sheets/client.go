// Package sheets reads a keyword cluster and H1 for an article topic from
// a Google Sheets table. One row per article; on duplicate topic rows the
// keywords merge and the first non-empty H1 wins.
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

	"multiagent-seo/internal/config"
)

// Result holds the keywords (split out of the comma-separated cell) and the
// article H1. Title (not H1) is kept as the field name to keep the prompt
// layer untouched.
type Result struct {
	Keywords []string
	Title    string
}

type Client interface {
	// Lookup returns the row matching topic (case-insensitive, trimmed).
	// Empty Keywords with nil error means "no row for this topic".
	Lookup(ctx context.Context, topic string) (Result, error)
}

type client struct {
	svc *sheets.Service
	cfg config.SheetsConfig
	log *slog.Logger
}

// New returns an error when credentials or spreadsheet ID are missing so
// callers can fall back to the mock.
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

func (c *client) Lookup(ctx context.Context, topic string) (Result, error) {
	topic = normalize(topic)
	if topic == "" {
		return Result{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Range spans topic..title, or topic..keyword when title is disabled.
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

// columnOffset returns the zero-based offset of col relative to base.
// Returns -1 when either is empty or not a single-letter reference.
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
