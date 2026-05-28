// Package huggingface implements checker.Client against HuggingFace's
// Inference API. The default model is trained on English HC3 data, so accuracy
// on non-English content is limited.
package huggingface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Result is the checker output this client produces. It is a standalone type
// (rather than importing the parent checker package) so the dependency only
// ever points checker -> huggingface, avoiding an import cycle with the
// factory that lives in the checker package.
type Result struct {
	AIScore          float64  `json:"ai_score"`
	PlagiarismScore  float64  `json:"plagiarism_score"`
	Original         bool     `json:"original"`
	Provider         string   `json:"provider"`
	Issues           []string `json:"issues,omitempty"`
	SentencesFlagged []string `json:"sentences_flagged,omitempty"`
	ReportURL        string   `json:"report_url,omitempty"`
}

const (
	defaultModel    = "Hello-SimpleAI/chatgpt-detector-roberta"
	apiBase         = "https://router.huggingface.co/hf-inference/models/"
	requestTimeout  = 60 * time.Second
	maxInputChars   = 1500 // RoBERTa context is ~512 tokens
	coldStartMaxTry = 3
	coldStartDelay  = 20 * time.Second

	minSentenceChars     = 40 // shorter snippets score too noisily to be useful
	maxSentencesScored   = 20 // bound API cost: at most N sentence-level calls
	maxFlaggedReturned   = 5  // K worst sentences fed to the humanize prompt
	sentenceCallParallel = 4  // in-flight cap for sentence-level calls
)

type Client struct {
	apiKey      string
	model       string
	aiThreshold float64
	http        *http.Client
	log         *slog.Logger
}

func New(apiKey, model string, aiThreshold float64, log *slog.Logger) *Client {
	if model == "" {
		model = defaultModel
	}
	if aiThreshold == 0 {
		aiThreshold = 0.8
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		apiKey:      apiKey,
		model:       model,
		aiThreshold: aiThreshold,
		http:        &http.Client{Timeout: requestTimeout},
		log:         log,
	}
}

// Response shape: [[{"label":"ChatGPT","score":0.92},{"label":"Human","score":0.08}]].
type label struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func (c *Client) Check(ctx context.Context, content string) (*Result, error) {
	input := content
	if len(input) > maxInputChars {
		input = input[:maxInputChars]
	}

	aiScore, err := c.score(ctx, input)
	if err != nil {
		return nil, err
	}

	res := &Result{
		AIScore:         round2(aiScore),
		PlagiarismScore: 0, // HF detector has no plagiarism signal
		Original:        aiScore < c.aiThreshold,
		Provider:        "huggingface:" + c.model,
	}

	// Sentence-level pass is purely diagnostic — it populates SentencesFlagged
	// for the humanize prompt and never overrides the headline AIScore.
	flagged := c.flagSentences(ctx, input)
	res.SentencesFlagged = flagged

	if !res.Original {
		res.Issues = []string{
			fmt.Sprintf("AI-likelihood %.2f exceeds threshold %.2f", aiScore, c.aiThreshold),
		}
		if len(flagged) > 0 {
			res.Issues = append(res.Issues, fmt.Sprintf("flagged %d sentences for rewrite", len(flagged)))
		}
	}
	return res, nil
}

// score sends one input through the detector and returns the max non-human label score.
func (c *Client) score(ctx context.Context, input string) (float64, error) {
	body, err := json.Marshal(map[string]any{"inputs": input})
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	url := apiBase + c.model

	var labels []label
	for attempt := 1; attempt <= coldStartMaxTry; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if reqErr != nil {
			return 0, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := c.http.Do(req)
		if doErr != nil {
			return 0, fmt.Errorf("huggingface request: %w", doErr)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// 503 means the model is cold-loading on HF infra; back off and retry.
		if resp.StatusCode == http.StatusServiceUnavailable {
			c.log.Warn("huggingface model loading, retrying",
				"model", c.model, "attempt", attempt, "max", coldStartMaxTry)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(coldStartDelay):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("huggingface returned %d: %s", resp.StatusCode, string(respBody))
		}

		var nested [][]label
		if err := json.Unmarshal(respBody, &nested); err != nil {
			return 0, fmt.Errorf("decode response: %w (body: %s)", err, string(respBody))
		}
		if len(nested) == 0 || len(nested[0]) == 0 {
			return 0, fmt.Errorf("huggingface returned empty labels: %s", string(respBody))
		}
		labels = nested[0]
		break
	}
	if labels == nil {
		return 0, fmt.Errorf("huggingface model %q still loading after %d attempts", c.model, coldStartMaxTry)
	}

	// Pick any non-human label so we tolerate model variations (ChatGPT/AI/etc.).
	var aiScore float64
	for _, l := range labels {
		if !isHumanLabel(l.Label) {
			if l.Score > aiScore {
				aiScore = l.Score
			}
		}
	}
	return aiScore, nil
}

// flagSentences returns up to maxFlaggedReturned sentences whose individual AI
// score meets the threshold, ordered by score descending. Failures on individual
// sentences are logged and skipped — they never bubble up.
func (c *Client) flagSentences(ctx context.Context, input string) []string {
	sentences := splitSentences(input)
	if len(sentences) == 0 {
		return nil
	}
	if len(sentences) > maxSentencesScored {
		// Longest sentences first — they're the highest-signal candidates and
		// fit the per-call budget defined by maxSentencesScored.
		sort.SliceStable(sentences, func(i, j int) bool {
			return len(sentences[i]) > len(sentences[j])
		})
		sentences = sentences[:maxSentencesScored]
	}

	type scored struct {
		text  string
		score float64
	}
	results := make([]scored, len(sentences))

	// Bounded concurrency with stdlib only: WaitGroup + buffered semaphore.
	sem := make(chan struct{}, sentenceCallParallel)
	var wg sync.WaitGroup
	for i, s := range sentences {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, s string) {
			defer wg.Done()
			defer func() { <-sem }()
			score, err := c.score(ctx, s)
			if err != nil {
				c.log.Warn("huggingface sentence scoring failed, skipping",
					"err", err, "len", len(s))
				return
			}
			results[i] = scored{text: s, score: score}
		}(i, s)
	}
	wg.Wait()

	kept := results[:0]
	for _, r := range results {
		if r.text != "" && r.score >= c.aiThreshold {
			kept = append(kept, r)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].score > kept[j].score
	})
	if len(kept) > maxFlaggedReturned {
		kept = kept[:maxFlaggedReturned]
	}
	if len(kept) == 0 {
		return nil
	}
	out := make([]string, len(kept))
	for i, r := range kept {
		out[i] = r.text
	}
	return out
}

// splitSentences breaks text on `. `, `! `, `? ` when followed by an uppercase
// letter or end-of-text. Trims whitespace and drops fragments shorter than
// minSentenceChars to avoid scoring noise on stubs.
func splitSentences(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i := 0; i < len(runes)-1; i++ {
		r := runes[i]
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		if runes[i+1] != ' ' {
			continue
		}
		// Require uppercase (or end) after the space to dodge "U.S. dollars" etc.
		if i+2 >= len(runes) || unicode.IsUpper(runes[i+2]) {
			seg := strings.TrimSpace(string(runes[start : i+1]))
			if len(seg) >= minSentenceChars {
				out = append(out, seg)
			}
			start = i + 2
		}
	}
	if start < len(runes) {
		seg := strings.TrimSpace(string(runes[start:]))
		if len(seg) >= minSentenceChars {
			out = append(out, seg)
		}
	}
	return out
}

func isHumanLabel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "human", "real", "label_0":
		return true
	}
	return false
}

func round2(v float64) float64 {
	return float64(int(v*100)) / 100
}
