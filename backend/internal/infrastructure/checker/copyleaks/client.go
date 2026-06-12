package copyleaks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"multiagent-seo/internal/domain/articles"
)

const (
	loginURL       = "https://id.copyleaks.com/v3/account/login/api"
	detectBaseURL  = "https://api.copyleaks.com/v2/writer-detector/"
	requestTimeout = 110 * time.Second //Copyleaks has a default request timeout of 110s
	tokenTTL       = 47 * time.Hour
	maxTextChars   = 25000 // Copyleaks accepts up to 25,000 chars for AI detection; we use 25k to leave some room for JSON overhead
	minTextChars   = 255   // Copyleaks requires at least 255 chars to perform a checkx
	sensitivity    = 2
	classAI        = 2  // according to Copyleaks docs, classification 2 means "AI-generated content"
	maxFlagged     = 10 // to avoid excessive memory usage if Copyleaks returns many flagged segments
)

var errUnauthorized = errors.New("copyleaks: unauthorized")

type Client struct {
	email       string
	key         string
	aiThreshold float64
	sandbox     bool
	http        *http.Client
	log         *slog.Logger

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func New(email, key string, aiThreshold float64, sandbox bool, log *slog.Logger) *Client {
	if aiThreshold == 0 {
		aiThreshold = 0.8
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		email:       email,
		key:         key,
		aiThreshold: aiThreshold,
		sandbox:     sandbox,
		http:        &http.Client{Timeout: requestTimeout},
		log:         log,
	}
}

func (c *Client) Check(ctx context.Context, content string) (*articles.CheckResult, error) {
	text := content
	if len(text) > maxTextChars {
		text = text[:maxTextChars]
	}
	if len(text) < minTextChars {
		return nil, fmt.Errorf("copyleaks: text too short (%d chars, min %d)", len(text), minTextChars)
	}

	token, err := c.authToken(ctx)
	if err != nil {
		return nil, err
	}

	scanID := nextScanID()
	det, err := c.detect(ctx, token, scanID, text)
	if errors.Is(err, errUnauthorized) {
		c.invalidateToken()
		if token, err = c.authToken(ctx); err == nil {
			det, err = c.detect(ctx, token, scanID, text)
		}
	}
	if err != nil {
		return nil, err
	}

	res := &articles.CheckResult{
		AIScore:          round2(det.ai),
		Original:         det.ai < c.aiThreshold,
		Provider:         "copyleaks",
		SentencesFlagged: det.flagged,
	}
	if !res.Original {
		res.Issues = []string{
			fmt.Sprintf("AI-likelihood %.2f exceeds threshold %.2f", det.ai, c.aiThreshold),
		}
		if len(det.flagged) > 0 {
			res.Issues = append(res.Issues, fmt.Sprintf("flagged %d segments for rewrite", len(det.flagged)))
		}
	}
	return res, nil
}

func (c *Client) authToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	body, err := json.Marshal(map[string]string{"email": c.email, "key": c.key})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("copyleaks login: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("copyleaks login read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copyleaks login returned %d: %s", resp.StatusCode, truncate(respBody))
	}

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("copyleaks login decode: %w (body: %s)", err, truncate(respBody))
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("copyleaks login: empty access_token")
	}

	c.token = out.AccessToken
	c.tokenExp = time.Now().Add(tokenTTL)
	return c.token, nil
}

func (c *Client) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

type detectRequest struct {
	Text        string `json:"text"`
	Sandbox     bool   `json:"sandbox"`
	Sensitivity int    `json:"sensitivity"`
}

type matchOffsets struct {
	Text struct {
		Chars struct {
			Starts  []int `json:"starts"`
			Lengths []int `json:"lengths"`
		} `json:"chars"`
	} `json:"text"`
}

type resultItem struct {
	Classification int            `json:"classification"`
	Matches        []matchOffsets `json:"matches"`
}

type detectResponse struct {
	Summary struct {
		AI float64 `json:"ai"`
	} `json:"summary"`
	Results []resultItem `json:"results"`
}

type detectResult struct {
	ai      float64
	flagged []string
}

func (c *Client) detect(ctx context.Context, token, scanID, text string) (detectResult, error) {
	body, err := json.Marshal(detectRequest{Text: text, Sandbox: c.sandbox, Sensitivity: sensitivity})
	if err != nil {
		return detectResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, detectBaseURL+scanID+"/check", bytes.NewReader(body))
	if err != nil {
		return detectResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return detectResult{}, fmt.Errorf("copyleaks detect: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return detectResult{}, fmt.Errorf("copyleaks detect read: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return detectResult{}, errUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return detectResult{}, fmt.Errorf("copyleaks detect returned %d: %s", resp.StatusCode, truncate(respBody))
	}

	var out detectResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return detectResult{}, fmt.Errorf("copyleaks detect decode: %w (body: %s)", err, truncate(respBody))
	}
	return detectResult{ai: out.Summary.AI, flagged: flaggedSegments(text, out.Results)}, nil
}

func flaggedSegments(text string, results []resultItem) []string {
	runes := []rune(text)
	var out []string
	for _, r := range results {
		if r.Classification != classAI {
			continue
		}
		for _, m := range r.Matches {
			starts := m.Text.Chars.Starts
			lengths := m.Text.Chars.Lengths
			for i := 0; i < len(starts) && i < len(lengths); i++ {
				s, l := starts[i], lengths[i]
				if s < 0 || l <= 0 || s+l > len(runes) {
					continue
				}
				seg := strings.TrimSpace(string(runes[s : s+l]))
				if seg == "" {
					continue
				}
				out = append(out, seg)
				if len(out) >= maxFlagged {
					return out
				}
			}
		}
	}
	return out
}

var scanCounter atomic.Int64

func nextScanID() string {
	return fmt.Sprintf("mas-%d-%d", time.Now().UnixNano(), scanCounter.Add(1))
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
