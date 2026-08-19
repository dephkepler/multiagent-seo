package clientdetail

import "context"

// Repository reads the aggregate card and owns the writes that are
// genuinely this package's own concern (notes, and deleting the client
// record itself) — editing a client's own fields (name/phone, segment
// override) is written through the packages that already own that data
// (consultations, clientsegments).
type Repository interface {
	// Get returns ErrNotFound if clientID doesn't match any client.
	Get(ctx context.Context, clientID string) (Detail, error)
	AddNote(ctx context.Context, clientID, text, createdBy string) (Note, error)
	// Delete returns ErrHasHistory if the client has any lead, consultation,
	// or case, ErrNotFound if clientID doesn't match any client.
	Delete(ctx context.Context, clientID string) error
}
