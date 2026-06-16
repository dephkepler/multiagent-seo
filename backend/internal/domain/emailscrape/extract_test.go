package emailscrape

import (
	"slices"
	"testing"
)

func TestExtractEmails(t *testing.T) {
	cases := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "plain text and mailto, deduped and lowercased",
			html: `Contact <a href="mailto:Info@Acme.com">us</a> or write to info@acme.com or sales@acme.com.`,
			want: []string{"info@acme.com", "sales@acme.com"},
		},
		{
			name: "image false positive dropped",
			html: `<img src="logo@2x.png"> real@site.com`,
			want: []string{"real@site.com"},
		},
		{
			name: "placeholder and vendor junk dropped",
			html: `foo@example.com bar@sentry.io baz@wixpress.com good@realbiz.de`,
			want: []string{"good@realbiz.de"},
		},
		{
			name: "trailing punctuation trimmed",
			html: `Email: hello@firm.io.`,
			want: []string{"hello@firm.io"},
		},
		{
			name: "none",
			html: `<p>no contacts here</p>`,
			want: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractEmails(c.html)
			if !slices.Equal(got, c.want) {
				t.Errorf("ExtractEmails() = %v, want %v", got, c.want)
			}
		})
	}
}
