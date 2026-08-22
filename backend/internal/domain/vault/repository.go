package vault

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound is returned when id doesn't match any row.
var ErrNotFound = errors.New("vault: entry not found")

// ErrGroupNotFound is returned when a group id doesn't match any row.
var ErrGroupNotFound = errors.New("vault: group not found")

// ErrGroupHasEntries is returned when deleting a group that still holds
// entries — delete the entries first.
var ErrGroupHasEntries = errors.New("vault: group has entries, refusing to delete")

type CreateEntry struct {
	GroupID   uuid.UUID
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
	List(ctx context.Context, groupID uuid.UUID) ([]Entry, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateEntry) (Entry, error)
	Delete(ctx context.Context, id uuid.UUID) error

	CreateGroup(ctx context.Context, name string) (Group, error)
	ListGroups(ctx context.Context) ([]GroupWithCount, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error
}
