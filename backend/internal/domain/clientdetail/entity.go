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

// ErrHasHistory is returned by Delete when the client has any lead,
// consultation, or case — real business history a stray click shouldn't
// be able to destroy. A junk/duplicate/test client with none of those can
// still be deleted.
var ErrHasHistory = errors.New("clientdetail: client has history, refusing to delete")

// ErrInvalidGender/ErrInvalidClientType guard UpdateClient — checked before
// the DB's own CHECK constraint so a bad value fails with a clear domain
// error instead of a raw Postgres constraint message reaching the client.
var ErrInvalidGender = errors.New("clientdetail: invalid gender")
var ErrInvalidClientType = errors.New("clientdetail: invalid client type")

type Client struct {
	ID   string
	Name string // full display name — see consultations.ComposeName
	// LastName/FirstName/Patronymic are the editable parts behind Name —
	// separate fields on the client card, not free text.
	LastName   string
	FirstName  string
	Patronymic string
	Gender     string // "", consultations.GenderMale, or consultations.GenderFemale
	Phone      string
	Email      string
	ClientType string // consultations.ClientTypeIndividual or ClientTypeLegalEntity
	// CompanyName/CompanyCode are only meaningful when ClientType is
	// legal_entity — left empty otherwise, not hidden/omitted.
	CompanyName string
	CompanyCode string
	// Address/Birthdate/TaxID are sensitive — already decrypted by the
	// repository by the time they reach here (see clientdetail_repository.go).
	// Birthdate is "YYYY-MM-DD" or "" when not recorded.
	Address     string
	Birthdate   string
	TaxID       string
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
	// RevenueTotal is this client's lifetime value — money actually
	// collected, not booked/contracted: sum of Case.Paid (not the fee) plus
	// completed, priced consultations. Same "earned, not booked" rule as
	// the leads dashboard (see leadstats): a signed case isn't cash until
	// it's paid, and a cancelled consultation never was.
	RevenueTotal  float64
	Leads         []Lead
	Consultations []Consultation
	Cases         []Case
	Notes         []Note
}
