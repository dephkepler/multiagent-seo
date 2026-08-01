package telegram

import "strings"

var ones = [...]string{"", "одна", "дві", "три", "чотири", "п'ять", "шість", "сім", "вісім", "дев'ять"}
var teens = [...]string{"десять", "одинадцять", "дванадцять", "тринадцять", "чотирнадцять", "п'ятнадцять", "шістнадцять", "сімнадцять", "вісімнадцять", "дев'ятнадцять"}
var tens = [...]string{"", "", "двадцять", "тридцять", "сорок", "п'ятдесят", "шістдесят", "сімдесят", "вісімдесят", "дев'яносто"}
var hundreds = [...]string{"", "сто", "двісті", "триста", "чотириста", "п'ятсот", "шістсот", "сімсот", "вісімсот", "дев'ятсот"}

// ukrainianNumberWords spells out a non-negative integer in Ukrainian, e.g.
// 800 -> "вісімсот". It's built for round consultation/invoice amounts
// (hundreds/thousands), not as a general-purpose number-to-text library —
// "one"/"two" use the feminine form (одна/дві) to agree with "гривня".
func ukrainianNumberWords(n int64) string {
	if n == 0 {
		return "нуль"
	}
	if n < 0 {
		return "мінус " + ukrainianNumberWords(-n)
	}

	var parts []string
	if n >= 1_000_000 {
		millions := n / 1_000_000
		parts = append(parts, ukrainianNumberWords(millions), pluralForm(millions, "мільйон", "мільйони", "мільйонів"))
		n %= 1_000_000
	}
	if n >= 1000 {
		thousands := n / 1000
		parts = append(parts, threeDigitsWords(thousands), pluralForm(thousands, "тисяча", "тисячі", "тисяч"))
		n %= 1000
	}
	if n > 0 || len(parts) == 0 {
		parts = append(parts, threeDigitsWords(n))
	}

	nonEmpty := parts[:0]
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " ")
}

// threeDigitsWords spells 0-999. "One"/"two" always come out feminine
// (одна/дві), matching both "гривня" and "тисяча" — the two nouns this gets
// used with.
func threeDigitsWords(n int64) string {
	h := n / 100
	rest := n % 100
	out := ""
	if h > 0 {
		out += hundreds[h]
	}
	if rest == 0 {
		return out
	}
	if out != "" {
		out += " "
	}
	if rest < 10 {
		out += ones[rest]
	} else if rest < 20 {
		out += teens[rest-10]
	} else {
		t := rest / 10
		o := rest % 10
		out += tens[t]
		if o > 0 {
			out += " " + ones[o]
		}
	}
	return out
}

// pluralForm picks the Ukrainian plural ending for n (1 / 2-4 / 5+, with the
// usual 11-14 exception), e.g. pluralForm(2, "тисяча", "тисячі", "тисяч") -> "тисячі".
func pluralForm(n int64, one, few, many string) string {
	n = n % 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
