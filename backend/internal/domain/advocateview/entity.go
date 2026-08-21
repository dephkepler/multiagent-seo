// Package advocateview is what an advocate is allowed to see of the CRM under
// their own login: their cases, the clients behind those cases, and their own
// money. It is deliberately a separate, narrower model than the admin's
// (clientdetail.Detail, finance.Settlement) rather than a filter over it —
// the admin card carries decrypted address/РНОКПП, colleagues' fees and
// firm-wide revenue, and retrofitting a filter into five queries is how one
// of those fields eventually leaks.
//
// Scope is defined by exactly one link: cases.advocate_id. consultations,
// leads and client_notes have no advocate column, so "my client" can only mean
// "a client I have a case with" — a client who only ever had a consultation is
// invisible here. That is a limit of the schema, not a decision, and it is
// written down rather than papered over.
package advocateview

import "time"

// Owner is the advocate a request is scoped to. The name is part of the
// identity because cases opened before the advocate roster carry only a
// free-text surname (cases.AdvocateName) — the same reason
// FinanceRepository.AdvocateCollections joins on the link OR an exact name
// match. Both places must agree, or an advocate would see money from a case
// the same page refuses to list.
type Owner struct {
	ID       string
	FullName string
}

// Advocate is the roster row behind the login — the source of the commission
// percentage the settlement is computed from.
type Advocate struct {
	ID                string
	FullName          string
	IsActive          bool
	CommissionPercent float64
}

type Case struct {
	ID          string
	ClientID    string
	ClientName  string
	ClientPhone string
	Category    string
	Status      string
	Description string
	Fee         float64
	Paid        float64
	CreatedAt   time.Time
	Payments    []Payment
}

// Owed is what the client still owes on this case — the advocate's own
// follow-up list.
func (c Case) Owed() float64 {
	owed := c.Fee - c.Paid
	if owed < 0 {
		return 0
	}
	return owed
}

type Payment struct {
	ID     string
	Amount float64
	PaidAt time.Time
}

// Client is a row in the advocate's own client list: contact details plus the
// totals of the cases that make the client theirs. Address, birthdate and
// РНОКПП are absent by construction — the adapter never selects them.
type Client struct {
	ID         string
	Name       string
	Phone      string
	Cases      int
	Fee        float64
	Paid       float64
	LastCaseAt time.Time
}

// Card is one client as the advocate sees them: contact, their own cases, the
// client's consultations (they are working with this person, so the visit
// history is theirs to see) and the shared note log. No firm-wide revenue
// total, and no other advocate's cases.
type Card struct {
	Client        Client
	Cases         []Case
	Consultations []Consultation
	Notes         []Note
}

type Consultation struct {
	ID          string
	ScheduledAt time.Time
	Price       float64
	Status      string
	CaseNote    string
}

type Note struct {
	ID        string
	Text      string
	CreatedBy string
	CreatedAt time.Time
}

// MonthMoney is one month of the advocate's own collections and what their
// percentage accrues on it.
type MonthMoney struct {
	Month     string // YYYY-MM
	Collected float64
	Accrued   float64
}

// Settlement is the advocate's own money and nothing else — no colleagues'
// rows, no firm totals, no unattributed pool.
type Settlement struct {
	AdvocateID        string
	FullName          string
	CommissionPercent float64
	Collected         float64
	Accrued           float64
	Paid              float64
	Outstanding       float64
	Months            []MonthMoney
	// PaidIsPartial warns that Paid counts only payouts the generator booked
	// (external_ref = advocate:<id>:<month>); lump sums entered by hand or
	// imported from the spreadsheet name nobody, so Outstanding may be too
	// high. Showing this beats presenting a number as final when it is not.
	PaidIsPartial bool
}

// Stats is the advocate's own scoreboard, derived from the cases they can
// already see — no extra query, and nothing in it that a case list would not
// have told them anyway.
type Stats struct {
	Cases         int
	Clients       int
	ByStatus      []StatusCount
	FeeTotal      float64
	PaidTotal     float64
	ClientDebt    float64
	AvgFee        float64
	Months        []MonthMoney
	FirstCaseAt   time.Time
	LastPaymentAt time.Time
}

type StatusCount struct {
	Status string
	Count  int
}
