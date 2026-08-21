package advocateview

import (
	"context"
	"errors"

	"multiagent-seo/internal/domain/cases"
)

// ErrNotFound covers both "no such row" and "not yours". An advocate asking
// for a client who exists but belongs to a colleague gets the same answer as
// for a client who does not exist — a 403 would confirm that the id is real,
// which is itself information about the firm's clients.
var ErrNotFound = errors.New("advocateview: not found")

// ErrStatusNotAllowed is returned for a status change that is not an
// advocate's call to make (cancelling a case writes off what the client owes).
var ErrStatusNotAllowed = errors.New("advocateview: status not allowed")

// Repository is the whole advocate-facing data surface. Every read takes an
// Owner, and there is no method that can return another advocate's rows —
// which is the point: the scoping is in the SQL, not in a caller that might
// forget to pass a filter.
type Repository interface {
	Advocate(ctx context.Context, advocateID string) (Advocate, error)
	ListCases(ctx context.Context, owner Owner) ([]Case, error)
	ListClients(ctx context.Context, owner Owner) ([]Client, error)
	// GetClient returns ErrNotFound unless the advocate has a case with this
	// client.
	GetClient(ctx context.Context, owner Owner, clientID string) (Card, error)
	// AddNote refuses (ErrNotFound) a client the advocate has no case with.
	AddNote(ctx context.Context, owner Owner, clientID, text, createdBy string) (Note, error)
	// UpdateCaseStatus carries the ownership check in its WHERE clause, so
	// there is no window between "is it mine" and "change it".
	UpdateCaseStatus(ctx context.Context, owner Owner, caseID, status string) error
	// CollectionsByMonth is what the advocate's cases actually collected, per
	// month of the payment ledger.
	CollectionsByMonth(ctx context.Context, owner Owner) ([]MonthMoney, error)
	// PaidOut is the total of payouts booked to this advocate — see
	// Settlement.PaidIsPartial for what it cannot see.
	PaidOut(ctx context.Context, advocateID string) (float64, error)
}

// AllowedStatus is what an advocate may set on their own case. Cancelling is
// missing on purpose: it writes off what the client owes and drops the case out
// of the receivable, so it stays an admin decision.
func AllowedStatus(status string) bool {
	return status == cases.StatusInProgress || status == cases.StatusCompleted
}

var (
	// ErrNoAdvocate means the login is not tied to a roster row — an admin
	// hitting an advocate-only endpoint, or an advocate account created
	// without the link.
	ErrNoAdvocate  = errors.New("advocateview: login has no advocate")
	ErrEmptyNote   = errors.New("advocateview: note is empty")
	ErrNoteTooLong = errors.New("advocateview: note is too long")
)
