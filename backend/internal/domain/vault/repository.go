package vault

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound is returned when id doesn't match any row.
var ErrNotFound = errors.New("vault: entry not found")

type CreateEntry struct {
	Title     string
	URL       string
	Username  string
	Password  string
	Notes     string
	CreatedBy string
}

// UpdateEntry is a partial edit — a nil field leaves the stored value
// unchanged, matching wordpress.UpdateSite's convention.
type UpdateEntry struct {
	Title    *string
	URL      *string
	Username *string
	Password *string
	Notes    *string
}

type Repository interface {
	Create(ctx context.Context, in CreateEntry) (Entry, error)
	List(ctx context.Context) ([]Entry, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateEntry) (Entry, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
