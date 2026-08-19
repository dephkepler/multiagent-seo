package clientdetail

import (
	"context"
	"fmt"
	"strings"

	domain "multiagent-seo/internal/domain/clientdetail"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/domain/webleads"
)

// clientWriter is the one method this service needs from
// consultations.Store — a narrow local port instead of depending on the
// whole Store interface for one call.
type clientWriter interface {
	UpdateClient(ctx context.Context, clientID string, edit consultations.ClientEdit) error
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

// UpdateClient edits everything staff can change on the client card. Phone
// gets the same normalization a lead coming in through the site already
// goes through, so a hand-typed number matches the format everything else
// (search, dedup) expects instead of drifting into its own format.
// Address/Birthdate/TaxID are passed through as plain text — the
// repository is what encrypts them at rest.
func (s *Service) UpdateClient(ctx context.Context, clientID string, edit consultations.ClientEdit) error {
	edit.LastName = strings.TrimSpace(edit.LastName)
	edit.FirstName = strings.TrimSpace(edit.FirstName)
	edit.Patronymic = strings.TrimSpace(edit.Patronymic)
	edit.Phone = webleads.NormalizePhone(edit.Phone)
	edit.Email = strings.TrimSpace(edit.Email)
	edit.CompanyName = strings.TrimSpace(edit.CompanyName)
	edit.CompanyCode = strings.TrimSpace(edit.CompanyCode)
	edit.Address = strings.TrimSpace(edit.Address)
	edit.Birthdate = strings.TrimSpace(edit.Birthdate)
	edit.TaxID = strings.TrimSpace(edit.TaxID)

	if !consultations.IsGender(edit.Gender) {
		return fmt.Errorf("clientdetail: update client: %w: %q", domain.ErrInvalidGender, edit.Gender)
	}
	if edit.ClientType == "" {
		edit.ClientType = consultations.ClientTypeIndividual
	}
	if !consultations.IsClientType(edit.ClientType) {
		return fmt.Errorf("clientdetail: update client: %w: %q", domain.ErrInvalidClientType, edit.ClientType)
	}

	if err := s.clients.UpdateClient(ctx, clientID, edit); err != nil {
		return fmt.Errorf("clientdetail: update client %q: %w", clientID, err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, clientID string) error {
	if err := s.repo.Delete(ctx, clientID); err != nil {
		return fmt.Errorf("clientdetail: delete %q: %w", clientID, err)
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
