package sheets

import (
	"context"
	"fmt"
	"log/slog"

	sheetsapi "google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/consultations"
)

type consultationSink struct {
	svc           *sheetsapi.Service
	spreadsheetID string
	sheet         string
	log           *slog.Logger
}

func NewConsultationSink(ctx context.Context, credentialsFile, spreadsheetID, sheet string, log *slog.Logger) (consultations.SheetWriter, error) {
	if log == nil {
		log = slog.Default()
	}
	if sheet == "" {
		return nil, fmt.Errorf("sheets: consultations sheet name is required")
	}
	svc, err := newSheetsService(ctx, credentialsFile, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return &consultationSink{svc: svc, spreadsheetID: spreadsheetID, sheet: sheet, log: log}, nil
}

func (s *consultationSink) AppendRow(ctx context.Context, c consultations.Consultation, client consultations.Client) error {
	row := []any{
		c.ScheduledAt.Format("02.01.2006"),
		c.ScheduledAt.Format("15:04"),
		client.Name,
		client.Phone,
		c.Price,
		c.CaseNote,
		c.ID,
		client.ID,
	}

	rangeStr := s.sheet + "!A:H"
	_, err := s.svc.Spreadsheets.Values.Append(s.spreadsheetID, rangeStr, &sheetsapi.ValueRange{
		Values: [][]any{row},
	}).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("append consultation row to sheet %q: %w", s.sheet, err)
	}

	s.log.InfoContext(ctx, "sheets: consultation row appended", "sheet", s.sheet, "consultation_id", c.ID)
	return nil
}
