package wplogin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"multiagent-seo/pkg/httpx"
)

type Authenticator struct {
	timeout   time.Duration
	log       *slog.Logger
	transport http.RoundTripper
}

func New(log *slog.Logger, timeout time.Duration) *Authenticator {
	if log == nil {
		log = slog.Default()
	}
	return &Authenticator{
		timeout: timeout,
		log:     log,
		transport: httpx.NewTransport(
			httpx.BlockPrivateIPs(),
			httpx.InsecureTLS(),
		),
	}
}

func (a *Authenticator) fetchLoginForm(ctx context.Context, client *http.Client, loginURL string) (loginForm, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return loginForm{}, 0, err
	}
	setBrowserHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return loginForm{}, 0, err
	}
	defer resp.Body.Close()

	form, perr := parseLoginForm(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	return form, resp.StatusCode, perr
}

// setBrowserHeaders makes the request look like a real Chrome navigation so that
// passive WAF/Cloudflare checks that only fingerprint headers let it through. It
// does not defeat active JS challenges or CAPTCHAs.
func setBrowserHeaders(req *http.Request) {
	h := req.Header
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Upgrade-Insecure-Requests", "1")
	h.Set("Sec-CH-UA", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	h.Set("Sec-CH-UA-Mobile", "?0")
	h.Set("Sec-CH-UA-Platform", `"Windows"`)
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Sec-Fetch-User", "?1")
}

func hasLoggedInCookie(jar http.CookieJar, u *url.URL) bool {
	for _, c := range jar.Cookies(u) {
		if strings.HasPrefix(c.Name, "wordpress_logged_in_") {
			return true
		}
	}
	return false
}
