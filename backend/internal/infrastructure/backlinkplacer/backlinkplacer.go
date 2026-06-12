package backlinkplacer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/internal/domain/linkbuilding/prompt"
)

type LLM interface {
	Complete(ctx context.Context, p string, maxTokens int) (string, error)
}

const (
	maxInputHTML = 20000
	maxTokens    = 2000
)

type Placer struct {
	llm LLM
	log *slog.Logger
}

func New(llm LLM, log *slog.Logger) *Placer {
	if log == nil {
		log = slog.Default()
	}
	return &Placer{llm: llm, log: log}
}

func (p *Placer) Place(ctx context.Context, html, targetURL string) (linkbuilding.BacklinkInsertion, error) {
	src := html
	if len(src) > maxInputHTML {
		src = src[:maxInputHTML]
	}

	reply, err := p.llm.Complete(ctx, prompt.Placer(src, targetURL), maxTokens)
	if err != nil {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: %w", err)
	}

	anchor, original, modified, err := parseReply(reply)
	if err != nil {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm parse: %w (reply head: %s)", err, head(reply, 300))
	}
	if !strings.Contains(modified, targetURL) {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: target URL missing from modified paragraph (reply head: %s)", head(reply, 300))
	}
	if !strings.Contains(html, original) {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: original paragraph not found verbatim in source html, model paraphrased (original head: %s)", head(original, 200))
	}

	full := strings.Replace(html, original, modified, 1)
	return linkbuilding.BacklinkInsertion{Anchor: anchor, ModifiedHTML: full}, nil
}

func parseReply(reply string) (anchor, original, modified string, err error) {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```html")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	origIdx := strings.Index(s, prompt.SepOriginal)
	if origIdx < 0 {
		return "", "", "", fmt.Errorf("backlink llm: separator %q missing from reply", prompt.SepOriginal)
	}
	headPart := s[:origIdx]
	rest := s[origIdx+len(prompt.SepOriginal):]

	modIdx := strings.Index(rest, prompt.SepModified)
	if modIdx < 0 {
		return "", "", "", fmt.Errorf("backlink llm: separator %q missing from reply", prompt.SepModified)
	}
	original = strings.TrimSpace(rest[:modIdx])
	modified = strings.TrimSpace(rest[modIdx+len(prompt.SepModified):])

	for _, fence := range []string{"```html", "```"} {
		original = strings.TrimPrefix(original, fence)
		modified = strings.TrimPrefix(modified, fence)
	}
	original = strings.TrimSuffix(original, "```")
	modified = strings.TrimSuffix(modified, "```")
	original = strings.TrimSpace(original)
	modified = strings.TrimSpace(modified)

	anchor = extractAnchor(headPart)
	if anchor == "" {
		return "", "", "", fmt.Errorf("backlink llm: ANCHOR line missing from reply")
	}
	if original == "" {
		return "", "", "", fmt.Errorf("backlink llm: ORIGINAL section empty")
	}
	if modified == "" {
		return "", "", "", fmt.Errorf("backlink llm: MODIFIED section empty")
	}
	return anchor, original, modified, nil
}

func extractAnchor(headPart string) string {
	for _, line := range strings.Split(headPart, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "ANCHOR:") {
			continue
		}
		v := strings.TrimSpace(line[len("ANCHOR:"):])
		v = strings.Trim(v, "\"'`")
		return strings.TrimSpace(v)
	}
	return ""
}

func head(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
