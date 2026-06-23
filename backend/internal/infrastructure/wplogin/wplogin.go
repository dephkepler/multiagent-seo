package wplogin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"multiagent-seo/internal/domain/linkbuilding"
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

func (a *Authenticator) Login(ctx context.Context, cred linkbuilding.SiteCredential) (linkbuilding.LoginResult, error) {
	res := linkbuilding.LoginResult{Row: cred.Row, BaseURL: cred.BaseURL}

	base := strings.TrimRight(strings.TrimSpace(cred.BaseURL), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		res.Status = "invalid site URL"
		return res, nil
	}
	origin, err := url.Parse(u.Scheme + "://" + u.Host)
	if err != nil {
		res.Status = "invalid site URL"
		return res, nil
	}

	loginURL := origin.String() + "/wp-login.php"
	if strings.Trim(u.Path, "/") != "" {
		loginURL = base
	}
	adminURL := origin.String() + "/wp-admin/"

	jar, err := cookiejar.New(nil)
	if err != nil {
		return res, fmt.Errorf("wplogin cookiejar: %w", err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "wordpress_test_cookie", Value: "WP Cookie check"}})
	client := &http.Client{Jar: jar, Timeout: a.timeout, Transport: a.transport}

	form, status, err := a.fetchLoginForm(ctx, client, loginURL)
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		a.log.DebugContext(ctx, "wplogin unreachable", "row", cred.Row, "err", err)
		res.Status = "unreachable"
		return res, nil
	}
	if status == http.StatusNotFound {
		res.Status = "wp-login.php not found (404)"
		a.log.DebugContext(ctx, "login failed", "row", cred.Row, "reason", "wp-login.php returns 404 — hidden/renamed login path or not WordPress; put the full login URL in the sheet")
		return res, nil
	}
	if form.recaptcha {
		res.Status = "captcha (manual: recaptcha)"
		a.log.DebugContext(ctx, "captcha not bypassed", "row", cred.Row, "reason", "reCAPTCHA/hCaptcha needs a browser")
		return res, nil
	}

	unsolvedChallenge := !form.hasMath && form.answerField != ""
	if unsolvedChallenge {
		a.log.DebugContext(ctx, "captcha not bypassed", "row", cred.Row, "reason", "unrecognized challenge field", "field", form.answerField)
	}

	values := url.Values{}
	for k, v := range form.hidden {
		values.Set(k, v)
	}
	values.Set("log", cred.Login)
	values.Set("pwd", cred.Password)
	values.Set("wp-submit", "Log In")
	values.Set("redirect_to", adminURL)
	values.Set("testcookie", "1")
	if form.hasMath && form.answerField != "" {
		values.Set(form.answerField, strconv.Itoa(form.mathAnswer))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(values.Encode()))
	if err != nil {
		a.log.DebugContext(ctx, "wplogin build login request failed", "row", cred.Row, "err", err)
		res.Status = "request build failed"
		return res, nil
	}
	setBrowserHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin.String())
	req.Header.Set("Referer", loginURL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		a.log.DebugContext(ctx, "wplogin unreachable", "row", cred.Row, "err", err)
		res.Status = "unreachable"
		return res, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		a.log.DebugContext(ctx, "wplogin read login response failed", "row", cred.Row, "err", err)
		res.Status = "unreachable"
		return res, nil
	}

	switch {
	case hasLoggedInCookie(jar, origin):
		res.OK = true
		if form.hasMath {
			res.Status = "login ok (captcha solved)"
			a.log.InfoContext(ctx, "captcha solved", "row", cred.Row, "answer", form.mathAnswer)
		} else {
			res.Status = "login ok"
		}
	case resp.StatusCode == http.StatusNotFound:
		res.Status = "wp-login.php not found (404)"
		a.log.DebugContext(ctx, "login failed", "row", cred.Row, "reason", "wp-login.php returns 404 — hidden/renamed login path or not WordPress")
	case form.hasMath:
		res.Status = "login failed (captcha)"
		a.log.DebugContext(ctx, "login failed", "row", cred.Row, "reason", "math captcha answer rejected or bad credentials")
	case unsolvedChallenge:
		res.Status = "captcha (unsolved)"
		a.log.DebugContext(ctx, "login failed", "row", cred.Row, "reason", "unsolved challenge field "+form.answerField)
	default:
		res.Status = "login failed"
		reason := loginError(bytes.NewReader(body))
		if reason == "" {
			reason = "credentials rejected (no error message returned)"
		}
		a.log.DebugContext(ctx, "login failed", "row", cred.Row, "reason", reason)
	}
	return res, nil
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
