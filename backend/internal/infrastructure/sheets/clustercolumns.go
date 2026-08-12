package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/articles"
)

// clusterColumnsTopN caps how many keywords from a cluster feed the writer
// prompt. Clusters here run past a hundred long-tail variants, sorted by
// volume; the top ones carry nearly all the SEO value, and the rest would
// just bloat the prompt.
const clusterColumnsTopN = 25

type clusterColumnsClient struct {
	svc           *sheets.Service
	spreadsheetID string
	sheet         string
	log           *slog.Logger
}

// NewClusterColumns reads a "keyword core" sheet laid out as groups of 3
// columns (cluster name, keyword, search volume, repeating left to right)
// instead of the usual one-row-per-topic layout. The highest-volume keyword
// in each cluster doubles as the topic's name, the role a dedicated topic
// column plays in the row-based format.
func NewClusterColumns(ctx context.Context, credentialsFile, spreadsheetID, sheet string, log *slog.Logger) (articles.TopicSource, error) {
	if credentialsFile == "" || spreadsheetID == "" || sheet == "" {
		return nil, fmt.Errorf("sheets: credentialsFile, spreadsheetId and sheet are required")
	}
	if log == nil {
		log = slog.Default()
	}

	credsJSON, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	creds, err := google.CredentialsFromJSON(ctx, credsJSON, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	svc, err := sheets.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &clusterColumnsClient{svc: svc, spreadsheetID: spreadsheetID, sheet: sheet, log: log}, nil
}

type keywordVolume struct {
	keyword string
	volume  float64
}

func (c *clusterColumnsClient) Clusters(ctx context.Context) (map[string]articles.Cluster, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rangeStr := fmt.Sprintf("'%s'!A1:ZZ2000", c.sheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rangeStr).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetch cluster columns %s: %w", rangeStr, err)
	}
	if len(resp.Values) == 0 {
		return map[string]articles.Cluster{}, nil
	}

	var groupCols []int
	for i, cell := range resp.Values[0] {
		if i%3 == 0 && strings.TrimSpace(fmt.Sprint(cell)) != "" {
			groupCols = append(groupCols, i)
		}
	}

	rowsByGroup := make(map[int][]keywordVolume, len(groupCols))
	for _, row := range resp.Values[1:] {
		for _, col := range groupCols {
			if col >= len(row) {
				continue
			}
			kw := strings.TrimSpace(fmt.Sprint(row[col]))
			if kw == "" {
				continue
			}
			var vol float64
			if col+1 < len(row) {
				vol, _ = strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(row[col+1])), 64)
			}
			rowsByGroup[col] = append(rowsByGroup[col], keywordVolume{keyword: kw, volume: vol})
		}
	}

	out := make(map[string]articles.Cluster, len(groupCols))
	for _, col := range groupCols {
		rows := rowsByGroup[col]
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].volume > rows[j].volume })
		if len(rows) > clusterColumnsTopN {
			rows = rows[:clusterColumnsTopN]
		}

		keywords := make([]string, len(rows))
		for i, r := range rows {
			keywords[i] = r.keyword
		}
		out[normalize(keywords[0])] = articles.Cluster{Keywords: keywords}
	}

	c.log.DebugContext(ctx, "sheets cluster columns",
		"sheet", c.sheet,
		"clusters", len(out),
	)
	return out, nil
}

func (c *clusterColumnsClient) Topics(ctx context.Context) ([]string, error) {
	clusters, err := c.Clusters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(clusters))
	for topic := range clusters {
		out = append(out, topic)
	}
	return out, nil
}

func (c *clusterColumnsClient) Lookup(ctx context.Context, topic string) (articles.Cluster, error) {
	clusters, err := c.Clusters(ctx)
	if err != nil {
		return articles.Cluster{}, err
	}
	return clusters[normalize(topic)], nil
}
