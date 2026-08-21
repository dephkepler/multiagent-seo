package telegram

import (
	"encoding/json"
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

// The keyboard is hand-rolled JSON because the library has no web_app type, so
// its shape is only ever checked here. Telegram rejects the whole sendMessage
// on a malformed reply_markup, which would leave a client with no answer at all.
func TestAppKeyboard(t *testing.T) {
	encoded, err := appKeyboard("https://app.example.com/")
	if err != nil {
		t.Fatalf("appKeyboard: %v", err)
	}

	var markup struct {
		Keyboard [][]struct {
			Text   string `json:"text"`
			WebApp *struct {
				URL string `json:"url"`
			} `json:"web_app"`
		} `json:"keyboard"`
		Resize bool `json:"resize_keyboard"`
	}
	if err := json.Unmarshal([]byte(encoded), &markup); err != nil {
		t.Fatalf("the markup is not valid JSON: %v", err)
	}

	if len(markup.Keyboard) != 2 {
		t.Fatalf("got %d rows, want 2", len(markup.Keyboard))
	}
	app := markup.Keyboard[0][0]
	if app.WebApp == nil || app.WebApp.URL != "https://app.example.com/" {
		t.Errorf("first button does not open the app: %+v", app)
	}
	// The chat path has to stay reachable, and its label is what handle()
	// matches to start the message flow.
	chat := markup.Keyboard[1][0]
	if chat.Text != requestButtonLabel {
		t.Errorf("second button = %q, want %q", chat.Text, requestButtonLabel)
	}
	if chat.WebApp != nil {
		t.Error("the chat button must not carry a web_app")
	}
	if !markup.Resize {
		t.Error("resize_keyboard is off, so the keyboard eats half the screen")
	}
}

// An http url makes Telegram reject the message the button rides on, so the
// button is dropped rather than the reply.
func TestUsableAppURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://app.example.com":     "https://app.example.com",
		"  https://app.example.com  ": "https://app.example.com",
		"http://app.example.com":      "",
		"app.example.com":             "",
		"":                            "",
	} {
		if got := usableAppURL(raw); got != want {
			t.Errorf("usableAppURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Renaming the button did not strand the clients who already had the old one on
// their screen.
func TestBothRequestButtonLabelsAreDistinct(t *testing.T) {
	if requestButtonLabel == legacyRequestButtonLabel {
		t.Fatal("the legacy label is meant to be the previous wording, not a copy")
	}
	if appButtonLabel == requestButtonLabel || appButtonLabel == legacyRequestButtonLabel {
		t.Error("the app button must not share a label with the chat button")
	}
	for _, label := range []string{requestButtonLabel, legacyRequestButtonLabel, appButtonLabel} {
		if _, taken := staffMenuCommands[label]; taken {
			t.Errorf("client label %q also maps to a staff command", label)
		}
	}
}
