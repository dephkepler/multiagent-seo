package cases

import "context"

type Store interface {
	Save(ctx context.Context, c Case) (Case, error)
	Get(ctx context.Context, caseID string) (Case, error)
	// ListByClient returns every case for a client, newest first — a
	// client can have more than one (e.g. a закриту справу and a нову),
	// so this is not the same as "the latest one".
	ListByClient(ctx context.Context, clientID string) ([]Case, error)
	// AddPayment adds to the running paid total — payments come in
	// installments (see package doc), there is no "mark as paid" toggle.
	AddPayment(ctx context.Context, caseID string, amount float64) (Case, error)
	UpdateStatus(ctx context.Context, caseID, status string) error
}
