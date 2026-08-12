// Package cases is the "дело" (клопотання/позов/супровід) that a
// consultation sometimes turns into — a legal case with its own contract
// fee, paid separately and often in installments, unrelated to the
// consultation's own (much smaller) price. This is where most of the
// business's real revenue is — see doc/abalisbotlead for the numbers.
package cases

import "time"

const (
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

// Categories is the fixed practice-area list offered to staff in the /case
// flow — free text is still accepted (Category is a plain string, not an
// enum in the DB), this is just what gets suggested. Picked from what
// actually recurs across both "55k" and "проекти": mobilization-adjacent
// matters dominate everything else combined.
var Categories = []string{
	"Мобілізація / ТЦК / ВЛК",
	"Звільнення зі служби / СЗЧ",
	"Борги / аліменти / майнові спори",
	"Оренда / виселення",
	"Виїзд за кордон",
	"Інше",
}

type Case struct {
	ID             string
	ClientID       string
	ConsultationID string // empty if not linked to a specific consultation
	AdvocateName   string
	Category       string // practice area — free text, see Categories for the suggested list
	Fee            float64 // the agreed contract amount
	PaidAmount     float64 // running total actually received — grows via AddPayment, not a full ledger
	Status         string
	Description    string
	CreatedBy      string
	CreatedAt      time.Time
}

// Owed is what's left to collect — the number that actually matters to the
// business day-to-day ("who still owes us money").
func (c Case) Owed() float64 {
	owed := c.Fee - c.PaidAmount
	if owed < 0 {
		return 0
	}
	return owed
}
