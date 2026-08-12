package modx

import "strings"

var translitTable = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'ґ': "g", 'д': "d",
	'е': "e", 'є': "ie", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'і': "i", 'ї': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh",
	'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "iu", 'я': "ia",
}

const maxAliasLen = 80

func slugify(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		out, known := translitTable[r]
		if !known {
			out = "-"
		}
		for _, c := range out {
			if c == '-' {
				if !prevDash && b.Len() > 0 {
					b.WriteByte('-')
					prevDash = true
				}
				continue
			}
			b.WriteRune(c)
			prevDash = false
		}
	}

	result := strings.Trim(b.String(), "-")
	if len(result) > maxAliasLen {
		result = strings.TrimRight(result[:maxAliasLen], "-")
	}
	return result
}
