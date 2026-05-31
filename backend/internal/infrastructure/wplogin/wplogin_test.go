package wplogin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"multiagent-seo/internal/domain/linkbuilding"
)

const (
	basicForm = `<form id="loginform">
<input type="text" name="log"><input type="password" name="pwd">
<input type="submit" name="wp-submit"></form>`

	mathForm = `<form id="loginform">
<label>Please enter an answer in digits: 5 × 4 =</label>
<input type="text" name="log"><input type="password" name="pwd">
<input type="text" name="math-answer">
<input type="submit" name="wp-submit"></form>`

	recaptchaForm = `<form id="loginform">
<input type="text" name="log"><input type="password" name="pwd">
<div class="g-recaptcha" data-sitekey="abc123"></div>
<input type="submit" name="wp-submit"></form>`

	hiddenForm = `<form id="loginform">
<input type="text" name="log"><input type="password" name="pwd">
<input type="hidden" name="wp-login-nonce" value="n0nce">
<input type="submit" name="wp-submit"></form>`

	wordedMathForm = `<form id="loginform">
<label>Please solve: ten minus six =</label>
<input type="text" name="log"><input type="password" name="pwd">
<input type="text" name="captcha">
<input type="submit" name="wp-submit"></form>`

	// An extra challenge field with no solvable math (e.g. a worded riddle).
	unknownChallengeForm = `<form id="loginform">
<label>Security question: capital of France?</label>
<input type="text" name="log"><input type="password" name="pwd">
<input type="text" name="security-answer">
<input type="submit" name="wp-submit"></form>`
)

// loginMux serves the login form on GET and, on POST, sets the logged-in cookie
// + redirects to wp-admin only when accept(form) is true.
func loginMux(loginHTML string, accept func(url.Values) bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/wp-login.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, loginHTML)
			return
		}
		_ = r.ParseForm()
		if accept(r.PostForm) {
			http.SetCookie(w, &http.Cookie{Name: "wordpress_logged_in_abc", Value: "s", Path: "/"})
			http.Redirect(w, r, "/wp-admin/", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, loginHTML)
	})
	mux.HandleFunc("/wp-admin/", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "Dashboard") })
	return mux
}

func wpServer(t *testing.T, loginHTML string, accept func(url.Values) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(loginMux(loginHTML, accept))
}

func validCreds(f url.Values) bool {
	return f.Get("log") == "monamedia" && f.Get("pwd") == "MonaM@123"
}

func cred(base string) linkbuilding.SiteCredential {
	return linkbuilding.SiteCredential{Row: 7, BaseURL: base, Login: "monamedia", Password: "MonaM@123"}
}

func TestLoginSuccess(t *testing.T) {
	srv := wpServer(t, basicForm, validCreds)
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK || res.Status != "login ok" {
		t.Fatalf("got %+v, want ok", res)
	}
	if res.Row != 7 {
		t.Errorf("row = %d, want 7", res.Row)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	srv := wpServer(t, basicForm, func(url.Values) bool { return false })
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OK || res.Status != "login failed" {
		t.Errorf("got %+v, want login failed", res)
	}
}

func TestLoginSolvesMathCaptcha(t *testing.T) {
	srv := wpServer(t, mathForm, func(f url.Values) bool {
		return validCreds(f) && f.Get("math-answer") == "20"
	})
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK || res.Status != "login ok (captcha solved)" {
		t.Fatalf("math captcha should be solved, got %+v", res)
	}
}

func TestLoginUnsolvableChallenge(t *testing.T) {
	// Server requires the security answer we can't compute, so login fails and
	// the verdict must flag an unsolved challenge rather than a plain failure.
	srv := wpServer(t, unknownChallengeForm, func(f url.Values) bool {
		return validCreds(f) && f.Get("security-answer") != ""
	})
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OK || res.Status != "captcha (unsolved)" {
		t.Errorf("got %+v, want captcha (unsolved)", res)
	}
}

func TestLoginRealCaptchaIsManual(t *testing.T) {
	srv := wpServer(t, recaptchaForm, func(url.Values) bool {
		t.Error("must not POST credentials when a real CAPTCHA is present")
		return false
	})
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OK || res.Status != "captcha (manual: recaptcha)" {
		t.Errorf("got %+v, want captcha (manual: recaptcha)", res)
	}
}

func TestLoginCarriesHiddenFields(t *testing.T) {
	srv := wpServer(t, hiddenForm, func(f url.Values) bool {
		return validCreds(f) && f.Get("wp-login-nonce") == "n0nce"
	})
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK {
		t.Errorf("hidden nonce must be carried forward, got %+v", res)
	}
}

func TestLoginEndpointMissing(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OK || res.Status != "wp-login.php not found (404)" {
		t.Errorf("got %+v, want not-found verdict", res)
	}
}

func TestLoginUnreachable(t *testing.T) {
	srv := wpServer(t, basicForm, validCreds)
	base := srv.URL
	srv.Close() // now refuses connections

	res, err := New(nil).Login(context.Background(), cred(base))
	if err != nil {
		t.Fatalf("network failure must not be an error, got: %v", err)
	}
	if res.OK || res.Status != "unreachable" {
		t.Errorf("got %+v, want unreachable verdict", res)
	}
}

func TestLoginContextCancelledReturnsError(t *testing.T) {
	srv := wpServer(t, basicForm, validCreds)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := New(nil).Login(ctx, cred(srv.URL))
	if err == nil {
		t.Fatal("cancelled context must return an error so the run aborts")
	}
	if res.OK {
		t.Errorf("must not record a success verdict on cancellation: %+v", res)
	}
}

func TestLoginCustomPhpPath(t *testing.T) {
	// Login lives at a renamed script; only that path is served (no wp-login.php).
	mux := http.NewServeMux()
	mux.HandleFunc("/secret-login.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, basicForm)
			return
		}
		_ = r.ParseForm()
		if validCreds(r.PostForm) {
			http.SetCookie(w, &http.Cookie{Name: "wordpress_logged_in_abc", Value: "s", Path: "/"})
			http.Redirect(w, r, "/wp-admin/", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, basicForm)
	})
	mux.HandleFunc("/wp-admin/", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "Dashboard") })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := linkbuilding.SiteCredential{Row: 1, BaseURL: srv.URL + "/secret-login.php", Login: "monamedia", Password: "MonaM@123"}
	res, err := New(nil).Login(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK {
		t.Errorf("custom .php login path must be used as-is, got %+v", res)
	}
}

func TestLoginError(t *testing.T) {
	html := `<div id="login_error"><strong>Error:</strong> The password you entered is incorrect.</div>`
	if got := loginError(strings.NewReader(html)); got != "Error: The password you entered is incorrect." {
		t.Errorf("loginError = %q", got)
	}
	if got := loginError(strings.NewReader("<html><body>no error here</body></html>")); got != "" {
		t.Errorf("no #login_error should be empty, got %q", got)
	}
}

func TestLoginOverSelfSignedTLS(t *testing.T) {
	// httptest's TLS server uses an untrusted self-signed cert; login must still
	// succeed because the adapter skips certificate verification.
	srv := httptest.NewTLSServer(loginMux(basicForm, validCreds))
	defer srv.Close()

	res, err := New(nil).Login(context.Background(), cred(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK {
		t.Errorf("self-signed TLS site must still log in, got %+v", res)
	}
}

func TestLoginInvalidURL(t *testing.T) {
	res, err := New(nil).Login(context.Background(), cred("not a url"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OK || res.Status != "invalid site URL" {
		t.Errorf("got %+v, want invalid-URL verdict", res)
	}
}

func TestSolveMath(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"Please enter an answer in digits: 5 × 4 =", 20, true},
		{"3 + 4 =", 7, true},
		{"9 - 2 =", 7, true},
		{"6 x 7 =", 42, true},
		{"2 * 8 =", 16, true},
		{"8 ÷ 2 =", 4, true},
		{"9 / 3 =", 3, true},
		{"5 / 0 =", 0, false}, // division by zero is not solvable
		{"no math here", 0, false},
	}
	for _, c := range cases {
		got, ok := solveMath(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("solveMath(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSolveWordedMath(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"ten minus six =", 4},
		{"five plus four =", 9},
		{"three times two =", 6},
		{"10 - six =", 4},        // mixed digit/word
		{"twenty minus 1 =", 19},     // tens word + digit
		{"twelve divided by 4 =", 3}, // worded division
		{"5 × 4 =", 20},              // pure digits still work
	}
	for _, c := range cases {
		got, ok := solveMath(normalizeWords(c.in))
		if !ok || got != c.want {
			t.Errorf("solve(normalize(%q)) = (%d,%v), want %d", c.in, got, ok, c.want)
		}
	}
}

func TestParseLoginForm(t *testing.T) {
	math, _ := parseLoginForm(strings.NewReader(mathForm))
	if !math.hasMath || math.mathAnswer != 20 || math.answerField != "math-answer" {
		t.Errorf("math parse = %+v, want hasMath/20/math-answer", math)
	}
	if math.recaptcha {
		t.Error("math form must not be flagged as a real captcha")
	}

	worded, _ := parseLoginForm(strings.NewReader(wordedMathForm))
	if !worded.hasMath || worded.mathAnswer != 4 || worded.answerField != "captcha" {
		t.Errorf("worded parse = %+v, want hasMath/4/captcha", worded)
	}

	rc, _ := parseLoginForm(strings.NewReader(recaptchaForm))
	if !rc.recaptcha {
		t.Error("recaptcha form must be flagged")
	}

	hf, _ := parseLoginForm(strings.NewReader(hiddenForm))
	if hf.hidden["wp-login-nonce"] != "n0nce" {
		t.Errorf("hidden fields = %+v, want wp-login-nonce=n0nce", hf.hidden)
	}
}
