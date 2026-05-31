package wplogin

import (
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// loginForm holds what we extract from a GET of wp-login.php before posting:
// the form's hidden fields (carried forward so plugin nonces survive), a solved
// arithmetic captcha if one is present, and whether a real CAPTCHA blocks us.
type loginForm struct {
	hidden      map[string]string
	answerField string
	mathAnswer  int
	hasMath     bool
	recaptcha   bool
}

// mathRe matches a simple arithmetic captcha like "5 × 4 =". Operators cover the
// plugins seen in the wild: + - × (also x or *) and ÷ (also /).
var mathRe = regexp.MustCompile(`(\d+)\s*([+\-x×*/÷])\s*(\d+)\s*=`)

// numberWords maps spelled-out numbers (0–20 and the tens) to digits so worded
// or mixed captchas — "ten minus six =", "five plus four =", "10 - six =" — are
// solved by the same digit-based matcher after normalization.
var numberWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	"thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
	"eighteen": 18, "nineteen": 19, "twenty": 20, "thirty": 30, "forty": 40,
	"fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90,
}

// wordOperators turns worded operators into the symbols mathRe understands.
var wordOperators = strings.NewReplacer(
	"multiplied by", "*", "divided by", "/", "plus", "+", "minus", "-", "times", "*",
)

var numberWordRe = regexp.MustCompile(`\b(zero|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety)\b`)

// captchaMarkers flag a real (image/JS) CAPTCHA we cannot solve over HTTP.
var captchaMarkers = []string{"g-recaptcha", "grecaptcha", "h-captcha", "hcaptcha", "data-sitekey"}

// knownFields are the standard wp-login inputs; any other visible text input in
// the form is treated as the captcha answer box.
var knownFields = map[string]bool{
	"log": true, "pwd": true, "wp-submit": true,
	"rememberme": true, "redirect_to": true, "testcookie": true,
}

func parseLoginForm(body io.Reader) (loginForm, error) {
	root, err := html.Parse(body)
	if err != nil {
		return loginForm{}, err
	}

	lf := loginForm{hidden: map[string]string{}, recaptcha: hasCaptchaMarker(root)}

	form := findLoginForm(root)
	if form == nil {
		return lf, nil
	}

	var text strings.Builder
	collectForm(form, &lf, &text)

	if n, ok := solveMath(normalizeWords(text.String())); ok {
		lf.hasMath = true
		lf.mathAnswer = n
	}
	return lf, nil
}

// findLoginForm prefers <form id="loginform">, falling back to any form that
// contains the username input (name="log").
func findLoginForm(root *html.Node) *html.Node {
	var byID, byField *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			if byID == nil && attrVal(n, "id") == "loginform" {
				byID = n
			}
			if byField == nil && formContainsInput(n, "log") {
				byField = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if byID != nil {
		return byID
	}
	return byField
}

func collectForm(form *html.Node, lf *loginForm, text *strings.Builder) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			name := attrVal(n, "name")
			if name != "" {
				switch strings.ToLower(attrVal(n, "type")) {
				case "hidden":
					lf.hidden[name] = attrVal(n, "value")
				case "text", "number", "tel", "":
					if !knownFields[name] && lf.answerField == "" {
						lf.answerField = name
					}
				}
			}
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			text.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
}

// normalizeWords lowercases the text and rewrites worded numbers/operators into
// digits and symbols so solveMath handles digit, worded, and mixed captchas.
func normalizeWords(s string) string {
	s = wordOperators.Replace(strings.ToLower(s))
	return numberWordRe.ReplaceAllStringFunc(s, func(w string) string {
		return strconv.Itoa(numberWords[w])
	})
}

func solveMath(s string) (int, bool) {
	m := mathRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[3])
	switch m[2] {
	case "+":
		return a + b, true
	case "-":
		return a - b, true
	case "x", "×", "*":
		return a * b, true
	case "/", "÷":
		if b == 0 {
			return 0, false
		}
		return a / b, true
	}
	return 0, false
}

func hasCaptchaMarker(root *html.Node) bool {
	found := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				v := strings.ToLower(a.Val)
				for _, m := range captchaMarkers {
					if strings.Contains(v, m) {
						found = true
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

func formContainsInput(form *html.Node, name string) bool {
	found := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode && n.Data == "input" && attrVal(n, "name") == name {
			found = true
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
	return found
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// loginError extracts WordPress's failure message from a POST response — the
// text of <div id="login_error"> — so a "login failed" carries the real reason
// (wrong password, unknown user, too many attempts…). Empty if none is present.
func loginError(body io.Reader) string {
	root, err := html.Parse(body)
	if err != nil {
		return ""
	}
	node := elementByID(root, "login_error")
	if node == nil {
		return ""
	}
	return strings.Join(strings.Fields(textContent(node)), " ")
}

func elementByID(root *html.Node, id string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && attrVal(n, "id") == id {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
