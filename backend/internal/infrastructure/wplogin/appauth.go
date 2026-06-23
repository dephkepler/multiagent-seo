package wplogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"multiagent-seo/pkg/httpx"
)

var nonceRe = regexp.MustCompile(`wpApiSettings\s*=\s*\{[^}]*"nonce":"([a-zA-Z0-9]+)"`)

func (a *Authenticator) IssueAppPassword(ctx context.Context, donorURL, login, password string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(donorURL), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid donor URL %q", donorURL)
	}
	origin, _ := url.Parse(u.Scheme + "://" + u.Host)

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("cookiejar: %w", err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "wordpress_test_cookie", Value: "WP Cookie check"}})
	client := &http.Client{Jar: jar, Timeout: a.timeout, Transport: a.transport}

	loginURL := origin.String() + "/wp-login.php"
	if strings.Trim(u.Path, "/") != "" {
		loginURL = base
	}

	form, status, err := a.fetchLoginForm(ctx, client, loginURL)
	if err != nil {
		return "", fmt.Errorf("fetch login form: %w", err)
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("wp-login.php returns 404 (hidden/renamed login or not WordPress)")
	}
	if form.recaptcha {
		return "", fmt.Errorf("real CAPTCHA on login — cannot bypass")
	}

	values := url.Values{}
	for k, v := range form.hidden {
		values.Set(k, v)
	}
	values.Set("log", login)
	values.Set("pwd", password)
	values.Set("wp-submit", "Log In")
	values.Set("redirect_to", origin.String()+"/wp-admin/")
	values.Set("testcookie", "1")
	if form.hasMath && form.answerField != "" {
		values.Set(form.answerField, strconv.Itoa(form.mathAnswer))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("login post: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	_ = resp.Body.Close()

	if !hasLoggedInCookie(jar, origin) {
		return "", fmt.Errorf("login failed (status %d): %s", resp.StatusCode, loginDiag(body))
	}

	nonce, err := fetchNonce(ctx, client, origin.String()+"/wp-admin/profile.php")
	if err != nil {
		return "", fmt.Errorf("fetch nonce: %w", err)
	}

	appPwd, err := createAppPassword(ctx, client, origin.String(), nonce, "multiagent-seo-backlinks")
	if err != nil {
		return "", fmt.Errorf("create app password: %w", err)
	}
	return appPwd, nil
}

func fetchNonce(ctx context.Context, client *http.Client, profileURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch nonce request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch nonce: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxPageBytes))
	if err != nil {
		return "", fmt.Errorf("read profile response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("profile status %d", resp.StatusCode)
	}
	m := nonceRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("wpApiSettings nonce not found in profile page")
	}
	return string(m[1]), nil
}

func createAppPassword(ctx context.Context, client *http.Client, origin, nonce, name string) (string, error) {
	endpoint := origin + "/wp-json/wp/v2/users/me/application-passwords"
	bodyJSON, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return "", fmt.Errorf("marshal app password request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return "", fmt.Errorf("create app password request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WP-Nonce", nonce)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create app password: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read app password response: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, snippet(body))
	}

	var out struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if out.Password == "" {
		return "", fmt.Errorf("empty password in response: %s", snippet(body))
	}
	return out.Password, nil
}

func loginDiag(body []byte) string {
	low := strings.ToLower(string(body))
	switch {
	case strings.Contains(low, "g-recaptcha") || strings.Contains(low, "recaptcha/api"):
		return "page shows reCAPTCHA — needs a real browser"
	case strings.Contains(low, "h-captcha") || strings.Contains(low, "hcaptcha"):
		return "page shows hCaptcha — needs a real browser"
	case strings.Contains(low, "cf-challenge") || strings.Contains(low, "challenge-platform") || strings.Contains(low, "checking your browser") || strings.Contains(low, "cloudflare"):
		return "Cloudflare / JS challenge — needs a real browser"
	case strings.Contains(low, "the password you entered") || strings.Contains(low, "incorrect") || strings.Contains(low, "is not registered") || strings.Contains(low, "invalid username"):
		return "WordPress rejected the credentials"
	case strings.Contains(low, "too many") || strings.Contains(low, "locked") || strings.Contains(low, "try again later") || strings.Contains(low, "limit"):
		return "rate-limited / login lockout"
	case strings.Contains(low, "two-factor") || strings.Contains(low, "2fa") || strings.Contains(low, "authentication code"):
		return "two-factor auth required"
	case len(body) == 0:
		return "empty response"
	default:
		return fmt.Sprintf("no known marker (%d bytes): %s", len(body), snippet(body))
	}
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
