package linkbuilding

import "testing"

func TestCountExternalDomains(t *testing.T) {
	links := []string{
		"https://other.com/page",          // external
		"https://other.com/another",       // same external domain → not double-counted
		"http://second.net",               // external
		"https://www.mysite.com/internal", // subdomain of own → internal
		"https://mysite.com/x",            // own apex → internal
		"/relative/path",                  // relative → no host → skip
		"#anchor",                         // anchor → skip
		"mailto:hi@mysite.com",            // mailto → skip
		"https://sub.example.co.uk/a",     // external (eTLD+1 example.co.uk)
		"javascript:void(0)",              // skip
	}
	got := CountExternalDomains("https://mysite.com", links)
	if got != 3 { // other.com, second.net, example.co.uk
		t.Errorf("CountExternalDomains = %d, want 3", got)
	}
}

func TestCountExternalDomains_NoLinks(t *testing.T) {
	if got := CountExternalDomains("https://mysite.com", nil); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestIsSuitable(t *testing.T) {
	accepted := []string{"Gambling", "news"}
	cases := map[string]bool{
		"gambling": true,  // case-insensitive match
		"NEWS":     true,  // case-insensitive match
		"  news  ": true,  // trimmed
		"tech":     false, // not accepted
		"":         false, // empty topic never suitable
	}
	for topic, want := range cases {
		if got := IsSuitable(topic, accepted); got != want {
			t.Errorf("IsSuitable(%q) = %v, want %v", topic, got, want)
		}
	}
	if IsSuitable("gambling", nil) {
		t.Error("empty accepted set must never be suitable")
	}
}
