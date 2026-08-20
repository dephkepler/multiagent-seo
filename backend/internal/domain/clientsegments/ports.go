package clientsegments

import "context"

type Repository interface {
	ListActivity(ctx context.Context) ([]Activity, error)
	// nil segment clears the override, falling back to the calculated value.
	SetSegmentOverride(ctx context.Context, clientID string, segment *string) error
	// idempotent (existing tag is fine); returns ErrNotFound if clientID
	// doesn't exist, ErrUnknownTag if tag isn't in client_tag_defs.
	AddTag(ctx context.Context, clientID, tag, createdBy string) error
	// idempotent; unlike AddTag, a missing client here is not an error either.
	RemoveTag(ctx context.Context, clientID, tag string) error

	// ListTagDefs returns the whole manual-tag vocabulary, alphabetical.
	ListTagDefs(ctx context.Context) ([]TagDef, error)
	// CreateTagDef adds a new vocabulary entry. Returns ErrTagDefExists if
	// label is already defined.
	CreateTagDef(ctx context.Context, label, createdBy string) error
	// RenameTagDef changes a vocabulary entry's label — every client_tags
	// row using oldLabel is updated to newLabel in the same statement (DB
	// FK, ON UPDATE CASCADE), not swept separately. Returns
	// ErrTagDefNotFound if oldLabel doesn't exist, ErrTagDefExists if
	// newLabel is already taken by a different entry.
	RenameTagDef(ctx context.Context, oldLabel, newLabel string) error
	// DeleteTagDef removes a vocabulary entry and, via the same FK (ON
	// DELETE CASCADE), every client_tags row that used it — idempotent,
	// deleting a label that's already gone is not an error.
	DeleteTagDef(ctx context.Context, label string) error
}
