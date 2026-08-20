package cases

import (
	"context"
	"time"
)

type Store interface {
	Save(ctx context.Context, c Case) (Case, error)
	Get(ctx context.Context, caseID string) (Case, error)
	// ListByClient returns every case for a client, newest first — a
	// client can have more than one (e.g. a закриту справу and a нову),
	// so this is not the same as "the latest one". Each Case's Payments is
	// populated (see Payment).
	ListByClient(ctx context.Context, clientID string) ([]Case, error)
	// AddPayment records one installment (paidAt, createdBy) in the
	// case_payments ledger and adds it to the running paid total — payments
	// come in installments (see package doc), there is no "mark as paid"
	// toggle.
	AddPayment(ctx context.Context, caseID string, amount float64, paidAt time.Time, createdBy string) (Case, error)
	UpdateStatus(ctx context.Context, caseID, status string) error
}
