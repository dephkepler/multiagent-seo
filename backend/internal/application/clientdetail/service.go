package clientdetail

import (
	"context"
	"fmt"
	"strings"

	domain "multiagent-seo/internal/domain/clientdetail"
	"multiagent-seo/internal/domain/webleads"
)

// clientWriter is the one method this service needs from
// consultations.Store — a narrow local port instead of depending on the
// whole Store interface for one call.
type clientWriter interface {
	UpdateClient(ctx context.Context, clientID, name, phone string) error
}

type Service struct {
	repo    domain.Repository
	clients clientWriter
}

func NewService(repo domain.Repository, clients clientWriter) *Service {
	return &Service{repo: repo, clients: clients}
}

func (s *Service) Get(ctx context.Context, clientID string) (domain.Detail, error) {
	d, err := s.repo.Get(ctx, clientID)
	if err != nil {
		return domain.Detail{}, fmt.Errorf("clientdetail: get %q: %w", clientID, err)
	}
	return d, nil
}

// UpdateClient edits name/phone as typed into the client card. Phone gets
// the same normalization a lead coming in through the site already goes
// through, so a hand-typed number matches the format everything else
// (search, dedup) expects instead of drifting into its own format.
func (s *Service) UpdateClient(ctx context.Context, clientID, name, phone string) error {
	name = strings.TrimSpace(name)
	phone = webleads.NormalizePhone(phone)
	if err := s.clients.UpdateClient(ctx, clientID, name, phone); err != nil {
		return fmt.Errorf("clientdetail: update client %q: %w", clientID, err)
	}
	return nil
}

func (s *Service) AddNote(ctx context.Context, clientID, text, createdBy string) (domain.Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Note{}, domain.ErrEmptyNote
	}
	n, err := s.repo.AddNote(ctx, clientID, text, createdBy)
	if err != nil {
		return domain.Note{}, fmt.Errorf("clientdetail: add note for %q: %w", clientID, err)
	}
	return n, nil
}
