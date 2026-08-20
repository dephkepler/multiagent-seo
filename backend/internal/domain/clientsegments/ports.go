package clientsegments

import "context"

type Repository interface {
	ListActivity(ctx context.Context) ([]Activity, error)
	// nil segment clears the override, falling back to the calculated value.
	SetSegmentOverride(ctx context.Context, clientID string, segment *string) error
	// idempotent (existing tag is fine); returns ErrNotFound if clientID doesn't exist.
	AddTag(ctx context.Context, clientID, tag, createdBy string) error
	// idempotent; unlike AddTag, a missing client here is not an error either.
	RemoveTag(ctx context.Context, clientID, tag string) error
}
