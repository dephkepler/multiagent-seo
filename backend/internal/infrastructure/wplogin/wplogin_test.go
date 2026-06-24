package wplogin

import (
	"strings"
	"testing"
)

const (
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
)

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
		{"5 / 0 =", 0, false},
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
		{"10 - six =", 4},
		{"twenty minus 1 =", 19},
		{"twelve divided by 4 =", 3},
		{"5 × 4 =", 20},
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
