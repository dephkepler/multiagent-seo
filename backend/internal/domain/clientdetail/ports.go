package clientdetail

import "context"

// Repository reads the aggregate card and owns the one write that's
// genuinely this package's own concern (notes) — everything else staff can
// change about a client (name/phone, segment override) is written through
// the packages that already own that data (consultations, clientsegments).
type Repository interface {
	// Get returns ErrNotFound if clientID doesn't match any client.
	Get(ctx context.Context, clientID string) (Detail, error)
	AddNote(ctx context.Context, clientID, text, createdBy string) (Note, error)
}
