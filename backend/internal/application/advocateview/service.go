// Package advocateview serves the advocate's own section. Every method takes
// the advocate id the middleware resolved from the login — there is no
// parameter a caller could point at somebody else, and the service refuses
// outright when the id is missing rather than falling back to "no filter".
package advocateview

import (
	"context"
	"fmt"
	"strings"

	domain "multiagent-seo/internal/domain/advocateview"
)

// maxNoteLen is what fits a call log entry; the same limit the admin card uses.
const maxNoteLen = 4000

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Cases(ctx context.Context, advocateID string) ([]domain.Case, error) {
	owner, err := s.owner(ctx, advocateID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListCases(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("advocateview: cases: %w", err)
	}
	return list, nil
}

func (s *Service) Clients(ctx context.Context, advocateID string) ([]domain.Client, error) {
	owner, err := s.owner(ctx, advocateID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListClients(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("advocateview: clients: %w", err)
	}
	return list, nil
}

func (s *Service) Client(ctx context.Context, advocateID, clientID string) (domain.Card, error) {
	owner, err := s.owner(ctx, advocateID)
	if err != nil {
		return domain.Card{}, err
	}
	card, err := s.repo.GetClient(ctx, owner, clientID)
	if err != nil {
		return domain.Card{}, fmt.Errorf("advocateview: client %q: %w", clientID, err)
	}
	return card, nil
}

func (s *Service) AddNote(ctx context.Context, advocateID, clientID, text, createdBy string) (domain.Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Note{}, fmt.Errorf("advocateview: add note: %w", domain.ErrEmptyNote)
	}
	if len(text) > maxNoteLen {
		return domain.Note{}, fmt.Errorf("advocateview: add note: %w", domain.ErrNoteTooLong)
	}
	owner, err := s.owner(ctx, advocateID)
	if err != nil {
		return domain.Note{}, err
	}
	note, err := s.repo.AddNote(ctx, owner, clientID, text, createdBy)
	if err != nil {
		return domain.Note{}, fmt.Errorf("advocateview: add note for client %q: %w", clientID, err)
	}
	return note, nil
}

func (s *Service) SetCaseStatus(ctx context.Context, advocateID, caseID, status string) error {
	if !domain.AllowedStatus(status) {
		return fmt.Errorf("advocateview: case %q status %q: %w", caseID, status, domain.ErrStatusNotAllowed)
	}
	owner, err := s.owner(ctx, advocateID)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateCaseStatus(ctx, owner, caseID, status); err != nil {
		return fmt.Errorf("advocateview: case %q status: %w", caseID, err)
	}
	return nil
}

func (s *Service) Settlement(ctx context.Context, advocateID string) (domain.Settlement, error) {
	advocate, err := s.repo.Advocate(ctx, advocateID)
	if err != nil {
		return domain.Settlement{}, fmt.Errorf("advocateview: settlement: %w", err)
	}
	months, err := s.repo.CollectionsByMonth(ctx, domain.Owner{ID: advocate.ID, FullName: advocate.FullName})
	if err != nil {
		return domain.Settlement{}, fmt.Errorf("advocateview: settlement collections: %w", err)
	}
	paid, err := s.repo.PaidOut(ctx, advocateID)
	if err != nil {
		return domain.Settlement{}, fmt.Errorf("advocateview: settlement payouts: %w", err)
	}
	return domain.NewSettlement(advocate, months, paid), nil
}

func (s *Service) Stats(ctx context.Context, advocateID string) (domain.Stats, error) {
	advocate, err := s.repo.Advocate(ctx, advocateID)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("advocateview: stats: %w", err)
	}
	owner := domain.Owner{ID: advocate.ID, FullName: advocate.FullName}

	caseList, err := s.repo.ListCases(ctx, owner)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("advocateview: stats cases: %w", err)
	}
	months, err := s.repo.CollectionsByMonth(ctx, owner)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("advocateview: stats collections: %w", err)
	}
	return domain.NewStats(caseList, domain.AccrueMonths(months, advocate.CommissionPercent)), nil
}

// owner resolves the login's advocate id into the identity the SQL scopes on.
// An unknown id is ErrNotFound, not an empty Owner: an empty Owner would match
// nothing today, but a future query that forgot the guard would match
// everything, and this is the wrong place to rely on luck.
func (s *Service) owner(ctx context.Context, advocateID string) (domain.Owner, error) {
	if strings.TrimSpace(advocateID) == "" {
		return domain.Owner{}, fmt.Errorf("advocateview: %w", domain.ErrNoAdvocate)
	}
	advocate, err := s.repo.Advocate(ctx, advocateID)
	if err != nil {
		return domain.Owner{}, fmt.Errorf("advocateview: advocate %q: %w", advocateID, err)
	}
	return domain.Owner{ID: advocate.ID, FullName: advocate.FullName}, nil
}
