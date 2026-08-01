package sheets

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/api/sheets/v4"

	"multiagent-seo/internal/domain/webleads"
)

type leadSink struct {
	svc           *sheets.Service
	spreadsheetID string
	sheet         string
	log           *slog.Logger
}

// NewLeadSink returns a webleads.SheetWriter that appends rows to a single
// sheet within a (likely shared, multi-tab) spreadsheet. It never touches
// any other tab.
func NewLeadSink(ctx context.Context, credentialsFile, spreadsheetID, sheet string, log *slog.Logger) (webleads.SheetWriter, error) {
	if log == nil {
		log = slog.Default()
	}
	if sheet == "" {
		return nil, fmt.Errorf("sheets: leads sheet name is required")
	}
	svc, err := newSheetsService(ctx, credentialsFile, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return &leadSink{svc: svc, spreadsheetID: spreadsheetID, sheet: sheet, log: log}, nil
}

// AppendRow adds one row: Дата | Имя | Телефон | Страница | Сообщение | ID заявки | ID клиента.
func (s *leadSink) AppendRow(ctx context.Context, lead webleads.Lead) error {
	row := []any{
		lead.ReceivedAt.Format("02.01.2006 15:04"),
		lead.Name,
		lead.Phone,
		lead.Page,
		lead.Message,
		lead.ShortID(),
		lead.ClientID,
	}

	rangeStr := s.sheet + "!A:G"
	// RAW, not USER_ENTERED: Sheets' "smart" parsing strips the leading "+"
	// off phone numbers (reads them as numbers) — RAW stores exactly what's given.
	_, err := s.svc.Spreadsheets.Values.Append(s.spreadsheetID, rangeStr, &sheets.ValueRange{
		Values: [][]any{row},
	}).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("append lead row to sheet %q: %w", s.sheet, err)
	}

	s.log.InfoContext(ctx, "sheets: lead row appended",
		"sheet", s.sheet,
		"message_id", lead.MessageID,
	)
	return nil
}
