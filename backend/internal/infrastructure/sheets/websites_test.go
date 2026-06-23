package sheets

import "testing"

func TestParseCredentials(t *testing.T) {
	values := [][]any{
		{"https://a.example", "root", "pw", "login ok", ""},
		{"https://b.example/", "u", "p", "", "placed: x"},
		{"https://nocreds.example", "", "", "", ""},
		{"not-a-url", "u", "p", "", ""},
	}

	got := parseCredentials(values)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2; got=%+v", len(got), got)
	}
	if got[0].Row != 1 || got[0].BaseURL != "https://a.example" || got[0].Login != "root" || got[0].LoginStatus != "login ok" {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Row != 2 || got[1].PlacementStatus != "placed: x" {
		t.Errorf("second: %+v", got[1])
	}
}
