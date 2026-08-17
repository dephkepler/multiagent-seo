package consultations

import (
	"strings"
	"time"
)

// Status values a consultation can hold. Every consultation starts
// Scheduled (the DB column defaults to it) — staff move it to one of the
// other three from the inline buttons the bot sends after booking.
const (
	StatusScheduled = "scheduled"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusNoShow    = "no_show"
)

type Consultation struct {
	ID          string
	ClientID    string
	ScheduledAt time.Time
	Price       float64
	CaseNote    string
	CreatedBy   string
	Status      string
}

type Client struct {
	ID             string
	Name           string
	Phone          string
	TelegramName   string
	TelegramChatID int64
}

// Gender is optional and empty ("") means not recorded — never guessed
// from a name, only ever set by staff on the client card.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// IsGender reports whether s is a recorded Gender* value or the "not set"
// empty string.
func IsGender(s string) bool {
	return s == "" || s == GenderMale || s == GenderFemale
}

// ClientType distinguishes a private individual from a company — affects
// what documents/fields a case needs (see ClientEdit.CompanyName/Code).
const (
	ClientTypeIndividual  = "individual"
	ClientTypeLegalEntity = "legal_entity"
)

// IsClientType reports whether s is a known ClientType* value.
func IsClientType(s string) bool {
	return s == ClientTypeIndividual || s == ClientTypeLegalEntity
}

// ClientEdit is what staff can change about a client's own contact details
// from the client card — see clientdetail.Service.UpdateClient.
//
// Address/Birthdate/TaxID pass through as plain text here; the repository
// is what encrypts them at rest (see ConsultationRepository.UpdateClient) —
// this type is transport, not storage, so it doesn't need to know that.
type ClientEdit struct {
	LastName    string
	FirstName   string
	Patronymic  string
	Phone       string
	Gender      string // "", GenderMale, or GenderFemale
	Email       string
	ClientType  string // ClientTypeIndividual or ClientTypeLegalEntity
	CompanyName string // only meaningful when ClientType is legal_entity
	CompanyCode string // ЄДРПОУ — only meaningful when ClientType is legal_entity
	Address     string // sensitive — encrypted at rest
	Birthdate   string // sensitive — encrypted at rest; "YYYY-MM-DD" or ""
	TaxID       string // РНОКПП/ІПН — sensitive, encrypted at rest
}

// ComposeName joins name parts into one display string in Прізвище Ім'я
// По батькові order (Ukrainian legal/business convention) — every other
// place in the app (bot messages, search, cases) keeps reading the single
// Client.Name it produces, so this is the one place "full name" logic
// lives, not duplicated at each call site. Empty parts are skipped, not
// left as gaps.
func ComposeName(lastName, firstName, patronymic string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{lastName, firstName, patronymic} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

type ReminderTarget struct {
	Consultation Consultation
	Client       Client
}

type Advocate struct {
	ID               string
	FullName         string
	TelegramUsername string
	TelegramChatID   int64
	// IsActive is false once an advocate has left — they drop out of
	// pickers for new work but stay on old cases/consultations untouched.
	IsActive bool
}
