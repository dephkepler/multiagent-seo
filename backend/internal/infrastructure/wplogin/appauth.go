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
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("login post: %w", err)
	}
	resp.Body.Close()

	if !hasLoggedInCookie(jar, origin) {
		return "", fmt.Errorf("login failed (no wordpress_logged_in cookie set)")
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
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
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
	bodyJSON, _ := json.Marshal(map[string]string{"name": name})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-WP-Nonce", nonce)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

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

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
