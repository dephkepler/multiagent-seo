package webleads

import (
	"context"
	"fmt"
	"log/slog"

	domain "multiagent-seo/internal/domain/webleads"
)

type Service struct {
	mail     domain.MailSource
	notifier domain.Notifier
	store    domain.Store
	sheet    domain.SheetWriter
	log      *slog.Logger
}

func NewService(mail domain.MailSource, notifier domain.Notifier, store domain.Store, sheet domain.SheetWriter, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{mail: mail, notifier: notifier, store: store, sheet: sheet, log: log}
}

func (s *Service) ProcessNewLeads(ctx context.Context) {
	messages, err := s.mail.FetchUnseen(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, "webleads: fetch mail failed", "err", err)
		return
	}
	if len(messages) == 0 {
		return
	}
	s.log.InfoContext(ctx, "webleads: new messages", "count", len(messages))

	for _, m := range messages {
		lead := domain.Parse(m.MessageID, m.From, m.Subject, m.Body, m.Date)

		if _, err := s.SubmitLead(ctx, lead); err != nil {
			s.log.ErrorContext(ctx, "webleads: telegram send failed, will retry next poll",
				"message_id", lead.MessageID,
				"err", err,
			)
			continue
		}

		if err := s.mail.MarkSeen(ctx, m.UID); err != nil {
			s.log.ErrorContext(ctx, "webleads: mark seen failed, may resend next poll",
				"uid", m.UID,
				"err", err,
			)
			continue
		}

		s.log.InfoContext(ctx, "webleads: processed",
			"name", lead.Name,
			"message_id", lead.MessageID,
		)
	}
}

// SubmitLead resolves the client, notifies Telegram, and persists the lead —
// shared by the mail poller and any other lead source (e.g. the Telegram
// self-booking flow). Returns the resolved client id (empty if resolution
// failed or the lead has no phone) so a caller that needs to attach more
// than Lead carries — e.g. an email collected in the same conversation —
// can follow up against that same client row instead of resolving it again.
// A notify failure is returned so a caller with retry semantics (mail
// polling) can act on it; DB/sheet failures are logged only, since Telegram
// already has the lead and retrying would duplicate it.
func (s *Service) SubmitLead(ctx context.Context, lead domain.Lead) (string, error) {
	if lead.Phone != "" {
		if clientID, err := s.store.ResolveClient(ctx, lead.Phone, lead.Name); err != nil {
			s.log.WarnContext(ctx, "webleads: resolve client failed, sending without a client id",
				"message_id", lead.MessageID,
				"err", err,
			)
		} else {
			lead.ClientID = clientID
		}
	}

	messageID, err := s.notifier.SendMessage(ctx, domain.FormatTelegram(lead), domain.PracticeAreaButtons())
	if err != nil {
		return "", fmt.Errorf("webleads: telegram send: %w", err)
	}
	lead.TelegramMessageID = messageID

	if err := s.store.Save(ctx, lead); err != nil {
		s.log.ErrorContext(ctx, "webleads: save to db failed",
			"message_id", lead.MessageID,
			"err", err,
		)
		return lead.ClientID, nil
	}
	if err := s.sheet.AppendRow(ctx, lead); err != nil {
		s.log.ErrorContext(ctx, "webleads: append to sheet failed, will retry later",
			"message_id", lead.MessageID,
			"err", err,
		)
		return lead.ClientID, nil
	}
	if err := s.store.MarkSheetSynced(ctx, lead.MessageID); err != nil {
		s.log.ErrorContext(ctx, "webleads: mark sheet synced failed",
			"message_id", lead.MessageID,
			"err", err,
		)
	}
	return lead.ClientID, nil
}

// SetLeadPracticeArea records what a lead is about — called from
// AdminBot's "leadpa:" callback handler when staff taps a button on the
// lead's own Telegram notification (see domain.PracticeAreaButtons).
func (s *Service) SetLeadPracticeArea(ctx context.Context, telegramMessageID int, area string) error {
	if err := s.store.SetPracticeArea(ctx, telegramMessageID, area); err != nil {
		return fmt.Errorf("webleads: set practice area: %w", err)
	}
	return nil
}
