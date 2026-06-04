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
	values := [][]any{
		{"URL", "topic", "outbound", "suitable"},
		{"https://travel.example", "travel", 7, "yes"},
		{"https://Greenworld.Com/", "travel", 1, "yes"},
		{"https://acme.com", "tech", 5, "no"},
		{"not-a-url", "topic", 1, "yes"},
		{"https://no-topic.example", "", 0, ""},
	}
	got := parseAVerdicts(values)

	if v, ok := got["https://travel.example"]; !ok || v.Topic != "travel" || !v.Suitable {
		t.Errorf("travel.example: %+v ok=%v", v, ok)
	}
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
	}

	values := [][]any{
		{"baseURL", "login", "password", "status", "placement"},
		{"https://greenworldhotels.com", "root", "pw", "login ok", ""},
		{"https://Pennyforward.com/", "u", "p", "", ""},
		{"https://random.example", "u", "p", "", ""},
		{"https://greenworldhotels.com/", "root", "pw", "", "placed: x"},
		{"https://acme.com", "", "", "", ""},
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
	}
	values := [][]any{
		{"baseURL", "login", "password", "status"},
		{"https://keep.example", "u", "p", "login ok"},
		{"https://stale.example", "u", "p", "login ok"},
		{"https://blank.example", "u", "p", ""},
		{"not-a-url", "", "", "failed"},
	}
	got := staleEStatusRows(values, aVerdicts)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("staleEStatusRows = %v, want [3]", got)
	}
}
