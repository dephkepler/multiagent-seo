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

func TestParseAVerdicts(t *testing.T) {
	// A:D = URL | topic | outbound | suitable
	values := [][]any{
		{"URL", "topic", "outbound", "suitable"},                 // header → skipped (col A not a URL)
		{"https://travel.example", "travel", 7, "yes"},           // suitable travel
		{"https://Greenworld.Com/", "travel", 1, "yes"},          // upper-case + trailing slash → normalized
		{"https://acme.com", "tech", 5, "no"},                    // unsuitable
		{"not-a-url", "topic", 1, "yes"},                         // garbage → skipped
		{"https://no-topic.example", "", 0, ""},                  // empty topic + suitable=false
	}
	got := parseAVerdicts(values)

	if v, ok := got["https://travel.example"]; !ok || v.Topic != "travel" || !v.Suitable {
		t.Errorf("travel.example: %+v ok=%v", v, ok)
	}
	// Trailing slash + capital letters get normalized to lower-case, no slash.
	if v, ok := got["https://greenworld.com"]; !ok || !v.Suitable {
		t.Errorf("greenworld.com (normalized): %+v ok=%v", v, ok)
	}
	if v, ok := got["https://acme.com"]; !ok || v.Suitable {
		t.Errorf("acme.com should be present but unsuitable: %+v ok=%v", v, ok)
	}
	if _, ok := got["not-a-url"]; ok {
		t.Error("garbage URL should be skipped")
	}
}

func TestParseECredentialsJoin(t *testing.T) {
	aVerdicts := map[string]qualVerdict{
		"https://greenworldhotels.com": {Topic: "travel", Suitable: true},
		"https://pennyforward.com":     {Topic: "ecommerce", Suitable: false},
		// Note: "https://random.example" intentionally absent — not in column A.
	}

	// E:I = baseURL | login | password | login_status | placement_status
	values := [][]any{
		{"baseURL", "login", "password", "status", "placement"},          // row 1: header (E not URL)
		{"https://greenworldhotels.com", "root", "pw", "login ok", ""},   // row 2: suitable travel → kept
		{"https://Pennyforward.com/", "u", "p", "", ""},                  // row 3: unsuitable per A → rejected_not_suitable
		{"https://random.example", "u", "p", "", ""},                     // row 4: not in A at all → rejected_unknown
		{"https://greenworldhotels.com/", "root", "pw", "", "placed: x"}, // row 5: trailing-slash variant joins → kept
		{"https://acme.com", "", "", "", ""},                             // row 6: missing creds → skipped silently
	}

	got, rejUnknown, rejNotSuit := parseECredentialsJoin(values, aVerdicts)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2; got=%+v", len(got), got)
	}
	if got[0].Row != 2 || got[0].BaseURL != "https://greenworldhotels.com" || got[0].Topic != "travel" || got[0].LoginStatus != "login ok" {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Row != 5 || got[1].PlacementStatus != "placed: x" {
		t.Errorf("second (trailing-slash variant): %+v", got[1])
	}
	if rejUnknown != 1 {
		t.Errorf("rejUnknown = %d, want 1 (random.example)", rejUnknown)
	}
	if rejNotSuit != 1 {
		t.Errorf("rejNotSuit = %d, want 1 (pennyforward unsuitable)", rejNotSuit)
	}
}

func TestStaleEStatusRows(t *testing.T) {
	aVerdicts := map[string]qualVerdict{
		"https://keep.example": {Topic: "travel", Suitable: true},
		// stale.example absent → orphan
	}
	// E:H = baseURL | login | password | login_status
	values := [][]any{
		{"baseURL", "login", "password", "status"},          // row 1: header (E not URL)
		{"https://keep.example", "u", "p", "login ok"},      // row 2: suitable → keep
		{"https://stale.example", "u", "p", "login ok"},     // row 3: not in A + has status → clear
		{"https://blank.example", "u", "p", ""},             // row 4: empty status → no-op
		{"not-a-url", "", "", "failed"},                     // row 5: not a site row → skip
	}
	got := staleEStatusRows(values, aVerdicts)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("staleEStatusRows = %v, want [3]", got)
	}
}
