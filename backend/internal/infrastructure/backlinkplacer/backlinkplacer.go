// Package backlinkplacer implements the linkbuilding.BacklinkPlacer port by
// asking an LLM to weave one inline anchor link into existing post HTML and
// to return both the chosen anchor and the modified HTML in a separator-based
// format (no JSON), which survives unescaped HTML in the body.
package backlinkplacer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"multiagent-seo/internal/domain/linkbuilding"
)

// LLM is the minimal completion surface this package needs. main.go adapts the
// existing provider client to it, the same way topicclassifier does.
type LLM interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}

const (
	// maxInputHTML caps how much of the post body we ship to the LLM. Posts
	// longer than this get truncated; placement still happens inside whatever
	// the LLM saw, which is the leading content for typical articles.
	maxInputHTML = 20000
	// maxTokens has to fit the modified HTML in the reply.
	maxTokens = 6000
	// htmlSeparator splits the reply between metadata (anchor) and raw HTML.
	// Using a literal sentinel instead of JSON dodges the unescaped '<' problem
	// Llama-3.3 hit when asked to embed HTML inside a JSON string value.
	htmlSeparator = "---HTML---"
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

	reply, err := p.llm.Complete(ctx, buildPrompt(src, targetURL), maxTokens)
	if err != nil {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: %w", err)
	}

	out, err := parseReply(reply)
	if err != nil {
		// Surface the head of the raw reply so we can diagnose why the model
		// broke the format without rerunning the whole job.
		p.log.WarnContext(ctx, "backlink llm parse failed",
			"err", err,
			"reply_head", head(reply, 500),
			"target_url", targetURL,
		)
		return linkbuilding.BacklinkInsertion{}, err
	}
	if !strings.Contains(out.ModifiedHTML, targetURL) {
		p.log.WarnContext(ctx, "backlink llm omitted target url",
			"target_url", targetURL,
			"reply_head", head(reply, 500),
		)
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: target URL missing from modified html")
	}

	p.log.DebugContext(ctx, "backlink placed", "anchor", out.Anchor, "target_url", targetURL)
	return out, nil
}

func buildPrompt(html, targetURL string) string {
	var b strings.Builder
	b.WriteString(`You are editing an existing WordPress blog post. Insert exactly ONE inline backlink to the URL below into the most contextually-fitting sentence of the post. Pick a natural anchor of 2-4 words drawn from the surrounding text; never invent unrelated phrasing. Do not paraphrase, add new sentences, change punctuation outside the link, or alter HTML structure beyond the new <a> tag. Use rel="noopener".`)
	b.WriteString("\n\nURL to link to: ")
	b.WriteString(targetURL)
	b.WriteString("\n\nExisting post HTML:\n")
	b.WriteString(html)
	b.WriteString("\n\nReply with EXACTLY this two-part format and nothing else (no prose, no markdown fences):\n")
	b.WriteString("ANCHOR: <chosen 2-4 word anchor>\n")
	b.WriteString(htmlSeparator)
	b.WriteString("\n<the entire modified post HTML with your single inline <a> tag inserted>")
	return b.String()
}

// parseReply pulls anchor + HTML out of the model's reply. The HTML half lives
// after the separator and is taken verbatim — no JSON-escaping required.
func parseReply(reply string) (linkbuilding.BacklinkInsertion, error) {
	s := strings.TrimSpace(reply)
	// Some models still wrap the whole thing in a code fence despite the
	// instruction; peel the most common variants.
	s = strings.TrimPrefix(s, "```html")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	idx := strings.Index(s, htmlSeparator)
	if idx < 0 {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: separator %q missing from reply", htmlSeparator)
	}

	head := s[:idx]
	body := strings.TrimSpace(s[idx+len(htmlSeparator):])
	body = strings.TrimPrefix(body, "```html")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	anchor := extractAnchor(head)
	if anchor == "" {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: ANCHOR line missing from reply")
	}
	if body == "" {
		return linkbuilding.BacklinkInsertion{}, fmt.Errorf("backlink llm: html body missing from reply")
	}
	return linkbuilding.BacklinkInsertion{Anchor: anchor, ModifiedHTML: body}, nil
}

func extractAnchor(head string) string {
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "ANCHOR:") {
			continue
		}
		v := strings.TrimSpace(line[len("ANCHOR:"):])
		// Strip optional quoting the model sometimes adds.
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
