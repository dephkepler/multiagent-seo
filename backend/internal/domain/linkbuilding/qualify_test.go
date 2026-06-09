package linkbuilding

import "testing"

func TestCountExternalDomains(t *testing.T) {
	links := []string{
		"https://other.com/page",
		"https://other.com/another",
		"http://second.net",
		"https://www.mysite.com/internal",
		"https://mysite.com/x",
		"/relative/path",
		"#anchor",
		"mailto:hi@mysite.com",
		"https://sub.example.co.uk/a",
		"javascript:void(0)",
	}
	got := CountExternalDomains("https://mysite.com", links)
	if got != 3 {
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
		"gambling": true,
		"NEWS":     true,
		"  news  ": true,
		"tech":     false,
		"":         false,
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
