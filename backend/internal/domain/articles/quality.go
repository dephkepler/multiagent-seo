package articles

import (
	"strings"
	"unicode/utf8"
)

type QualityInput struct {
	Content        string
	SEOTitle       string
	SEODescription string
	Keywords       []string
	MinWords       int
	MaxWords       int
}

const (
	qualityMinH2       = 5
	qualityWordTol     = 0.9
	qualitySEOTitleMax = 60
	qualityDescMin     = 150
	qualityDescMax     = 160
)

func QualityFloor(in QualityInput) (bool, []string) {
	var reasons []string

	words := len(strings.Fields(in.Content))
	if in.MinWords > 0 && words < int(float64(in.MinWords)*qualityWordTol) {
		reasons = append(reasons, "too few words")
	}
	if in.MaxWords > 0 && words > in.MaxWords {
		reasons = append(reasons, "too many words")
	}

	if countH2(in.Content) < qualityMinH2 {
		reasons = append(reasons, "too few H2")
	}

	low := strings.ToLower(in.Content)
	for _, kw := range in.Keywords {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw != "" && !strings.Contains(low, kw) {
			reasons = append(reasons, "missing keyword: "+kw)
		}
	}

	if t := strings.TrimSpace(in.SEOTitle); t != "" && utf8.RuneCountInString(t) > qualitySEOTitleMax {
		reasons = append(reasons, "seo title too long")
	}
	if d := strings.TrimSpace(in.SEODescription); d != "" {
		n := utf8.RuneCountInString(d)
		if n < qualityDescMin || n > qualityDescMax {
			reasons = append(reasons, "seo description length out of range")
		}
	}

	return len(reasons) == 0, reasons
}

func countH2(content string) int {
	n := 0
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") && !strings.HasPrefix(t, "### ") {
			n++
		}
	}
	return n
}
