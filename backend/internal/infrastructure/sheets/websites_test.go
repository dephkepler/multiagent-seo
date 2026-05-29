package sheets

import (
	"fmt"
	"testing"

	"multiagent-seo/internal/domain/linkbuilding"
)

func TestSuitableCell(t *testing.T) {
	if got := suitableCell(true); got != "yes" {
		t.Errorf("suitableCell(true) = %q, want %q", got, "yes")
	}
	if got := suitableCell(false); got != "no" {
		t.Errorf("suitableCell(false) = %q, want %q", got, "no")
	}
}

func TestResultRangeTargetsRow(t *testing.T) {
	r := linkbuilding.Result{Row: 7}
	got := fmt.Sprintf("%s!B%d:D%d", "Donors", r.Row, r.Row)
	want := "Donors!B7:D7"
	if got != want {
		t.Errorf("range = %q, want %q", got, want)
	}
}
