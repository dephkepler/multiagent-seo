package sheets

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func newSheetsService(ctx context.Context, credentialsFile, spreadsheetID string) (*sheets.Service, error) {
	if credentialsFile == "" || spreadsheetID == "" {
		return nil, fmt.Errorf("sheets: credentialsFile and spreadsheetId are required")
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
	return svc, nil
}
