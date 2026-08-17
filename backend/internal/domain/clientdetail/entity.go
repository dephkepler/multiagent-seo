// Package clientdetail assembles everything staff need to see about one
// client on a single screen — the admin UI's client card: their leads
// (original inquiries), consultations, cases (дела), manual call notes,
// and money actually collected. It reads across leads/consultations/cases/
// client_notes for one client_id; nothing here computes or stores a
// funnel classification — see clientsegments for that, not duplicated here.
package clientdetail

import (
	"errors"
	"time"
)

// ErrNotFound is returned when clientID doesn't match any row in clients —
// distinct from a client that exists but has an empty history everywhere,
// which is a normal, non-error Detail.
var ErrNotFound = errors.New("clientdetail: client not found")

// ErrEmptyNote is returned by AddNote for blank text — a note nobody wrote
// anything into isn't worth a row.
var ErrEmptyNote = errors.New("clientdetail: note text is empty")

type Client struct {
	ID          string
	Name        string
	Phone       string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
}

// Lead is one original inquiry (email or self-request) tied to this client.
type Lead struct {
	ID         string
	ReceivedAt time.Time
	Message    string
	Page       string
}

type Consultation struct {
	ID          string
	ScheduledAt time.Time
	Price       float64
	Status      string
	CaseNote    string
}

// Case is one дело (клопотання/позов/супровід) — see domain/cases.
type Case struct {
	ID          string
	Description string
	Category    string
	Status      string
	Fee         float64
	Paid        float64
	CreatedAt   time.Time
}

// Note is a staff-written call/contact log entry — see the package doc.
type Note struct {
	ID        string
	Text      string
	CreatedBy string
	CreatedAt time.Time
}

// Detail is one client's full card.
type Detail struct {
	Client Client
	// RevenueTotal is money actually collected — sum of Case.Paid, not
	// contracted fees. Same "earned, not booked" rule as the leads
	// dashboard (see leadstats): a signed case isn't cash until it's paid.
	RevenueTotal  float64
	Leads         []Lead
	Consultations []Consultation
	Cases         []Case
	Notes         []Note
}
