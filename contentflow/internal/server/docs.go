package server

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

// swaggerUIHTML is a self-contained Swagger UI page loaded from a public CDN.
// It points at /openapi.yaml served by this same process, so "Try it out"
// requests go to whichever host the user opened /docs on.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Contentflow API — Swagger UI</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
    });
  </script>
</body>
</html>`

// landingPageHTML is shown at the root URL. It's deliberately a small,
// self-contained "this thing is alive" page that points testers at /docs
// without needing them to know about Swagger UI in advance.
const landingPageHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Contentflow API</title>
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
    <h1>Contentflow API</h1>
    <p class="sub">Генератор SEO-статей: ключевое слово → данные конкурентов → LLM → черновик в WordPress.</p>

    <a class="cta" href="/docs">Открыть Swagger UI →</a>
    <a class="cta secondary" href="/openapi.yaml">openapi.yaml</a>

    <div class="card">
      <h2>Что здесь есть</h2>
      <ul class="endpoints">
        <li>
          <span class="method post">POST</span><code>/generate</code>
          <div class="hint">Сгенерировать статью по ключевому слову. Долгий запрос (30–90 секунд). Главный эндпоинт для тестирования.</div>
        </li>
        <li>
          <span class="method get">GET</span><code>/articles</code>
          <div class="hint">Список всех сгенерированных статей в виде JSON. В браузере выглядит как сырой текст — это нормально, эндпоинт для программ. Чтобы посмотреть глазами — пользуйся Swagger UI.</div>
        </li>
        <li>
          <span class="method post">POST</span><code>/articles/{id}/publish</code>
          <div class="hint">Опубликовать ранее созданный черновик из WordPress на сайт.</div>
        </li>
      </ul>
    </div>

    <div class="card">
      <h2>Как протестировать за 30 секунд</h2>
      <ol style="margin: 0; padding-left: 20px;">
        <li>Нажми <strong>«Открыть Swagger UI»</strong> выше.</li>
        <li>Раскрой блок <code>POST /generate</code>.</li>
        <li>Нажми <strong>Try it out</strong>, в Examples выбери <code>cheap_test</code> (самый дешёвый прогон).</li>
        <li>Нажми <strong>Execute</strong>, подожди ~15 секунд.</li>
        <li>В ответе будет ссылка <code>wp_edit_url</code> — это черновик в админке WordPress.</li>
      </ol>
    </div>
  </div>
</body>
</html>`

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(openAPISpec)
}

func (s *Server) handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func (s *Server) handleLandingPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(landingPageHTML))
}
