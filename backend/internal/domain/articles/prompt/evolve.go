package prompt

import (
	"fmt"
	"strings"

	"multiagent-seo/internal/domain/articles"
)

const styleMarker = "=== STYLE (auto) ==="

func SplitWriter(body string) (frozen, evolving string) {
	before, after, found := strings.Cut(body, styleMarker)
	if !found {
		return "", strings.TrimSpace(body)
	}
	return strings.TrimSpace(after), strings.TrimSpace(before)
}

func JoinWriter(frozen, evolving string) string {
	if frozen == "" {
		return evolving
	}
	return evolving + "\n\n" + styleMarker + "\n\n" + frozen
}

func EvolveWriter(evolving string, failures []articles.PromptFailure) string {
	var ev strings.Builder
	for i, f := range failures {
		fmt.Fprintf(&ev, "\nArticle #%d — AI score %.2f\n", i+1, f.AIScore)
		if len(f.Flagged) == 0 {
			ev.WriteString("Flagged sentences: (none captured)\n")
			continue
		}
		ev.WriteString("Flagged sentences:\n")
		for _, s := range f.Flagged {
			fmt.Fprintf(&ev, "  - %s\n", s)
		}
	}

	return fmt.Sprintf(`You are an expert prompt engineer. Below is the "writing style" section of the
instructions an AI writer uses to produce SEO blog articles. Your job: rewrite it
so the articles read as genuinely human-written and stop getting flagged as AI.

(Formatting and structural requirements are enforced separately — focus ONLY on
writing style, not on layout, headings, or SEO rules.)

# Current style instructions
"""
%s
"""

# Evidence — where these instructions fail
An AI-content detector flagged the articles below as machine-written. Each shows the
AI-likelihood (0..1, higher = more robotic) and the exact sentences it flagged.
Treat these as what the current instructions produce too often:
%s
# Improve along these four axes
1. Burstiness — vary sentence length; mix short punchy lines with longer ones, avoid a uniform rhythm.
2. Voice — a personal, opinionated, human tone instead of neutral corporate prose.
3. Specificity — concrete facts, numbers, and examples instead of generic filler.
4. Phrasing — kill formulaic openings, transitions, and closings (e.g. "In conclusion", "It is worth noting", "There are many factors to consider").

# Rules
- Keep every {{placeholder}} (like {{keyword}}) exactly as written.
- Do NOT add formatting, SEO, or layout instructions — style only.
- Output ONLY the rewritten style instructions. No preamble, no explanation.`, evolving, ev.String())
}
