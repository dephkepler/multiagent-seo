package expenseimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Expected holds the two totals the sheet itself states for a month: the one in
// the top summary row ("Расходы") and the one under the per-category
// breakdown. They do not always agree — December 2024 differs by 1 500 — so
// both are carried and both are compared, instead of picking a winner.
type Expected struct {
	Tab       string
	Summary   float64
	Breakdown float64
}

type Diff struct {
	Tab       string
	Parsed    float64
	Summary   float64
	Breakdown float64
}

func (d Diff) SummaryDelta() float64   { return round2(d.Parsed - d.Summary) }
func (d Diff) BreakdownDelta() float64 { return round2(d.Parsed - d.Breakdown) }

// OK is true when the imported rows match at least one of the sheet's own two
// totals — anything else needs a human before the numbers are trusted.
func (d Diff) OK() bool {
	return math.Abs(d.SummaryDelta()) < 0.01 || math.Abs(d.BreakdownDelta()) < 0.01
}

// Header: tab,summary_total,breakdown_total
func ParseExpected(r io.Reader) ([]Expected, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = 3
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("expenseimport: read totals: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("expenseimport: empty totals file")
	}

	out := make([]Expected, 0, len(records)-1)
	for i, rec := range records[1:] {
		summary, err := parseAmount(rec[1])
		if err != nil {
			return nil, fmt.Errorf("expenseimport: totals line %d: summary: %w", i+2, err)
		}
		breakdown, err := parseAmount(rec[2])
		if err != nil {
			return nil, fmt.Errorf("expenseimport: totals line %d: breakdown: %w", i+2, err)
		}
		out = append(out, Expected{Tab: strings.TrimSpace(rec[0]), Summary: summary, Breakdown: breakdown})
	}
	return out, nil
}

// Reconcile pairs what was parsed against what the sheet claims, per tab. A tab
// present in one side and missing from the other still gets a row, with zero on
// the missing side — a silently dropped month is the failure this exists to catch.
func Reconcile(byTab map[string]float64, expected []Expected) []Diff {
	index := make(map[string]Expected, len(expected))
	tabs := make(map[string]struct{}, len(expected)+len(byTab))
	for _, e := range expected {
		index[e.Tab] = e
		tabs[e.Tab] = struct{}{}
	}
	for tab := range byTab {
		tabs[tab] = struct{}{}
	}

	out := make([]Diff, 0, len(tabs))
	for tab := range tabs {
		e := index[tab]
		out = append(out, Diff{
			Tab:       tab,
			Parsed:    round2(byTab[tab]),
			Summary:   e.Summary,
			Breakdown: e.Breakdown,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tab < out[j].Tab })
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
