package webleads_test

import (
	"testing"
	"time"

	"multiagent-seo/internal/domain/webleads"
)

func TestParse_UkrainianPlainText(t *testing.T) {
	body := `Ім'я: Анна

Номер телефону: 0976004626

Коментар: ФК «ПОЗИКА» подала позов по договору КУМО від 21.01.2022. Номер справи є, але матеріалів у E-Court немає.

Сторінка (ID): Головна (236)`

	got := webleads.Parse("<msg-1@abalis.com.ua>", "info@abalis.com.ua", "Нова заявка", body, time.Now())

	if got.Name != "Анна" {
		t.Errorf("Name = %q, want %q", got.Name, "Анна")
	}
	if got.Phone != "+380976004626" {
		t.Errorf("Phone = %q, want %q (normalized to E.164)", got.Phone, "+380976004626")
	}
	if got.Page != "Головна (236)" {
		t.Errorf("Page = %q, want %q", got.Page, "Головна (236)")
	}
	wantMessage := `ФК «ПОЗИКА» подала позов по договору КУМО від 21.01.2022. Номер справи є, але матеріалів у E-Court немає.`
	if got.Message != wantMessage {
		t.Errorf("Message = %q, want %q", got.Message, wantMessage)
	}
}

func TestParse_RussianHTML(t *testing.T) {
	body := `
<p><b>Имя:</b> Davyd</p>
<p><b>Номер телефона:</b> +491709007231</p>

<p><b>Коментар:</b> тест </p>
<p><b>Страница (ID):</b> Адвокаты по кредитам и микрозаймам (МФО) (887)</p>
`

	got := webleads.Parse("<msg-2@abalis.com.ua>", "info@abalis.com.ua", "Запрос консультации", body, time.Now())

	if got.Name != "Davyd" {
		t.Errorf("Name = %q, want %q", got.Name, "Davyd")
	}
	if got.Phone != "+491709007231" {
		t.Errorf("Phone = %q, want %q", got.Phone, "+491709007231")
	}
	if got.Message != "тест" {
		t.Errorf("Message = %q, want %q", got.Message, "тест")
	}
	if got.Page != "Адвокаты по кредитам и микрозаймам (МФО) (887)" {
		t.Errorf("Page = %q, want %q", got.Page, "Адвокаты по кредитам и микрозаймам (МФО) (887)")
	}
}

func TestParse_UnknownTemplateFallsBackToWholeText(t *testing.T) {
	body := "<p>Просто текст без меток, обычная переписка.</p>"

	got := webleads.Parse("<msg-3@abalis.com.ua>", "someone@example.com", "Re: вопрос", body, time.Now())

	if got.Name != "" || got.Phone != "" || got.Page != "" {
		t.Errorf("expected no fields matched, got Name=%q Phone=%q Page=%q", got.Name, got.Phone, got.Page)
	}
	if got.Message != "Просто текст без меток, обычная переписка." {
		t.Errorf("Message = %q, want the whole cleaned text", got.Message)
	}
}
