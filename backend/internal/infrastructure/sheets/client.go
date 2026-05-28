// Package sheets adapts a Google Sheets keyword table to the generate.TopicSource
// port. One row per article; on duplicate topic rows the keywords merge and the
// first non-empty H1 wins. It is a copy of the legacy internal/sheets client,
// decoupled from config: the constructor takes primitives so infrastructure does
// not depend on internal/config.
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

	"multiagent-seo/internal/domain/generate"
)

type client struct {
	svc           *sheets.Service
	spreadsheetID string
	sheet         string
	topicCol      string
	keywordCol    string
	titleCol      string
	headerRow     bool
	log           *slog.Logger
}

var _ generate.TopicSource = (*client)(nil)

// New returns an error when credentials or spreadsheet ID are missing so
// callers can fall back to the mock.
func New(ctx context.Context, credentialsFile, spreadsheetID, sheet, topicCol, keywordCol, titleCol string, headerRow bool, log *slog.Logger) (generate.TopicSource, error) {
	if credentialsFile == "" || spreadsheetID == "" {
		return nil, fmt.Errorf("sheets: credentialsFile and spreadsheetId are required")
	}

	data, err := os.ReadFile(credentialsFile)
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

	return &client{
		svc:           svc,
		spreadsheetID: spreadsheetID,
		sheet:         sheet,
		topicCol:      topicCol,
		keywordCol:    keywordCol,
		titleCol:      titleCol,
		headerRow:     headerRow,
		log:           log,
	}, nil
}

func (c *client) Lookup(ctx context.Context, topic string) (generate.Cluster, error) {
	topic = normalize(topic)
	if topic == "" {
		return generate.Cluster{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Range spans topic..title, or topic..keyword when title is disabled.
	endCol := c.keywordCol
	wantTitle := c.titleCol != ""
	if wantTitle {
		endCol = c.titleCol
	}
	rangeStr := fmt.Sprintf("%s!%s:%s", c.sheet, c.topicCol, endCol)

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return generate.Cluster{}, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	titleIdx := columnOffset(c.topicCol, c.titleCol)

	seen := make(map[string]struct{})
	var out generate.Cluster
	for i, row := range resp.Values {
		if c.headerRow && i == 0 {
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
			"sheet", c.sheet,
			"topic_normalized", topic,
			"rows_scanned", len(resp.Values),
			"range", rangeStr,
		)
	} else {
		c.log.Info("sheets lookup",
			"sheet", c.sheet,
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
