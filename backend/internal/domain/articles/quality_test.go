package articles

import (
	"strings"
	"testing"
)

func goodArticle() string {
	var b strings.Builder
	b.WriteString("# Running Shoes Guide\n\n")
	b.WriteString("An intro about running shoes with enough words to pass.\n\n")
	for range 5 {
		b.WriteString("## Section heading\n\nSome body text about running shoes here.\n\n")
	}
	b.WriteString("## FAQ\n\nQ: Is this a question? A: Yes.\n\n")
	// pad word count
	b.WriteString(strings.Repeat("word ", 1000))
	return b.String()
}

func TestQualityFloorPasses(t *testing.T) {
	in := QualityInput{
		Content:  goodArticle(),
		Keywords: []string{"running shoes"},
		MinWords: 500,
		MaxWords: 5000,
	}
	if ok, reasons := QualityFloor(in); !ok {
		t.Errorf("expected pass, got reasons: %v", reasons)
	}
}

func TestQualityFloorCopywriterRules(t *testing.T) {
	cases := map[string]struct {
		mutate func(string) string
		want   string
	}{
		"no H1":      {func(s string) string { return strings.Replace(s, "# Running Shoes Guide", "Running Shoes Guide", 1) }, "does not start with H1"},
		"no FAQ":     {func(s string) string { return strings.Replace(s, "## FAQ", "## Other", 1) }, "missing FAQ section"},
		"bold kw":    {func(s string) string { return strings.Replace(s, "running shoes with", "**running shoes** with", 1) }, "keyword is bold: running shoes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := QualityInput{Content: c.mutate(goodArticle()), Keywords: []string{"running shoes"}, MinWords: 500, MaxWords: 5000}
			ok, reasons := QualityFloor(in)
			if ok {
				t.Fatalf("expected failure for %q", name)
			}
			found := false
			for _, r := range reasons {
				if r == c.want {
					found = true
				}
			}
			if !found {
				t.Errorf("want reason %q, got %v", c.want, reasons)
			}
		})
	}
}
