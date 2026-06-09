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

	"multiagent-seo/internal/domain/articles"
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

func New(ctx context.Context, credentialsFile, spreadsheetID, sheet, topicCol, keywordCol, titleCol string, headerRow bool, log *slog.Logger) (articles.TopicSource, error) {
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

func (c *client) Lookup(ctx context.Context, topic string) (articles.Cluster, error) {
	topic = normalize(topic)
	if topic == "" {
		return articles.Cluster{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

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
		return articles.Cluster{}, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	titleIdx := columnOffset(c.topicCol, c.titleCol)

	seen := make(map[string]struct{})
	var out articles.Cluster
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
		c.log.WarnContext(ctx, "sheets lookup: no match",
			"sheet", c.sheet,
			"topic_normalized", topic,
			"rows_scanned", len(resp.Values),
			"range", rangeStr,
		)
	} else {
		c.log.DebugContext(ctx, "sheets lookup",
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
