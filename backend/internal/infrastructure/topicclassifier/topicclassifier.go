// Package topicclassifier implements the linkbuilding.TopicClassifier port by
// asking an LLM to pick the page's main topic from a candidate list.
package topicclassifier

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"multiagent-seo/internal/domain/linkbuilding"
)

// LLM is the minimal completion surface this package needs. main.go adapts the
// existing provider client to it, keeping this package off the articles context.
type LLM interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}

const (
	classifyMaxTokens = 20
	textSampleLimit   = 1500
)

type Classifier struct {
	llm LLM
	log *slog.Logger
}

var _ linkbuilding.TopicClassifier = (*Classifier)(nil)

func New(llm LLM, log *slog.Logger) *Classifier {
	return &Classifier{llm: llm, log: log}
}

func (c *Classifier) Classify(ctx context.Context, page linkbuilding.Page, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}

	prompt := buildPrompt(page, candidates)

	reply, err := c.llm.Complete(ctx, prompt, classifyMaxTokens)
	if err != nil {
		return "", fmt.Errorf("classify topic: %w", err)
	}

	topic := matchCandidate(reply, candidates)
	c.log.DebugContext(ctx, "topic classified",
		"reply", strings.TrimSpace(reply),
		"matched", topic,
	)
	return topic, nil
}

func buildPrompt(page linkbuilding.Page, candidates []string) string {
	var b strings.Builder

	b.WriteString("Classify the website below into exactly one of these topics:\n")
	b.WriteString(strings.Join(candidates, ", "))
	b.WriteString("\n\nReply with ONLY the single topic word, no prose or punctuation. ")
	b.WriteString("If none of the topics fit, reply: none\n\n")

	if t := strings.TrimSpace(page.Title); t != "" {
		fmt.Fprintf(&b, "Title: %s\n", t)
	}
	if m := strings.TrimSpace(page.MetaDescription); m != "" {
		fmt.Fprintf(&b, "Meta: %s\n", m)
	}
	if h := strings.TrimSpace(strings.Join(page.Headings, " | ")); h != "" {
		fmt.Fprintf(&b, "Headings: %s\n", h)
	}
	if s := strings.TrimSpace(page.TextSample); s != "" {
		fmt.Fprintf(&b, "Text: %s\n", trimSample(s, textSampleLimit))
	}

	return b.String()
}

func trimSample(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

func matchCandidate(reply string, candidates []string) string {
	norm := normalize(reply)
	if norm == "" {
		return ""
	}
	for _, cand := range candidates {
		if normalize(cand) == norm {
			return cand
		}
	}
	return ""
}

// normalize lowercases and strips surrounding quotes/punctuation/whitespace so a
// reply like `"Tech."` still matches the candidate `Tech`.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Trim(s, " \t\r\n.,;:!?\"'`()[]{}")
}
