package webleads_test

import (
	"testing"
	"time"

	"multiagent-seo/internal/domain/webleads"
)

func TestParse_PhoneNormalization(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"UA local, no country code", "0976004626", "+380976004626"},
		{"UA local, spaces and dashes", "096 700-46-26", "+380967004626"},
		{"already E.164, untouched", "+491709007231", "+491709007231"},
		{"has + and spaces", "+380 67 077 07 91", "+380670770791"},
		{"garbage, kept as-is", "позвоните мне", "позвоните мне"},
		{"empty, stays empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := "Телефон: " + c.raw
			got := webleads.Parse("<msg@abalis.com.ua>", "info@abalis.com.ua", "Заявка", body, time.Now())
			if got.Phone != c.want {
				t.Errorf("Phone = %q, want %q", got.Phone, c.want)
			}
		})
	}
}
