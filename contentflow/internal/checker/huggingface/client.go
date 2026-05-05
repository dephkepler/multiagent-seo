// Package huggingface implements checker.Client using HuggingFace's
// Inference API (router.huggingface.co/hf-inference) and a text-classification
// AI-detection model. The default model — Hello-SimpleAI/chatgpt-detector-roberta —
// returns a softmax over {ChatGPT, Human}; AIScore is taken from the ChatGPT label.
//
// The model was trained on English HC3 data, so accuracy on non-English content
// (e.g. Russian SEO articles) is limited. It is intended primarily for trying the
// originality-check pipeline against a free, real provider while a paid detector
// (Originality.ai, GPTZero) is being procured.
package huggingface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"contentflow/internal/checker"
)

const (
	defaultModel    = "Hello-SimpleAI/chatgpt-detector-roberta"
	apiBase         = "https://router.huggingface.co/hf-inference/models/"
	requestTimeout  = 60 * time.Second
	maxInputChars   = 1500 // RoBERTa context is ~512 tokens; trim to keep latency predictable
	coldStartMaxTry = 3
	coldStartDelay  = 20 * time.Second
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

// Inference API response shape: [[{"label":"ChatGPT","score":0.92},{"label":"Human","score":0.08}]]
type label struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func (c *Client) Check(ctx context.Context, content string) (*checker.Result, error) {
	input := content
	if len(input) > maxInputChars {
		input = input[:maxInputChars]
	}

	body, err := json.Marshal(map[string]any{"inputs": input})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := apiBase + c.model

	var labels []label
	for attempt := 1; attempt <= coldStartMaxTry; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := c.http.Do(req)
		if doErr != nil {
			return nil, fmt.Errorf("huggingface request: %w", doErr)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Cold start — model loading on HF infra. Wait and retry.
		if resp.StatusCode == http.StatusServiceUnavailable {
			c.log.Warn("huggingface model loading, retrying",
				"model", c.model, "attempt", attempt, "max", coldStartMaxTry)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(coldStartDelay):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("huggingface returned %d: %s", resp.StatusCode, string(respBody))
		}

		var nested [][]label
		if err := json.Unmarshal(respBody, &nested); err != nil {
			return nil, fmt.Errorf("decode response: %w (body: %s)", err, string(respBody))
		}
		if len(nested) == 0 || len(nested[0]) == 0 {
			return nil, fmt.Errorf("huggingface returned empty labels: %s", string(respBody))
		}
		labels = nested[0]
		break
	}
	if labels == nil {
		return nil, fmt.Errorf("huggingface model %q still loading after %d attempts", c.model, coldStartMaxTry)
	}

	// Pick the AI score: the label that is NOT "Human" / "Real" / "Fake-Human".
	// This keeps the parser tolerant to model variations (e.g. desklib uses
	// {"AI","Human"}, others use {"ChatGPT","Human"}).
	var aiScore float64
	for _, l := range labels {
		if !isHumanLabel(l.Label) {
			if l.Score > aiScore {
				aiScore = l.Score
			}
		}
	}

	res := &checker.Result{
		AIScore:         round2(aiScore),
		PlagiarismScore: 0, // HF detector does not provide plagiarism signal
		Original:        aiScore < c.aiThreshold,
		Provider:        "huggingface:" + c.model,
	}
	if !res.Original {
		res.Issues = []string{
			fmt.Sprintf("AI-likelihood %.2f exceeds threshold %.2f", aiScore, c.aiThreshold),
		}
	}
	return res, nil
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
