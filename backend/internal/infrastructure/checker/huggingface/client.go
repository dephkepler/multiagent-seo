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
	"sync/atomic"
	"time"
	"unicode"

	"multiagent-seo/internal/domain/articles"
)

const (
	defaultModel    = "Hello-SimpleAI/chatgpt-detector-roberta"
	apiBase         = "https://router.huggingface.co/hf-inference/models/"
	requestTimeout  = 60 * time.Second
	maxInputChars   = 1500
	coldStartMaxTry = 3
	coldStartDelay  = 20 * time.Second

	minSentenceChars     = 40
	maxSentencesScored   = 20
	maxFlaggedReturned   = 5
	sentenceCallParallel = 4
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

type label struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func (c *Client) Check(ctx context.Context, content string) (*articles.CheckResult, error) {
	input := content
	if len(input) > maxInputChars {
		input = input[:maxInputChars]
	}

	aiScore, err := c.score(ctx, input)
	if err != nil {
		return nil, err
	}

	res := &articles.CheckResult{
		AIScore:         round2(aiScore),
		PlagiarismScore: 0,
		Original:        aiScore < c.aiThreshold,
		Provider:        "huggingface:" + c.model,
	}

	flagged, err := c.flagSentences(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("flag sentences: %w", err)
	}
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
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return 0, fmt.Errorf("read response: %w", readErr)
		}

		if resp.StatusCode == http.StatusServiceUnavailable {
			c.log.WarnContext(ctx, "huggingface model loading, retrying",
				"model", c.model, "attempt", attempt, "max", coldStartMaxTry)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(coldStartDelay):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("huggingface returned %d: %s", resp.StatusCode, truncate(respBody))
		}

		var nested [][]label
		if err := json.Unmarshal(respBody, &nested); err != nil {
			return 0, fmt.Errorf("decode response: %w (body: %s)", err, truncate(respBody))
		}
		if len(nested) == 0 || len(nested[0]) == 0 {
			return 0, fmt.Errorf("huggingface returned empty labels: %s", truncate(respBody))
		}
		labels = nested[0]
		break
	}
	if labels == nil {
		return 0, fmt.Errorf("huggingface model %q still loading after %d attempts", c.model, coldStartMaxTry)
	}

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

func (c *Client) flagSentences(ctx context.Context, input string) ([]string, error) {
	sentences := splitSentences(input)
	if len(sentences) == 0 {
		return nil, nil
	}
	if len(sentences) > maxSentencesScored {
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

	sem := make(chan struct{}, sentenceCallParallel)
	var wg sync.WaitGroup
	var failures atomic.Int64
	var firstErr atomic.Pointer[error]
	for i, s := range sentences {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, s string) {
			defer wg.Done()
			defer func() { <-sem }()
			score, err := c.score(ctx, s)
			if err != nil {
				failures.Add(1)
				firstErr.CompareAndSwap(nil, &err)
				return
			}
			results[i] = scored{text: s, score: score}
		}(i, s)
	}
	wg.Wait()
	if n := failures.Load(); n > 0 {
		err := *firstErr.Load()
		return nil, fmt.Errorf("scoring failed for %d/%d sentences: %w", n, len(sentences), err)
	}

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
		return nil, nil
	}
	out := make([]string, len(kept))
	for i, r := range kept {
		out[i] = r.text
	}
	return out, nil
}

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

func truncate(b []byte) string {
	const maxErrBody = 4 << 10
	if len(b) > maxErrBody {
		return string(b[:maxErrBody]) + "...(truncated)"
	}
	return string(b)
}
