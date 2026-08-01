package telegram

import (
	"strings"
	"testing"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"1500", 1500, false},
		{"1500.50", 1500.50, false},
		{"1500,50", 1500.50, false},
		{" 1500 ", 1500, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-100", 0, true},
		{"0", 0, true},
	}
	for _, c := range cases {
		got, err := parseAmount(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseAmount(%q) = %v, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAmount(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	if got := formatAmount(1500); got != "1500" {
		t.Errorf("formatAmount(1500) = %q, want %q", got, "1500")
	}
	if got := formatAmount(1500.5); got != "1500.50" {
		t.Errorf("formatAmount(1500.5) = %q, want %q", got, "1500.50")
	}
}

func TestFormatCard(t *testing.T) {
	got := formatCard("4149500021111037")
	want := "4149 5000 2111 1037"
	if got != want {
		t.Errorf("formatCard() = %q, want %q", got, want)
	}
}

func TestBuildConsultText(t *testing.T) {
	got := buildConsultText("Михайленко Наталія", "29.07.2026", "15:00", 800)

	for _, want := range []string{
		"Михайленко Наталія",
		"на <b>15:00 29.07.2026</b> є погодженням",
		"<b>Вартість консультації складає 800 (вісімсот) грн.</b>",
		"розмірі <b>800 грн</b>",
		"ст. 207 Цивільного кодексу України",
		`<a href="https://abalis.com.ua/publichnyij-dogovor-oferta/">оферта</a>`,
		"<b>Оплата здійснюється не пізніше ніж за 2 години до початку консультації або протягом 30 хвилин після її завершення.</b>",
		"стягнення заборгованих коштів у судовому порядку",
		"З повагою, ТОВ «Абаліс».",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildConsultText() missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Підтвердженням згоди") {
		t.Errorf("buildConsultText() should no longer include the closing confirmation paragraph, got:\n%s", got)
	}
}

func TestBuildConsultText_EscapesHTML(t *testing.T) {
	got := buildConsultText(`Іванов & <sons>`, "1.1.2027", "10:00", 500)
	if strings.Contains(got, "<sons>") {
		t.Errorf("expected HTML-special characters in name to be escaped, got:\n%s", got)
	}
}
