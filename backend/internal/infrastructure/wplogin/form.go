package wplogin

import (
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type loginForm struct {
	hidden      map[string]string
	answerField string
	mathAnswer  int
	hasMath     bool
	recaptcha   bool
}

var mathRe = regexp.MustCompile(`(\d+)\s*([+\-x×*/÷])\s*(\d+)\s*=`)

var numberWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	"thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
	"eighteen": 18, "nineteen": 19, "twenty": 20, "thirty": 30, "forty": 40,
	"fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90,
}

var wordOperators = strings.NewReplacer(
	"multiplied by", "*", "divided by", "/", "plus", "+", "minus", "-", "times", "*",
)

var numberWordRe = regexp.MustCompile(`\b(zero|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety)\b`)

var captchaMarkers = []string{"g-recaptcha", "grecaptcha", "h-captcha", "hcaptcha", "data-sitekey"}

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
