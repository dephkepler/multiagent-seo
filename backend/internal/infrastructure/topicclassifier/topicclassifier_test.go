package topicclassifier

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"multiagent-seo/internal/domain/linkbuilding"
)

type fakeLLM struct {
	reply string
	err   error
}

func (f fakeLLM) Complete(context.Context, string, int) (string, error) {
	return f.reply, f.err
}

func newTestClassifier(reply string) *Classifier {
	return New(fakeLLM{reply: reply}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestClassify(t *testing.T) {
	candidates := []string{"Tech", "Finance", "Travel"}
	page := linkbuilding.Page{Title: "Example"}

	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{"exact match case-insensitive returns original casing", "tech", "Tech"},
		{"surrounding quotes and punctuation still match", " \"Finance.\" \n", "Finance"},
		{"unknown reply yields empty", "Sports", ""},
		{"other yields empty", "other", ""},
		{"none yields empty", "none", ""},
		{"empty reply yields empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newTestClassifier(tt.reply).Classify(context.Background(), page, candidates)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyMultiWordCandidate(t *testing.T) {
	// Would have failed under the old 20-token cap that truncated the reply.
	candidates := []string{"Web Development", "Finance", "Travel"}
	page := linkbuilding.Page{Title: "Building modern web apps"}

	got, err := newTestClassifier("Web Development").Classify(context.Background(), page, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Web Development" {
		t.Fatalf("Classify() = %q, want %q", got, "Web Development")
	}
}

func TestClassifyMalformedReply(t *testing.T) {
	candidates := []string{"Tech", "Finance"}
	page := linkbuilding.Page{Title: "Example"}

	tests := []struct {
		name  string
		reply string
	}{
		{"empty reply yields none", ""},
		{"whitespace-only reply yields none", "   \n\t "},
		{"punctuation-only reply yields none", "...!?"},
		{"unrelated prose yields none", "I cannot determine this"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Contract: malformed/empty reply maps to "" (none) without error.
			got, err := newTestClassifier(tt.reply).Classify(context.Background(), page, candidates)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "" {
				t.Fatalf("Classify() = %q, want empty", got)
			}
		})
	}
}

func TestClassifyNormalizeRobustness(t *testing.T) {
	candidates := []string{"Tech", "Web Development"}
	page := linkbuilding.Page{Title: "Example"}

	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{"trailing punctuation and quotes", "\"Tech!\"", "Tech"},
		{"uppercase with surrounding whitespace", "  TECH  ", "Tech"},
		{"backticked lowercase", "`tech`", "Tech"},
		{"multi-word with punctuation and case", " (web development). ", "Web Development"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newTestClassifier(tt.reply).Classify(context.Background(), page, candidates)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyEmptyCandidates(t *testing.T) {
	got, err := newTestClassifier("Tech").Classify(context.Background(), linkbuilding.Page{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("Classify() = %q, want empty", got)
	}
}
