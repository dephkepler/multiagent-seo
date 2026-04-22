package sheets

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"contentflow/internal/config"
)

type Client interface {
	FetchKeywords(ctx context.Context) ([]string, error)
}

type client struct {
	svc *sheets.Service
	cfg config.SheetsConfig
}

func New(ctx context.Context, cfg config.SheetsConfig) (Client, error) {
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

	return &client{svc: svc, cfg: cfg}, nil
}

func (c *client) FetchKeywords(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rangeStr := fmt.Sprintf("%s!%s:%s", c.cfg.Sheet, c.cfg.KeywordColumn, c.cfg.KeywordColumn)

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.cfg.SpreadsheetID, rangeStr).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("fetch range %s: %w", rangeStr, err)
	}

	keywords := make([]string, 0, len(resp.Values))
	for _, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		kw := strings.TrimSpace(fmt.Sprintf("%v", row[0]))
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}

	return keywords, nil
}
