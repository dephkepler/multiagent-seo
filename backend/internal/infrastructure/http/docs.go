package http

import (
	"encoding/json"
	"net/http"
	"sync"

	"multiagent-seo/internal/oapigen"
)

// The spec is static for the process lifetime, so marshal it once.
var (
	specOnce  sync.Once
	specBytes []byte
	specErr   error
)

func openAPISpecJSON() ([]byte, error) {
	specOnce.Do(func() {
		swagger, err := oapigen.GetSwagger()
		if err != nil {
			specErr = err
			return
		}
		specBytes, specErr = json.Marshal(swagger)
	})
	return specBytes, specErr
}

func handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	b, err := openAPISpecJSON()
	if err != nil {
		http.Error(w, "openapi spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(b)
}

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func handleLandingPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(landingPageHTML))
}

// swaggerUIHTML points at the same-origin /openapi.json so "Try it out"
// requests target whichever host opened /docs.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ContentFlow API — Swagger UI</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      // Keep the Authorize token in localStorage so a page reload doesn't drop it.
      persistAuthorization: true,
    });
  </script>
</body>
</html>`

const landingPageHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ContentFlow API</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0; min-height: 100vh; padding: 40px 20px;
      font: 16px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #f6f8fa; color: #1f2328; display: flex; justify-content: center;
    }
    .wrap { max-width: 720px; width: 100%; }
    h1 { margin: 0 0 8px; font-size: 32px; }
    .sub { color: #59636e; margin: 0 0 24px; }
    .status {
      display: inline-flex; align-items: center; gap: 8px;
      padding: 6px 12px; border-radius: 999px;
      background: #dafbe1; color: #1a7f37; font-weight: 600; font-size: 14px;
      margin-bottom: 24px;
    }
    .status::before {
      content: ""; width: 8px; height: 8px; border-radius: 50%;
      background: #1a7f37; box-shadow: 0 0 0 4px rgba(26,127,55,0.15);
    }
    .cta {
      display: inline-block; padding: 12px 24px; margin: 4px 8px 4px 0;
      background: #0969da; color: white; text-decoration: none;
      border-radius: 6px; font-weight: 600;
    }
    .cta:hover { background: #0860c7; }
    .cta.secondary { background: white; color: #1f2328; border: 1px solid #d1d9e0; }
    .cta.secondary:hover { background: #f6f8fa; }
    .card {
      background: white; border: 1px solid #d1d9e0; border-radius: 8px;
      padding: 20px 24px; margin-top: 24px;
    }
    .card h2 { margin: 0 0 12px; font-size: 18px; }
    ul.endpoints { list-style: none; padding: 0; margin: 0; }
    ul.endpoints li { padding: 10px 0; border-bottom: 1px solid #eaeef2; }
    ul.endpoints li:last-child { border-bottom: none; }
    code {
      background: #eaeef2; padding: 2px 6px; border-radius: 4px;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px;
    }
    .method { font-weight: 700; padding: 2px 6px; border-radius: 4px; color: white; font-size: 12px; margin-right: 6px; }
    .method.get { background: #1f883d; }
    .method.post { background: #0969da; }
    .hint { color: #59636e; font-size: 14px; margin: 6px 0 0; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="status">сервис работает</div>
    <h1>ContentFlow API</h1>
    <p class="sub">Генератор SEO-статей: ключевое слово → данные конкурентов → LLM → черновик в WordPress.</p>

    <a class="cta" href="/docs">Открыть Swagger UI →</a>
    <a class="cta secondary" href="/openapi.json">openapi.json</a>

    <div class="card">
      <h2>Что здесь есть</h2>
      <ul class="endpoints">
        <li>
          <span class="method post">POST</span><code>/auth/login</code>
          <div class="hint">Получить JWT. Остальные ручки требуют заголовок <code>Authorization: Bearer &lt;token&gt;</code>.</div>
        </li>
        <li>
          <span class="method post">POST</span><code>/generate</code>
          <div class="hint">Поставить ключевое слово в очередь. Возвращает <code>id</code> за 1 секунду — генерация идёт в фоне (30 сек – 5 мин).</div>
        </li>
        <li>
          <span class="method get">GET</span><code>/articles/{id}</code>
          <div class="hint">Узнать статус статьи. Поллите этот эндпоинт после <code>/generate</code>, пока <code>status</code> не сменится с <code>generating</code> на <code>draft</code> (готово) или <code>failed</code>.</div>
        </li>
        <li>
          <span class="method get">GET</span><code>/articles</code>
          <div class="hint">Список всех сгенерированных статей.</div>
        </li>
        <li>
          <span class="method post">POST</span><code>/articles/{id}/publish</code>
          <div class="hint">Опубликовать готовый черновик на сайт WordPress.</div>
        </li>
      </ul>
    </div>
  </div>
</body>
</html>`
