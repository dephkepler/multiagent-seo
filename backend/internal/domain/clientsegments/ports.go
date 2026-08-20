package clientsegments

import "context"

type Repository interface {
	ListActivity(ctx context.Context) ([]Activity, error)
	// nil segment clears the override, falling back to the calculated value.
	SetSegmentOverride(ctx context.Context, clientID string, segment *string) error
	// idempotent; ErrNotFound if clientID doesn't exist, ErrUnknownTag if tag isn't in client_tag_defs.
	AddTag(ctx context.Context, clientID, tag, createdBy string) error
	// idempotent; unlike AddTag, a missing client here is not an error either.
	RemoveTag(ctx context.Context, clientID, tag string) error
	// ClientTags is a scoped read for callers (the bot's /tags menu) that
	// only need one client's manual tags — a plain indexed lookup, not
	// List's full ListActivity/Derive pass over every client.
	ClientTags(ctx context.Context, clientID string) ([]string, error)

	// category, then label — matches the two-level grouping the picker/
	// management UI show.
	ListTagDefs(ctx context.Context) ([]TagDef, error)
	// ErrTagDefExists if label is already defined.
	CreateTagDef(ctx context.Context, label, category, createdBy string) error
	// UpdateTagDef changes label and/or category — nil leaves that field
	// as-is. Renaming the label cascades to client_tags via DB FK (ON
	// UPDATE CASCADE), so every client carrying the old label follows in
	// the same statement. ErrTagDefNotFound if label doesn't exist,
	// ErrTagDefExists if newLabel collides with a different entry.
	UpdateTagDef(ctx context.Context, label string, newLabel, newCategory *string) error
	// cascades to client_tags via DB FK (ON DELETE CASCADE); deleting an already-gone label is not an error.
	DeleteTagDef(ctx context.Context, label string) error
}
