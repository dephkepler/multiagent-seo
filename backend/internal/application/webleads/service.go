package webleads

import (
	"context"
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

// ProcessNewLeads fetches unread mail, delivers each as a Telegram message,
// saves it, appends it to the shared sheet, and marks it seen only once
// delivery succeeded. A message that fails to send is left unseen on
// purpose, so the next poll retries it instead of losing the lead. A DB or
// sheet failure is logged but doesn't block marking the message seen —
// Telegram already has it, and re-polling the same message would just
// resend the Telegram notification. The sheet write is only attempted once
// the DB save succeeds, since sheet_synced_at bookkeeping needs that row to
// exist.
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

		if err := s.notifier.SendMessage(ctx, domain.FormatTelegram(lead)); err != nil {
			s.log.ErrorContext(ctx, "webleads: telegram send failed, will retry next poll",
				"message_id", lead.MessageID,
				"err", err,
			)
			continue
		}

		if err := s.store.Save(ctx, lead); err != nil {
			s.log.ErrorContext(ctx, "webleads: save to db failed",
				"message_id", lead.MessageID,
				"err", err,
			)
		} else if err := s.sheet.AppendRow(ctx, lead); err != nil {
			s.log.ErrorContext(ctx, "webleads: append to sheet failed, will retry later",
				"message_id", lead.MessageID,
				"err", err,
			)
		} else if err := s.store.MarkSheetSynced(ctx, lead.MessageID); err != nil {
			s.log.ErrorContext(ctx, "webleads: mark sheet synced failed",
				"message_id", lead.MessageID,
				"err", err,
			)
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
