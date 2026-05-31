package sheets

import (
	"testing"
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
	cases := []struct {
		sheet string
		row   int
		want  string
	}{
		{"Donors", 7, "Donors!B7:D7"},
		{"Donors", 1, "Donors!B1:D1"},
		{"Targets", 42, "Targets!B42:D42"},
	}
	for _, c := range cases {
		if got := resultRange(c.sheet, c.row); got != c.want {
			t.Errorf("resultRange(%q, %d) = %q, want %q", c.sheet, c.row, got, c.want)
		}
	}
}

func TestParseCredentialRows(t *testing.T) {
	// Columns are D:G = suitable | URL | login | password.
	values := [][]any{
		{"suitable", "URL", "login", "password"},                    // row 1: header → skipped
		{"yes", "https://shdacademy.vn", "monamedia", "MonaM@123"},   // row 2: suitable → kept
		{"no", "https://unsuitable.example", "user", "pass"},        // row 3: not suitable → skipped
		{"yes", "https://only-url.example"},                         // row 4: no login/pass → skipped
		{"yes", "not-a-url", "user", "pass"},                        // row 5: not a URL → skipped
		{"YES", " https://trimmed.example ", " admin ", " pw "},     // row 6: suitable (any case), trimmed → kept
	}

	got := parseCredentialRows(values)

	if len(got) != 2 {
		t.Fatalf("got %d credentials, want 2 (only suitable rows): %+v", len(got), got)
	}
	if got[0].Row != 2 || got[0].BaseURL != "https://shdacademy.vn" || got[0].Login != "monamedia" || got[0].Password != "MonaM@123" {
		t.Errorf("first credential wrong: %+v", got[0])
	}
	// row number preserved as the 1-based sheet line, fields trimmed, "YES" matched.
	if got[1].Row != 6 || got[1].BaseURL != "https://trimmed.example" || got[1].Login != "admin" || got[1].Password != "pw" {
		t.Errorf("second credential wrong: %+v", got[1])
	}
}

func TestStaleStatusRows(t *testing.T) {
	// Columns are D:H = suitable | URL | login | password | status.
	values := [][]any{
		{"suitable", "URL", "login", "password", "status"},   // row 1: header (E not URL) → skip
		{"yes", "https://keep.example", "u", "p", "login ok"}, // row 2: suitable → keep
		{"no", "https://stale.example", "u", "p", "login ok"}, // row 3: unsuitable + status → clear
		{"no", "https://blank.example", "u", "p", ""},         // row 4: unsuitable but no status → skip
		{"no", "not-a-url", "", "", "failed"},                 // row 5: not a site row → skip
	}

	got := staleStatusRows(values)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("staleStatusRows = %v, want [3]", got)
	}
}
