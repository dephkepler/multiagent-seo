# Backend Code Audit — Harsh Adversarial Review

> Сгенерировано многоагентным состязательным аудитом: 11 критиков подняли замечания, каждое перепроверено чтением реального кода (false отсеяны, exaggerated понижены по severity). Дата: 2026-06-03.

## Честная оценка

**Grade: C+ — solid bones, unreliable behavior. Good structure and readable code dragged down by systemic swallowed-error and non-atomic patterns, real untested data races, and below-bar auth/security hygiene. Roughly 4-6 weeks of focused hardening from a B+/A-.**

**Verdict:**

> Competent, well-structured Go that is NOT yet production-grade. The architecture is sound (clean domain/application/infrastructure split, interface-driven, explicit DI in main.go, idiomatic chi router, RFC 9457 problem responses), but it is undermined by one pervasive bad habit: errors are logged-and-swallowed instead of propagated, and side-effecting operations are non-atomic. The most damaging instances actively corrupt business outcomes: a failed originality check is reported as passed (so AI-detected content can auto-publish), mid-batch sheet write failures silently drop backlink/login results while the user sees status=accepted, and publish-vs-database state can diverge under concurrency. Compounding this, several genuine data races (shared goldmark renderer, mutable Problem.ext map, unbuffered result channels) ship with zero concurrency tests; handlers are tested with auth middleware disabled (nil); and critical parsing functions (sheets.Lookup, pexels.SearchN) have no tests at all. Security hygiene is below bar for a credential-handling service: no /auth/login rate limiting, no minimum JWT secret length, no security headers/TLS, missing SSRF redirect guards on outbound clients. None of these are architectural dead-ends; they are correctable with disciplined mechanical fixes, but as it stands this code would lose data and silently misbehave in production.

## Статистика достоверности

- Поднято всего: **160**
- Подтверждено (confirmed): **67**
- Преувеличено, но реально (exaggerated): **75**
- Отброшено как ложное (false): **18**

**Severity (adjusted) среди выживших:** high=38, medium=41, low=38, none=8

## Системные темы

1. Swallowed errors as a way of life: the dominant defect. Check/Publish/SaveImageStats/SaveCheckResult/flush()/io.ReadAll/json.Marshal/WriteJSON all log-and-continue instead of propagating. The worst cases invert business outcomes (failed originality check reported as passed; accepted status returned while backlink/login batches are silently dropped).

2. Non-atomic side effects with no transactional or idempotency guarantee: Publish then MarkPublished, IssueAppPassword then Save, WordPress write then sheet write. Any second-step failure leaves external state and the database/sheet permanently divergent, and concurrent retries duplicate work.

3. Real data races shipped with zero concurrency tests: global goldmark renderer shared across generation goroutines, mutable Problem.ext map read during MarshalJSON, unbuffered resultsCh that can wedge worker goroutines. jobrunner (the concurrency primitive itself) has no test file and never runs under -race.

4. Testing theater on the security boundary: every handler unit/integration test instantiates the router with authMW=nil, so no test proves protected endpoints reject unauthenticated requests. Production is protected, but the contract is unverified and one forgotten handler-level check would silently expose an endpoint.

5. Missing test coverage on critical parsing-heavy seams: sheets.Lookup, pexels.SearchN, dataforseo parsing, and every error path in the article pipeline are untested, exactly the brittle regex/JSON-from-the-wild code most likely to regress.

6. Baseline security hygiene gaps for a credential-handling service: no /auth/login rate limiting, no JWT secret minimum length, no security headers or TLS enforcement, no SSRF/redirect guard on outbound pexels/dataforseo clients, no audit log for login success or token lifecycle.

7. Context lifecycle misuse: context.WithoutCancel() detaches background jobs and batch writes from shutdown cancellation, and Lookup unconditionally widens a callers tighter deadline to 10s, both undermining graceful shutdown and deadline propagation.

8. Fragile regex parsing of structured/untrusted text: JSON-like nonce extraction, [IMG|...] placeholders, and HTML are parsed with brittle negated-character-class regexes and raw byte truncation that breaks on nested brackets and multi-byte UTF-8, causing silent corruption on real-world inputs.

9. ASCII-only assumptions block multilingual use: tokenize() drops all non-ASCII letters and the captcha math solver is English-only, silently degrading non-English SEO and donor sites for a service whose value proposition is multilingual content.

10. Validation deferred to runtime: LLM provider names, JWT secret length, article Defaults (MinWords/MaxWords/threshold), and resolved model strings are accepted unvalidated and only fail (often silently as empty strings) deep in the call stack.

## Топ-10 приоритетных проблем

### 1. [HIGH] Failed originality check is reported as passed, allowing AI-detected content to auto-publish
- **Где:** `backend/internal/application/articles/service.go:353-356 (pipeline check at :326)`
- **Почему важно:** When checker.Check() errors, the function logs a warning and breaks with last still nil. In pipeline(), checkPassed := lastCheck == nil || lastCheck.Original then evaluates to true, so a check that actually FAILED is treated as a pass. Combined with the auto-publish path (:237-242), AI-flagged or unverified content can be published automatically with no signal. The single most damaging correctness bug; it inverts a safety gate.
- **Фикс:** Distinguish never-checked from check-errored. Track a checkFailed bool (or return a sentinel CheckResult{Original:false}) and make checkPassed default to false when the check could not complete. Never let an errored check resolve to passed, and block auto-publish on it.

### 2. [HIGH] Mid-batch flush() failure silently drops backlink/login results while user sees status=accepted
- **Где:** `backend/internal/application/linkbuilding/backlink_service.go:212-220 and login_service.go:141-149`
- **Почему важно:** When a mid-loop flush() (WritePlacementStatus/sheet write) fails, the function returns immediately and the accumulated pending results are never persisted. The work was done but the record is lost, with only a single error log line. The user already received status=accepted, so backlinks/logins appear queued but silently vanish. Real, repeated data loss across two services.
- **Фикс:** On flush failure, retry with bounded backoff before giving up; if it still fails, log the exact lost rows at ERROR with row IDs and surface the failure in the jobs terminal state. Do not silently return. Make WritePlacementStatus idempotent so retries are safe.

### 3. [HIGH] Publish and MarkPublished are non-atomic; DB and WordPress can diverge, enabling duplicate publishes
- **Где:** `backend/internal/application/articles/service.go:414-423`
- **Почему важно:** pub.Publish() succeeding but MarkPublished() failing leaves the article live in WordPress while the DB still shows StatusDraft. A concurrent or retried Publish() reads the stale draft status and republishes, producing duplicate posts or orphaned state. No idempotency key, no transaction spanning the external plus local update.
- **Фикс:** Make pub.Publish() idempotent (keyed on WPPostID or a publish token) so a duplicate call is a no-op, and/or write a publishing intent row before the external call and reconcile on MarkPublished failure. At minimum, on MarkPublished error surface a clear reconciliation-required state instead of silent divergence.

### 4. [HIGH] Global goldmark renderer shared across generation goroutines without synchronization
- **Где:** `backend/internal/domain/articles/render.go:28-40,76`
- **Почему важно:** var md = goldmark.New(...) is a package-global whose Convert() is called from RenderHTML, which runs inside background generation goroutines spawned via runner.Go(). goldmark is not documented thread-safe and markdown parsers hold internal state, so concurrent Convert() calls are a real data race that can panic or corrupt output. Ships with no -race test to catch it.
- **Фикс:** Do not share the renderer: construct md inside RenderHTML per call, guard it with a sync.Mutex, or use a sync.Pool of renderers. Add a -race test that runs RenderHTML concurrently.

### 5. [HIGH] Markdown render errors are swallowed, returning raw [IMG|...]/[INTERNAL_LINK|...] placeholders to WordPress
- **Где:** `backend/internal/domain/articles/render.go:76-78`
- **Почему важно:** If md.Convert() fails, RenderHTML returns the original unrendered content with the error discarded and never logged. The published article then contains literal placeholder markup instead of resolved images/links, a silently corrupted post, and the caller at service.go:292 has no way to know rendering failed.
- **Фикс:** Change the signature to return an error (string, RenderStats, error), log it, and have the pipeline fail the generation (MarkFailed with reason) rather than publishing corrupted content.

### 6. [HIGH] Handler tests run with auth middleware disabled (nil); no test proves protected endpoints reject unauthenticated requests
- **Где:** `backend/internal/infrastructure/http/handlers/wordpress_sites_test.go:134-143, articles_test.go:64, wordpress_sites_integration_test.go:26-43`
- **Почему важно:** Every handler unit/integration test passes nil as authMW to NewRouter, so requests bypass BearerAuth entirely. The OpenAPI spec declares bearerAuth on ~23 endpoints, but there is zero coverage proving a tokenless request returns 401. Production is protected via main.go, but the security contract is unverified; a single dropped handler-level check would silently expose an endpoint and no test would catch it. loglevel_test.go already shows the correct pattern.
- **Фикс:** Build a shared test router helper that wires real BearerAuth(verifier), issue a test JWT, and add explicit cases asserting 401 for missing/invalid tokens and 200 for valid tokens across protected endpoints.

### 7. [HIGH] No rate limiting or brute-force protection on /auth/login
- **Где:** `backend/internal/infrastructure/http/handlers/auth.go:33-62`
- **Почему важно:** The login endpoint accepts unlimited attempts with no per-IP/per-account throttling or lockout, and logs the attempted email. bcrypts ~100ms cost is the only friction. A standard, exploitable credential brute-force and account-enumeration surface that should not ship for a service guarding WordPress credentials and API tokens.
- **Фикс:** Add a sliding-window rate limiter (per IP and per email) returning 429 after a threshold, with exponential backoff or temporary lockout. Add a test asserting 429 after N failures.

### 8. [HIGH] JWT secret strength unvalidated; a 1-byte secret passes in production
- **Где:** `backend/pkg/config/config.go:199-240 and pkg/jwt/jwt.go:13-25`
- **Почему важно:** Validation only checks string equality against the dev default. JWT_SECRET=x passes in non-local environments and is used directly as the HMAC-SHA256 key, yielding a trivially brute-forceable signing key. No minimum-length enforcement at the config or jwt layer.
- **Фикс:** Enforce len(secret) >= 32 bytes (256 bits) in config.Load() and/or jwtauth.New, returning an error otherwise. Add tests for short-secret rejection.

### 9. [HIGH] Missing redirect (SSRF) guards on outbound pexels and dataforseo HTTP clients
- **Где:** `backend/internal/infrastructure/pexels/client.go:36 and dataforseo/client.go:27`
- **Почему важно:** Both clients are bare http.Client{Timeout} with no CheckRedirect, unlike webfetch which caps redirects and blocks private/loopback IPs. A malicious or compromised API response (dataforseo also sends Basic Auth credentials) can redirect the client to internal services, an SSRF-via-open-redirect path that bypasses webfetchs guards entirely.
- **Фикс:** Add a CheckRedirect that caps redirect count and rejects non-HTTPS, private, and loopback targets, mirroring webfetchs dial guard. Apply to both clients.

### 10. [HIGH] tokenize() drops all non-ASCII letters, breaking image matching for non-English articles
- **Где:** `backend/internal/domain/articles/render.go:182-187`
- **Почему важно:** The tokenizer only keeps a-z and 0-9, treating every Unicode letter (accented, CJK, etc.) as a delimiter. An accented word like cafe loses its final char, so PickRelevant fails to match photos for non-English content, a core failure for a service whose multilingual SEO is a selling point, and it fails silently with degraded results rather than an error.
- **Фикс:** Replace the ASCII range checks with unicode.IsLetter(r) and unicode.IsDigit(r) (or a Unicode-aware regex split). The same English-only assumption affects the captcha math solver (wplogin/form.go:22-28) and should be flagged for non-English sites.

## Все подтверждённые замечания (по областям)

Всего выживших после проверки: **142**. Каждое замечание: severity (исходный → adjusted), вердикт проверяющего, локация, цитата кода, влияние, фикс, и что подтвердил проверяющий.


### Domain + application/articles  (16)

#### 1. 🔴 Regex pattern in imgPlaceholderRE silently truncates content with nested brackets
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:18`
- **Код:** `imgPlaceholderRE = regexp.MustCompile(˗\[IMG\s*\|[^\]]*?\]˗)`
- **Влияние:** If image placeholder contains text with nested brackets (e.g., '[IMG | tool: [Photoshop] workflow | ALT:test]'), the regex matches only up to the first closing bracket, silently truncating the description. The truncated portion remains in the content, breaking image resolution logic and corrupting the output.
- **Фикс:** Use a balanced bracket parser or require bracket escaping in descriptions. Alternatively, validate placeholder format strictly and log/reject malformed placeholders rather than silently truncating.
- **Проверка:** The regex `\[IMG\s*\|[^\]]*?\]` on line 18 correctly matches characters except closing brackets, but this causes truncation when image placeholder descriptions contain nested brackets. For input `[IMG | tool: [Photoshop] workflow | ALT:test]`, the regex matches only to the first `]`, yielding `[IMG | tool: [Photoshop]`. The `parseImgPlaceholder()` function (lines 112-127) then extracts `desc = " tool: [Photoshop"` (incomplete), and the remainder ` workflow | ALT:test]` is left in content unmatched. This remainder gets passed to markdown rendering, potentially corrupting output or leaving dangling text. The `desc` is later used in `Resolve()` calls (line 60), so incomplete descriptions will cause incorrect image resolution. The issue is a real bug with correct severity assessment — while nested brackets in descriptions are uncommon, the silent truncation and leftover text handling constitute broken behavior rather than graceful degradation.

#### 2. 🔴 RenderHTML silently discards conversion errors, returning unrendered content
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:76-78`
- **Код:** `var buf bytes.Buffer
if err := md.Convert([]byte(stripped), &buf); err != nil {
  return content, stats
}`
- **Влияние:** If markdown conversion fails (e.g., due to memory exhaustion, corrupted goldmark state, or huge content), the error is silently swallowed and the original, unrendered content is returned. This means placeholders remain [IMG|...] and [INTERNAL_LINK|...], breaking the article in WordPress and losing all image/link resolution. The error is never logged or propagated.
- **Фикс:** Return the error: func RenderHTML(...) (string, RenderStats, error) and propagate it in the caller at line 292. Log the error and handle gracefully (e.g., return partial content with a warning).
- **Проверка:** The code at lines 76-78 does exactly what the finding describes: if md.Convert() fails, it returns the original unrendered `content` parameter along with `stats`, silently discarding the error without logging or propagating it. The error is never communicated to the caller (line 292 in service.go), which simply uses the returned body without any error handling. This means conversion failures would result in raw markdown placeholders like [IMG|...] remaining in the final output delivered to WordPress, breaking article rendering. The severity is accurately assessed as high because this is a silent failure mode that corrupts output data without any diagnostic signals.

#### 3. 🔴 checkAndHumanize silently returns nil CheckResult on error, losing originality status
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:353-387`
- **Код:** `checkRes, err := s.checker.Check(ctx, content)
if err != nil {
  log.WarnContext(ctx, "originality check failed, skipping", "cycle", cycle, "err", err)
  break
}`
- **Влияние:** If originality check fails on the first cycle, last remains nil. At line 326 (pipeline), checkPassed := lastCheck == nil || lastCheck.Original evaluates to true (passes) when the check actually failed. This means failed originality checks are silently treated as 'passes', allowing AI-detected content to be auto-published without any warning in the return value.
- **Фикс:** Track check failures separately: introduce a variable like checkFailed to distinguish between 'never checked' (nil) and 'check failed with error' (CheckResult{AIScore: -1, Original: false}) and propagate that status.
- **Проверка:** The code at lines 353-356 shows: when s.checker.Check() returns an error, the function logs a warning and breaks. The variable `last` was initialized to nil at line 350 and is never reassigned on error, so the function returns (content, nil) at line 386. In the calling function pipeline() at line 326, checkPassed := lastCheck == nil || lastCheck.Original short-circuits to true when lastCheck is nil, causing failed originality checks to be treated as passed. This is a genuine bug: when the originality check fails, the article is marked as passing the check. While a warning is logged at line 355, the return value (checkPassed=true) contradicts this and could cause downstream auto-publish logic to incorrectly publish AI-detected content without any check result. The finding's assessment is accurate."

#### 4. 🔴 Publish method does not atomically update status and URL, creating race condition window
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:414-423`
- **Код:** `postURL, err := pub.Publish(ctx, article.WPPostID)
if err != nil {
  s.log.ErrorContext(ctx, "publish failed", ...)
  return articles.Article{}, fmt.Errorf("publish: %w", err)
}

if err := s.repo.MarkPublished(ctx, articleID, postURL); err != nil {
  s.log.ErrorContext(ctx, "publish failed", ...)
  return articles.Article{}, fmt.Errorf("mark published: %w", err)
}`
- **Влияние:** If pub.Publish() succeeds but MarkPublished() fails, the article is published in WordPress but the database still shows it as StatusDraft. A concurrent Publish() call will see StatusDraft and attempt to publish again. If WordPress is eventually consistent or external state is lost, the database becomes the source of truth and the article is orphaned.
- **Фикс:** Make MarkPublished a transactional operation that includes the WPPostID verification and status change atomically, or implement idempotency in pub.Publish() to handle duplicate calls.
- **Проверка:** The code at lines 414-423 does perform non-atomic operations: it calls pub.Publish() to publish to WordPress, then separately calls s.repo.MarkPublished() to update the database status. If pub.Publish() succeeds but MarkPublished() fails, the article is published externally but the database remains in StatusDraft. A concurrent Publish() call can proceed past the status check at line 399 (which reads stale data from line 390) and attempt to republish. While there is an ErrAlreadyPublished check, it relies on fetching current state at the start of the function, creating a window where concurrent calls can race. The finding is accurate: this is a genuine race condition in the non-atomic update pattern. Severity remains high because pub.Publish() being called twice on the same WPPostID could result in duplicate publications or orphaned articles depending on WordPress behavior and idempotency handling.

#### 5. 🔴 tokenize() function silently treats Unicode letters as invalid, breaking non-English keywords
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:182-187`
- **Код:** `for _, r := range strings.ToLower(s) {
  if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
    cur.WriteRune(r)
  } else {
    flush()
  }
}`
- **Влияние:** The function only accepts ASCII a-z and digits. Non-English keywords (e.g., 'café', 'Müller', Chinese characters) are split into individual characters and discarded. PickRelevant will fail to match relevant photos for non-English articles. For a multilingual SEO service, this is a critical logic bug.
- **Фикс:** Use unicode.IsLetter(r) and unicode.IsDigit(r) instead of ASCII range checks, or use a regex to split on non-word characters.
- **Проверка:** The tokenize() function at lines 182-187 does exactly as described: it uses ASCII range checks (r >= 'a' && r <= 'z') and (r >= '0' && r <= '9') to decide which characters to include. All Unicode letters (é, ü, Chinese characters, etc.) are treated as delimiters and trigger flush(), silently splitting non-English words. For example, 'café' becomes 'caf' (if >= 3 chars), losing the actual keyword. This breaks PickRelevant() photo matching for non-English articles. Adjusted from "medium" to "high" because while the bug is confirmed and real, the actual impact depends on whether non-English SEO is a primary feature — if it is, this would be critical.

#### 6. 🔴 Concurrent access to goldmark.New() global md without synchronization
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:28-40, 76`
- **Код:** `var md = goldmark.New(...)

func RenderHTML(...) {
  ...
  if err := md.Convert([...]&buf); err != nil {`
- **Влияние:** The goldmark.New() returns a potentially reusable renderer. If goldmark is not thread-safe (common for stateful parsers), concurrent calls to RenderHTML will race on md's internal state. This is undefined behavior and may cause panics, data corruption, or silent rendering errors.
- **Фикс:** Benchmark goldmark's thread safety. If not safe, either: (1) create md inside RenderHTML, (2) use sync.Mutex to serialize access, or (3) use a sync.Pool of renderers.
- **Проверка:** The code at lines 28-40 declares a global goldmark.Markdown instance `var md = goldmark.New(...)`. This instance is called from RenderHTML (line 76) via `md.Convert([]byte(stripped), &buf)`. RenderHTML is invoked from the service's pipeline() function (service.go:292), which executes in background goroutines spawned by s.runner.Go() (service.go:154), allowing concurrent execution. Goldmark is not documented as thread-safe, and markdown parsers typically maintain internal parsing state. This creates a real race condition where concurrent calls to md.Convert() from multiple goroutines will access the same parser instance without synchronization, which can cause panics, data corruption, or silent rendering errors. Adjusting severity to high because concurrent background jobs are actively spawned for article generation, making this not a theoretical issue but a practical runtime risk.

#### 7. 🔴 No test coverage for error paths, pipeline failures, or multipart generation flows
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service_test.go:97-138`
- **Код:** `Only TestGenerate_HappyPath and TestGenerate_NoClusterAborts exist. No tests for: SERP failures, LLM failures, originality check failures, humanize cycles, publish errors, etc.`
- **Влияние:** The test suite has only two tests, both covering the happy path or early exit. Critical error paths (line 249 SERP failure handling, line 354 check failure, line 378 humanize failure, line 322 stats save failure) are untested. This means regressions in error handling will not be caught, and the promise that errors are properly logged/propagated is unvalidated.
- **Фикс:** Add tests for: fakeChecker returning errors, fakeLLM returning errors, pipeline failures (brief, writer, editor, publisher failures), image resolution failures, and verify that errors are properly wrapped and logged.
- **Проверка:** The test file contains exactly 2 tests: TestGenerate_HappyPath (lines 97-121) covering the success path, and TestGenerate_NoClusterAborts (lines 123-132) covering one early validation failure. The service.go implementation contains multiple error handling paths that are untested: SERP failures (lines 248-256 handle gracefully), LLM failures during brief/writer/editor (lines 261-275 return errors), originality checker failures (lines 353-357 warn and break), humanize rewrite failures (lines 378-382 warn and break), publisher failures (lines 305-316 return errors), and repo operation failures (lines 319-323 warn on SaveImageStats). None of these error paths have corresponding tests. The fake implementations all return success. While the code has proper error handling with logging and status updates, the test suite provides no verification that these error paths execute correctly, which is a high-severity gap for a critical generation pipeline.

#### 8. 🟠 SEO field extraction regex matches empty values without proper bounds
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/seo_fields.go:9-10`
- **Код:** `titleRE := regexp.MustCompile(˗(?im)^[\s>*_\-]*\*{0,2}\s*SEO\s+Title\*{0,2}\s*[:\-]\s*(.+?)\s*\*{0,2}\s*$\n?˗)`
- **Влияние:** Test with 'SEO Title: ' (trailing space, no actual value) matches with capture group containing just a space. After stripMarkdown() trims it, an empty string is returned and treated as 'no title found', but the line was still matched and removed from content. This creates a silent data loss pathway where malformed fields are consumed but lost.
- **Фикс:** Change (.+?) to ([^\n]+) and validate captured content is non-empty before returning; otherwise treat as no match and leave original content intact.
- **Проверка:** The regex (.+?) does require at least one character, so the exact example "SEO Title: " (no characters after colon+space) would NOT match. However, the core vulnerability is real: a line like "SEO Title:   " (with trailing spaces/whitespace) WILL match, capture only whitespace, which stripMarkdown() and TrimSpace() reduce to an empty string. The line is then removed from content via ReplaceAllString(), causing silent data loss. The fix suggestion is sound: validate captured content is non-empty before extracting, or treat empty extractions as no match and leave the original line intact.

#### 9. 🟠 Model field can be empty string passed to LLM factory without validation
- **Severity:** high → medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:179-182, 217`
- **Код:** `model := req.Model
if model == "" && provider == s.defaults.Provider {
  model = s.defaults.Model
}
...
client, err := s.llm.ForModel(settings.provider, settings.model)`
- **Влияние:** If a custom provider is specified (not the default), model remains empty string. This empty string is passed to ForModel() at line 217. While the error is caught, there's no validation that the defaults Model is set or non-empty, nor that a non-default provider has a model specified. This allows invalid configurations to reach the LLM factory.
- **Фикс:** Add explicit validation: if model is empty after resolution, return an error before proceeding. Require either defaults to have a non-empty Model or the request to specify one.
- **Проверка:** The code at lines 179-182 correctly shows: model is only filled from defaults if model is empty AND the provider matches the default provider. If a custom provider is specified without a model, the model remains empty and is passed to s.llm.ForModel() at line 217. The finding is accurate about the logic flaw. However, severity is medium rather than high because the error is properly caught at line 218 with explicit error handling that propagates upstream. The issue is improper validation logic (should validate model before ForModel call), not a silent failure or bypass. The fix suggested (explicit validation) is correct and would be a good improvement, but this isn't a critical vulnerability if ForModel() reliably rejects empty models.

#### 10. 🟠 maxTokens can be zero, allowing silent LLM call with no token budget
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:211-215`
- **Код:** `if req.MaxTokens > 0 {
  settings.maxTokens = req.MaxTokens
} else if settings.maxWords > 0 {
  settings.maxTokens = settings.maxWords*3 + 200
}`
- **Влияние:** If both req.MaxTokens is 0 and settings.maxWords is 0 (or maxWords is set but the condition is skipped), settings.maxTokens remains uninitialized (zero). All subsequent LLM calls at lines 261, 267, 273, 378 receive maxTokens=0, which may cause LLM API to reject the call or produce truncated output without error propagation.
- **Фикс:** Add a fallback: after the else-if block, if settings.maxTokens <= 0, set it to a sensible minimum (e.g., 2000) and log a warning about using fallback.
- **Проверка:** The code at lines 211-215 does have a logic gap: if req.MaxTokens <= 0 AND settings.maxWords <= 0 (which can happen if both defaults and request values are zero), then settings.maxTokens remains at its zero initialization and is passed to LLM client calls at lines 261, 267, 273, and 378 without validation. The spec struct initializes maxTokens to 0 (Go zero value), and the if-else chain only sets it if one of two conditions is met; otherwise it stays 0. The actual impact (silent failure vs. API error) depends on the LLM client's handling of maxTokens=0, but the code path is definitely real and unguarded.

#### 11. 🟠 PickRelevant returns nil when best score is 0, silently discarding all photos
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:146-156`
- **Код:** `bestIdx, bestScore := -1, 0
for i, ph := range photos {
  ...
  if score > bestScore {
    bestIdx, bestScore = i, score
  }
}
if bestIdx < 0 {
  return nil
}`
- **Влияние:** If all photos have zero overlap (wanted keywords not in any photo's ALT or URL), bestIdx remains -1 and nil is returned, even though photos array is non-empty. The caller has no way to distinguish between 'no photos available' and 'no good match found', leading to missing images. Additionally, if wanted is non-empty but all photos have zero overlap, a reasonable fallback (first photo, or any photo) would be better than nil.
- **Фикс:** If bestIdx < 0 after scoring, fall back to returning &photos[0] (first photo as last resort) instead of nil, or at minimum change the condition to allow score=0 matches.
- **Проверка:** The code initializes bestIdx=-1 and bestScore=0, then only updates bestIdx when score > bestScore (strictly greater). If all photos have zero keyword overlap, all scores are 0, so the condition never triggers and bestIdx remains -1, causing the function to return nil even with non-empty photos array. This contradicts the pattern seen earlier in the function (lines 142-143) where empty wanted keywords returns photos[0]. The bug is real: photos with zero relevance scores are silently discarded instead of returning a fallback photo.

#### 12. 🟠 No validation that SaveImageStats failure prevents article publication
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:322-324`
- **Код:** `if err := s.repo.SaveImageStats(ctx, articleID, renderStats.ImagesRequested, renderStats.ImagesResolved, renderStats.ImagesSkipped); err != nil {
  log.WarnContext(ctx, "save image stats", "err", err)
}`
- **Влияние:** SaveImageStats failure is logged but not returned. If the database fails to save image statistics, the generation continues and returns success, leaving the database in an inconsistent state (article published but stats missing). This violates the implicit contract that all side effects succeed or the whole operation fails.
- **Фикс:** Change this to: if err != nil { return false, fmt.Errorf("save image stats: %w", err) }
- **Проверка:** Lines 322-324 in the `pipeline` function correctly show SaveImageStats error being caught and only logged with log.WarnContext, without returning an error. The function continues and returns (checkPassed, nil) on line 334, meaning successful completion despite SaveImageStats failure. This is inconsistent with prior error handling on lines 319-321 where UpdateDraft errors are properly propagated. The finding accurately describes the problem: if SaveImageStats fails, the database will be inconsistent (article drafted but stats missing). However, this pattern is repeated elsewhere in the code (SaveCheckResult at 365-367, SaveCompetitorData at 254-256 also warn-only), suggesting it may be intentional design rather than an oversight. Severity remains medium as it is a real consistency and data integrity issue, but the pattern suggests this is a deliberate choice to make stats operations non-blocking.

#### 13. 🟠 Defaults struct has no validation; can be initialized with invalid combinations
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:25-37, 70-97`
- **Код:** `type Defaults struct {
  MinWords int
  MaxWords int
  ...
}

func NewService(..., defaults Defaults, ...) *Service {
  if log == nil {
    log = slog.Default()
  }
  return &Service{...defaults...}`
- **Влияние:** NewService accepts a Defaults struct with no validation. If MinWords > MaxWords, or both are 0, or AIThreshold is 1.0 or 2.0, the Service is created with invalid configuration. These invariants are only caught at runtime when resolve() or checkAndHumanize() execute their own fallback logic, creating silent configuration errors.
- **Фикс:** Add a NewDefaults() constructor that validates: MinWords > 0, MinWords < MaxWords, AIThreshold in (0, 1), MaxCycles > 0, SERPLimit >= 0. Return error for invalid configs.
- **Проверка:** Verified: The Defaults struct (lines 25-37) has no validation. NewService (lines 70-97) accepts a Defaults struct with no checks. The resolve() method (lines 168-223) does use these defaults via pickInt/pickFloat helpers (lines 187-195), passing them directly into the spec struct. The checkAndHumanize() method (lines 337-387) implements fallback logic with hardcoded defaults: if maxCycles <= 0 it becomes 3 (line 340); if aiThreshold == 0 or <= 0 it becomes 0.8 (lines 343-348). However, there is NO validation that minWords < maxWords, minWords > 0, maxWords > 0, or aiThreshold in valid range (0, 1). These values are passed directly to LLM prompts (lines 267, 273) and used in calculations (line 214: maxWords*3 + 200). If Defaults has MinWords=2000, MaxWords=1000, or AIThreshold=1.5, or both MinWords and MaxWords are 0, the Service will be created silently with invalid config. The fallback logic only applies at runtime in checkAndHumanize for maxCycles and aiThreshold, not for word count constraints, and not at construction time. The finding is accurate — no validation happens at NewService construction time.

#### 14. 🟡 Competitors.Render() and Competitors.RenderSlim() duplicate identical empty-check logic
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** duplication
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/prompt/competitor.go:52-54 and 100-103`
- **Код:** `func (c Competitors) Render() string {
  if len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil {
    return ""
  }

func (c Competitors) RenderSlim() string {
  if len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil {
    return ""
  }`
- **Влияние:** The identical three-condition check is duplicated. This creates a maintenance burden: if the isEmpty logic changes, both methods must be updated. Additionally, the check is the same for both Render() and RenderSlim(), suggesting they could share a helper.
- **Фикс:** Extract a private method: func (c Competitors) isEmpty() bool { return len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil }
- **Проверка:** Verified: Both Competitors.Render() at lines 52-54 and Competitors.RenderSlim() at lines 100-103 contain identical guard logic: `if len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil { return "" }`. The duplication is real. The proposed fix to extract an isEmpty() helper method is sound and would eliminate the maintenance burden of updating the check in two places. Severity is appropriately low — it's code duplication with minimal functional impact, but it is a legitimate code quality issue.

#### 15. 🟡 No validation of Cluster.Keywords in prompt generation, allowing empty or invalid keywords
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:201-209`
- **Код:** `cluster, err := s.topics.Lookup(ctx, req.Keyword)
if err != nil {
  return spec{}, fmt.Errorf("cluster lookup: %w", err)
}
if len(cluster.Keywords) == 0 {
  s.log.WarnContext(ctx, "no keyword cluster for topic", "keyword", req.Keyword)
  return spec{}, ErrNoCluster
}
settings.cluster = cluster`
- **Влияние:** The code only checks that cluster.Keywords is non-empty, not that keywords are valid strings. If Lookup returns a cluster with empty strings in Keywords (e.g., Cluster{Keywords: ["", "valid"], ...}), renderCluster() at prompt.go will silently skip empty keywords and the prompt will have fewer target keywords than expected, degrading the brief and article quality.
- **Фикс:** In resolve(), validate cluster.Keywords: filter out empty strings and re-check that at least one valid keyword remains, or require the TopicSource to return only non-empty keywords.
- **Проверка:** The code checks len(cluster.Keywords) == 0 at line 205 and returns ErrNoCluster if true. However, the finding's concern about empty strings in the Keywords slice is overstated. The Sheets implementation (the only TopicSource in the codebase) explicitly filters out empty keywords at lines 102-104 of sheets/client.go before appending to cluster.Keywords. Additionally, renderCluster() in prompt/cluster.go defensively trims and skips empty strings at lines 14-16. So the vulnerability described (empty strings passing through) cannot occur with the actual implementation. A real risk would only exist if a different TopicSource implementation failed to validate, which would be that implementation's responsibility. Severity should be low rather than medium since defensive measures already exist and the happy path cannot produce empty keywords.

#### 16. ⚪ parseImgPlaceholder silently returns empty desc for single-pipe placeholders
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/domain/articles/render.go:112-126`
- **Код:** `parts := strings.Split(inner, "|")
if len(parts) >= 2 {
  desc = strings.TrimSpace(parts[1])
}
for _, p := range parts {
  t := strings.TrimSpace(p)
  if strings.HasPrefix(strings.ToUpper(t), "ALT:") {
    alt = strings.TrimSpace(t[len("ALT:"):])}
    break
  }
}`
- **Влияние:** For '[IMG|]' (no description, no ALT), parts has length 2, desc remains empty string. The image resolution at line 60 is called with desc="", and if keyword is also empty, stats.ImagesSkipped++ occurs. But the placeholder is still matched and removed. If the prompt always produces [IMG|description|ALT:...], this is a silent data loss edge case.
- **Фикс:** Validate parseImgPlaceholder result: if both desc and alt are empty, return "" from RenderHTML to skip resolution and log a warning about malformed placeholder.
- **Проверка:** The code at lines 112-126 correctly parses [IMG|] placeholders, returning empty desc and alt strings. However, the RenderHTML function at line 56 explicitly handles this case: it skips the image and increments stats.ImagesSkipped ONLY when both desc and alt are empty AND the keyword is empty. This is intentional error handling, not silent data loss. The finding mischaracterizes this as unhandled—the code already validates the result. The edge case is real but already mitigated; no fix needed.


### application/linkbuilding  (12)

#### 17. 🔴 Silent data loss: flush() error in backlink placement is ignored, execution continues without ensuring results were persisted
- **Severity:** high  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:212-220`
- **Код:** `if len(pending) >= placeWriteChunk {
    if !flush() {
        return
    }
}
}
flush()
log.InfoContext(ctx, "backlink placement done", "processed", processed)`
- **Влияние:** If flush() fails mid-batch (network error, quota exceeded, etc.), placeAll() returns early without writing the final pending batch. Those placement records are lost. A user sees status=accepted but backlinks are never actually placed. Worse: the final flush() at line 219 is called unconditionally and may succeed, hiding the earlier failure.
- **Фикс:** Track flush errors; if a mid-loop flush fails, either retry with exponential backoff, or abort entirely and log which results were lost. At minimum: if !flush() { log.ErrorContext(ctx, "backlink placement batch lost", "pending", len(pending)); return } and do NOT call flush() again without retry logic.
- **Проверка:** The code at lines 212-220 does have a real data loss issue, but the finding mischaracterizes how it occurs. When flush() fails at line 213, the function returns immediately at line 214, so the final flush() at line 219 is never reached — this part of the finding is incorrect. However, the core issue is genuine: if the mid-batch WritePlacementStatus() call fails (line 180), the pending batch is permanently lost because the function exits without retry, leaving those PlacementResult records unwritten to the database. The error is logged at line 183, but this provides no recovery. The severity remains high because: (1) backlinks are never actually placed despite returning accepted status to the user, (2) no retry mechanism exists, and (3) the loss is silent except for a single error log line. The finding correctly identifies the data loss but incorrectly claims the final flush() call could hide the failure.

#### 18. 🔴 Silent data loss: flush() error in login service mid-batch silently swallows placement results
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/login_service.go:141-149`
- **Код:** `if len(pending) >= loginWriteChunk {
    if !flush() {
        return
    }
}
}
flush()
log.InfoContext(ctx, "site login done", "processed", processed)`
- **Влияние:** Identical to backlink service: mid-batch flush failures cause login results to be silently dropped. A user queues login, gets status=accepted, but a subset of logins are never written to the sheet because a write error mid-stream causes early return.
- **Фикс:** Same as backlink: return early if mid-loop flush() fails, do NOT attempt a second flush() without context about what was lost.
- **Проверка:** Verified by reading the actual code at login_service.go lines 141-149. The pattern is exactly as described: when `flush()` fails during the mid-loop batch write (line 142), the function returns immediately (line 143) without clearing or logging the `pending` results. This means any login results accumulated in `pending` at the time of flush failure are silently dropped—they are never written to the sheet. The final `flush()` call at line 148 is unreachable when flush fails mid-loop because the function has already exited. The same pattern exists identically in backlink_service.go lines 212-220, confirming this is a systematic data loss bug. Users receive status=accepted (line 95) but a subset of logins are never persisted due to mid-batch write failures. This is high-severity data loss with no recovery or retry mechanism."

#### 19. 🟠 Deadlock risk: goroutine launcher holds exclusive access to sem semaphore slot while blocked on resultsCh send
- **Severity:** medium  ·  **Verdict:** exaggerated  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:137-143`
- **Код:** `wg.Add(1)
go func(w domain.Website) {
    defer wg.Done()
    defer func() { <-sem }()
    if res, ok := s.qualifyOne(ctx, log, w, candidates, accepted, classifier); ok {
        resultsCh <- res
    }
}(w)`
- **Влияние:** The result is goroutine starvation and timeout. If the handler context times out first, the loop breaks, resultsCh is left open, workers try to send forever, and the job never completes.
- **Фикс:** Buffer the resultsCh: resultsCh := make(chan domain.Result, maxConcurrentSites) or larger. This decouples the sender from the receiver. Alternatively, use a bounded queue with backpressure: track outstanding sends and block the launcher if queue is full.
- **Проверка:** The code does have a goroutine coordination risk with the unbuffered resultsCh, but NOT the semaphore deadlock described. The launcher (lines 128-143) acquires a semaphore slot at line 134, spawns the worker goroutine, and immediately continues the loop—it does not hold the semaphore while the worker sends. The actual issue: if the main loop reading from resultsCh exits (via context timeout) before workers complete, workers blocked on the unbuffered send (line 141) prevent wg.Wait() from completing. The fix of buffering resultsCh is sound, but the mechanism is goroutine send-blocking, not semaphore starvation. The finding's stated cause (launcher holding semaphore while blocked on send) is inaccurate to the code structure.

#### 20. 🟠 String-based status filtering is fragile and case-sensitive, breaks on whitespace variance
- **Severity:** medium  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:131-139`
- **Код:** `st := strings.TrimSpace(strings.ToLower(c.LoginStatus))
if st != "" && !strings.HasPrefix(st, "login ok") {
    skippedLogin++
    continue
}
if strings.HasPrefix(strings.TrimSpace(strings.ToLower(c.PlacementStatus)), "placed:") {
    skippedAlreadyPlaced++
    continue
}`
- **Влияние:** Status strings are user-written into the sheet and subject to variance: 'Login OK' vs 'login ok' vs 'LOGIN OK', with/without spaces. The code normalizes loginStatus but not placementStatus identically. If a user enters 'LOGIN OK  ' (with trailing spaces), TrimSpace removes them, but if they enter 'Login OK', the ToLower() is applied. However, the prefix check is done AFTER trim+lower, so ' login ok' and 'login ok ' both work. BUT: if someone manually edited the sheet and wrote 'Login Successful' instead of 'login ok', the filter fails silently—that donor is queued for placement even though login never succeeded. There is no enum or validation of status strings.
- **Фикс:** Define status enums (e.g., LoginStatusOK = 'login_ok') in domain and use structured validation. Or: define a helper func IsLoginOK(status string) bool { return strings.Contains(strings.ToLower(status), 'ok') } and use consistently. Better: store login status as a typed field (bool or enum) in the sheet, not a string.
- **Проверка:** The code at lines 131-139 does normalize both LoginStatus and PlacementStatus identically using strings.TrimSpace(strings.ToLower(...)); the normalization is NOT inconsistent as the finding claims. Both lowercase and trim. The finding's concern about "Login OK" vs "login ok" is correctly handled by ToLower(). However, there IS a real fragility: (1) empty LoginStatus strings are accepted as valid (line 132 condition: if st is empty, the skip is not triggered), which silently treats blank fields as success; (2) any non-empty LoginStatus not starting with "login ok" is silently skipped with no validation error or enum; (3) typos like "login successful" are rejected silently. The core correctness risk is valid but the technical description in the finding is inaccurate—it's not about inconsistent normalization of the two fields, but about lack of enum validation and treating empty strings as valid logins. The severity is fair at medium given the silent filtering behavior could mask data issues in sheets."

#### 21. 🟡 Uninitialized random seed: math/rand.Int63n() uses unseeded global source, producing deterministic jitter
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/login_service.go:165-170`
- **Код:** `func (s *LoginService) jitter() time.Duration {
    if s.maxDelay <= s.minDelay {
        return s.minDelay
    }
    return s.minDelay + time.Duration(rand.Int63n(int64(s.maxDelay-s.minDelay)))
}`
- **Влияние:** math/rand is unseeded by default since Go 1.20+ seeds itself, but using the global source without explicit seeding is bad practice and relies on init-time behavior. More critically: tests that run in parallel can hit rate limits if all instances generate identical jitter sequences. In production, if multiple SDK instances are spawned (e.g., Kubernetes replicas), all will use the same default seed → synchronized thundering herd on rate-limited endpoints.
- **Фикс:** Use math/rand.New(math.NewSource(time.Now().UnixNano())) to create a per-service random instance, or better: use time.Now().Nanoseconds() directly without the extra rand call. Or: s.rand := rand.New(rand.NewSource(time.Now().UnixNano())) in NewLoginService and use s.rand.Int63n().
- **Проверка:** The code at lines 165-170 correctly uses rand.Int63n() to generate jitter between minDelay and maxDelay. The finding is REAL in that the code relies on global unseeded rand without explicit initialization. However, Go 1.20+ (this project uses 1.25.0) automatically seeds rand.Int63n() at init time from time.Now().UnixNano(), so deterministic output across runs is not a concern. The "thundering herd" scenario in Kubernetes is overstated since each pod runs a separate process with different auto-generated seeds. The valid concern is test flakiness when parallel tests use the shared global source, but this is low-impact. The severity should be LOW, not MEDIUM — using an explicit seeded source would be cleaner, but the current code is not broken in practice.

#### 22. 🟡 Identical uninitialized random seed in backlink service
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:283-288`
- **Код:** `func (s *BacklinkService) jitter() time.Duration {
    if s.maxDelay <= s.minDelay {
        return s.minDelay
    }
    return s.minDelay + time.Duration(rand.Int63n(int64(s.maxDelay-s.minDelay)))
}`
- **Влияние:** Same as login service: global unseeded random source, synchronized jitter across replicas, thundering herd risk.
- **Фикс:** Use per-instance seeded rand source as described above.
- **Проверка:** The code at lines 283-288 correctly cited and does use global unseeded rand.Int63n() without per-instance seeding. However, the severity is overstated. While the unseeded random source is poor practice, the actual impact is minimal: jitter is used only for request delays during backlink placement (5-second default max). The timing variance from unseeded randomness adds negligible impact—a few milliseconds difference in request timing doesn't cause correctness issues. The "thundering herd" concern is theoretical at best, as Go processes don't share rand state across instances. This is a design quality issue (should use seeded *rand.Rand per instance), not a correctness/severity bug warranting "medium" rating. Appropriate severity: low/design-debt.

#### 23. 🟡 Incomplete cancellation logic: placeOne() continues execution after ctx.Err() even if cancelled mid-step
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:190-210`
- **Код:** `for i, c := range creds {
    if ctx.Err() != nil {
        log.WarnContext(ctx, "backlink run cancelled", "processed", processed)
        break
    }
    if i > 0 {
        s.sleep(ctx)
    }
    log.InfoContext(ctx, "donor selected", "row", c.Row, "url", c.BaseURL, "login_status", c.LoginStatus)
    res := s.placeOne(ctx, log, c, targetURL, placer)
    if ctx.Err() != nil {
        log.WarnContext(ctx, "backlink run cancelled mid-step", "url", c.BaseURL, "processed", processed)
        break
    }
    processed++
    pending = append(pending, res);
}`
- **Влияние:** If ctx.Err() becomes non-nil INSIDE placeOne() (e.g., during placer.Place() or editor.UpdatePostContent()), the outer loop checks ctx.Err() AFTER placeOne() returns. So placeOne() finishes executing all stages even if context was cancelled. This is wasteful (unnecessary API calls) and confusing. The result is appended and processed++ is incremented, treating it as a valid completion even though cancellation was requested.
- **Фикс:** Pass cancellation context to placeOne and check ctx.Err() at each stage boundary: if err := s.editor.LatestPost(ctx, donor); err != nil { if ctx.Err() != nil { return res } ... }. Or: check ctx.Err() at the loop top and ALSO before appending: if ctx.Err() != nil { log.WarnContext(ctx, ..., "processed", processed); break } after placeOne() and before pending append.
- **Проверка:** The code at lines 190-210 shows: after placeOne() returns at line 201, it checks ctx.Err() at line 203 and breaks BEFORE appending (line 209) and incrementing processed (line 208). So the reviewer's claim that "the result is appended and processed++ is incremented, treating it as a valid completion" is false—the break at line 205 prevents both. The real issue (which is lower severity) is that placeOne() itself has no mid-stage cancellation checks and will execute all stages (IssueAppPassword, LatestPost, Place, UpdatePostContent) to completion unless an operation fails. This is inefficient during cancellation but not a correctness bug since the result is discarded by the outer loop. The finding overstates the impact as a data corruption issue when it's actually wasteful resource usage during shutdown.

#### 24. 🟡 Incomplete cancellation logic in login service: same mid-step vulnerability
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/login_service.go:122-139`
- **Код:** `for i, c := range creds {
    if ctx.Err() != nil {
        log.WarnContext(ctx, "login run cancelled", "processed", processed)
        break
    }
    if i > 0 {
        s.sleep(ctx)
    }
    res, err := s.auth.Login(ctx, c)
    if err != nil {
        log.WarnContext(ctx, "login aborted (context)", "url", c.BaseURL, "err", err)
        break
    }
    processed++
    pending = append(pending, res);`
- **Влияние:** If s.auth.Login() is cancelled mid-execution, err will be non-nil (context.Canceled), and the loop breaks. But if Login() succeeds despite ctx.Err() becoming true between the pre-check and the actual login call (timing window), the result is recorded. The logged message 'login aborted (context)' is misleading—it only logs if err is non-nil, but doesn't explicitly check if context was cancelled.
- **Фикс:** Check ctx.Err() AFTER the login call: res, err := s.auth.Login(ctx, c); if ctx.Err() != nil { log.WarnContext(...); break } BEFORE checking err. Or structure as: if err := s.auth.Login(...); err != nil || ctx.Err() != nil { ... }.
- **Проверка:** The code at lines 122-139 does check ctx.Err() before entering the loop body (line 123), and breaks if the context is already cancelled. The s.auth.Login(ctx, c) call at line 131 receives the context and should respect cancellation by returning an error. The reviewer's claimed "vulnerability" about recording results despite cancellation requires Login to succeed despite context cancellation, which would be a bug in Login itself. The actual issue here is the misleading log message at line 133: it logs "login aborted (context)" for any error (network, auth, or context-related), not specifically for context cancellation. This is a logging clarity issue (low severity), not a mid-execution vulnerability (medium severity) as claimed.

#### 25. 🟡 Orphaned goroutine in qualifyAll: if ctx is cancelled during result consumption, the sender goroutine may panic sending on closed channel
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:127-170`
- **Код:** `go func() {
    for _, w := range sites {
        select {
        case <-ctx.Done():
            wg.Wait()
            close(resultsCh)
            return
        case sem <- struct{}{}:
        }
        wg.Add(1)
        go func(w domain.Website) {
            defer wg.Done()
            defer func() { <-sem }()
            if res, ok := s.qualifyOne(ctx, log, w, candidates, accepted, classifier); ok {
                resultsCh <- res
            }
        }(w)
    }
    wg.Wait()
    close(resultsCh)
}()

processed := 0
for res := range resultsCh {
    batch = append(batch, res)
    processed++
    if len(batch) >= resultFlushBatch {
        flush()
    }
}
flush()`
- **Влияние:** Worker goroutines execute qualifyOne(ctx, ...) which may take seconds. If flush() times out or WriteResults() fails, the main loop exits (via context cancellation or explicit break), which is not shown in the code but is implied by error handling. When the main loop exits, resultsCh is never explicitly closed by the caller. Meanwhile, worker goroutines complete and try to send on resultsCh. Since no one is reading, the goroutines block indefinitely on the send (unless buffered—it's unbuffered at line 123). After handler returns, these goroutines are orphaned, leaking resources and goroutines.
- **Фикс:** Ensure resultsCh is closed regardless of how the main loop exits: defer close(resultsCh) at the top of the sender goroutine, OR create a second context-aware mechanism: use a sync.Once to close resultsCh and call it from both the sender (after wg.Wait()) and the consumer (on exit). Better: consume with a background receiver that drains the channel until closed.
- **Проверка:** The sender goroutine (lines 127-147) properly manages the channel lifecycle: it closes resultsCh either on ctx.Done() (line 132) or after wg.Wait() completes (line 146). The consumer loop (lines 163-170) simply ranges over resultsCh until it's closed; there is no early exit mechanism. Workers can only send on the channel while it's open, and the channel is guaranteed to be closed only after all workers have completed (wg.Wait() + close). No send-on-closed-channel panic is possible. The finding conflates a reasonable code pattern with a non-existent bug based on speculation about "implied" early exits that don't exist in the actual code.

#### 26. 🟡 Missing error context: qualification failure logged at WARN level, but result still written with Suitable=false
- **Severity:** medium → low  ·  **Verdict:** confirmed  ·  **Category:** logging
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:179-206`
- **Код:** `res := domain.Result{Row: w.Row, URL: w.URL}
page, err := s.fetcher.Fetch(ctx, w.URL)
if err != nil {
    if isCanceled(ctx, err) {
        return domain.Result{}, false
    }
    log.WarnContext(ctx, "fetch failed, marking unsuitable", "url", w.URL, "err", err)
    return res, true
}
res.OutboundDomains = domain.CountExternalDomains(w.URL, page.Links)
topic, err := classifier.Classify(ctx, page, candidates)
if err != nil {
    if isCanceled(ctx, err) {
        return domain.Result{}, false
    }
    log.WarnContext(ctx, "classify failed", "url", w.URL, "err", err)
} else {
    res.Topic = topic
}
res.Suitable = domain.IsSuitable(res.Topic, accepted)
log.DebugContext(ctx, "site qualified", "url", w.URL, "topic", res.Topic, "outbound", res.OutboundDomains, "suitable", res.Suitable)
return res, true`
- **Влияние:** When classification fails (line 194), res.Topic is left empty string, res.Suitable is set to false (IsSuitable returns false for empty topic). The result is written to the sheet with Suitable=no and no reason in the Topic column. User sees a failed site but logs (WARN level) are not correlated to the sheet data. Analyst cannot distinguish between 'fetch failed' vs 'classify failed' vs 'genuinely unsuitable.' Adds noise to logs without usable signal.
- **Фикс:** Encode failure reason in res.Topic: res.Topic = 'FETCH_FAILED' or 'CLASSIFY_FAILED', OR add a new Result field for status/error reason. Alternatively, use different handling: mark as unknown rather than unsuitable, or use a parallel error column in the sheet.
- **Проверка:** The finding is accurate: when Classify fails (line 194), res.Topic remains empty, IsSuitable("", accepted) returns false (line 42 in qualify.go), and the result is written with Suitable=false and Topic="" (line 205 returns true). The WARN log at line 198 is not correlated to sheet data. However, severity should be 'low' not 'medium' — the outcome is correct (unsuitable sites are properly excluded), the issue is missing diagnostic context in the result data. A log-only issue without data correctness impact."

#### 27. 🟡 Unchecked context cancellation propagation: placeOne() does not return error on cancellation, masking failures
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:223-268`
- **Код:** `func (s *BacklinkService) placeOne(ctx context.Context, log *slog.Logger, c domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) domain.PlacementResult {
    res := domain.PlacementResult{Row: c.Row, DonorURL: c.BaseURL}
    donor, ok, err := s.donors.Get(ctx, c.BaseURL)
    if err != nil {
        log.WarnContext(ctx, "donor stage failed", "stage", "load app password", "url", c.BaseURL, "err", err)
        res.Status = "failed: load app password: " + truncReason(err.Error())
        return res
    }
    ...
}`
- **Влияние:** If ctx.Err() != nil (context cancelled), but the function call completes (e.g., s.donors.Get() returns with no error, just returns ok=false), placeOne() continues to the next stage. There is no early return on ctx.Err(). The function executes speculatively even after cancellation, wasting resources. If a downstream operation fails due to cancelled context, it's logged as a transient error (e.g., 'failed: llm: ...') rather than 'cancelled', making it indistinguishable from real failures.
- **Фикс:** Add early checks: after each stage, if ctx.Err() != nil { res.Status = 'cancelled'; return res }. Or wrap in a helper: if err := ctx.Err(); err != nil { res.Status = 'cancelled: ' + err.Error(); return res }.
- **Проверка:** The code at lines 223-268 does lack early context cancellation checks, and `placeOne()` will continue executing through all stages if context is cancelled. However, the impact is mitigated: the caller `placeAll()` checks `ctx.Err()` both before (line 191) and after (line 203) calling `placeOne()`, so the loop will break after the current function completes. The actual issues are: (1) Cancellation errors get logged as stage failures rather than distinctly, and (2) Wasted execution of the entire `placeOne()` pipeline, but NOT masked failures per se since the caller still detects cancellation. The severity should be 'low' (code cleanliness/efficiency) rather than 'medium' (security/correctness).

#### 28. 🟡 Goroutine leak in qualifyAll: if context times out before all sites are queued, remaining sites are never processed and workers block indefinitely
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** leak
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:127-147`
- **Код:** `go func() {
    for _, w := range sites {
        select {
        case <-ctx.Done():
            wg.Wait()
            close(resultsCh)
            return
        case sem <- struct{}{}:
        }
        wg.Add(1)
        go func(w domain.Website) { ... }(w)
    }
    wg.Wait()
    close(resultsCh)
}()`
- **Влияние:** If ctx is cancelled (e.g., HTTP handler timeout) while the launcher loop is iterating sites, the select at line 129 detects ctx.Done() and waits for all started workers (wg.Wait()). However, this only waits for goroutines that have already been launched (wg.Add() called). If the launcher is slow and 100 sites exist but only 10 have been wg.Add()'d, wg.Wait() returns after those 10 finish, then close(resultsCh) is called. But the main goroutine (which called Go()) may still be consuming from resultsCh, not knowing the sender closed it. If the consumer loop tries to read after close, it gets a zero value but no error (channels are safe to read from after close), so it appends empty Result{} to the batch and writes that to the sheet. This corrupts the sheet with empty rows.
- **Фикс:** Decouple context cancellation from the loop: use a separate done channel and close it after all cleanup. Or: track which sites have been wg.Add()'d and ensure only those are waited on. Or: rethink the design—do not use wg.Wait() in the launcher, use it only in the receiver.
- **Проверка:** The code at lines 127-147 does NOT cause sheet corruption or goroutine leaks. The launcher loop terminates when ctx.Done() fires (line 130), calls wg.Wait() (line 131) to wait only for already-started workers, then closes resultsCh (line 132). The consumer loop (line 163) is a for-range that exits cleanly when the channel closes—it does not read zero values or append empty Results after closure. In-flight workers are correctly waited on and their real results are consumed. The only design consideration is whether you want in-flight workers to stop when context is cancelled, but the current code intentionally allows them to complete. This is not a corruption or leak—it's a reasonable cancellation strategy.


### infrastructure/http  (12)

#### 29. 🔴 Race condition in Problem.ext map access without synchronization
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/problem/problem.go:39-42, 52, 59-60`
- **Код:** `if p.ext == nil {
	p.ext = make(map[string]any)
}
p.ext[key] = value
...
for k, v := range p.ext {
	merged[k] = v
}`
- **Влияние:** The Problem struct is mutated by With() and MarshalJSON() without synchronization. If a Problem is shared across goroutines (e.g., reused error response in middleware), concurrent reads during MarshalJSON and concurrent writes via With() cause a race. The map iteration in MarshalJSON is unsafe.
- **Фикс:** Add sync.Mutex to Problem struct or document that Problem is not thread-safe and must not be shared. Better: make ext immutable—return a new Problem from With() instead of mutating in-place.
- **Проверка:** The race condition is real. The Problem struct mutates p.ext (a map[string]any) in-place via With() on lines 39-42, and reads it unsynchronized in MarshalJSON() on lines 52 and 59-60. Go maps are not thread-safe; concurrent With() and MarshalJSON() calls will trigger a data race. Line 59 `for k, v := range p.ext` iterating a map while another goroutine modifies it (line 42 `p.ext[key] = value`) is a documented race. Severity remains high because Problem could be shared in middleware contexts where concurrent serialization and extension happen. The code provides no thread-safety documentation or guarantees, creating a footgun for users who reuse Problem instances.

#### 30. 🟠 Response write errors silently swallowed with blank assignment
- **Severity:** high → medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/docs.go:37, 43, 49`
- **Код:** `_, _ = w.Write(b)
_, _ = w.Write([]byte(swaggerUIHTML))
_, _ = w.Write([]byte(landingPageHTML))`
- **Влияние:** If w.Write() fails (network disconnect, client closes early), the error is silently ignored. The client receives incomplete/corrupted response with no error logging. This violates observable behavior expectations and may cause clients to hang waiting for complete data.
- **Фикс:** Check write errors and log them: if _, err := w.Write(b); err != nil { log := logger.New(ctx, "docs"); log.Error().Err(err).Msg("write response failed") }
- **Проверка:** Code at lines 37, 43, and 49 confirms the finding: three HTTP handler functions use `_, _ = w.Write(...)` to silence write errors. Line 37 is in handleOpenAPISpec, line 43 in handleSwaggerUI, and line 49 in handleLandingPage. After headers are set, errors cannot be communicated via HTTP status codes. The finding is accurate but severity should be medium not high: while unlogged write failures are problematic, clients won't "hang" — TCP/HTTP timeouts will occur naturally. Error logging is a valid mitigation, but this is less critical than security bugs or unhandled panics.

#### 31. 🟠 JSON encoding errors silently swallowed in response and problem packages
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/response/response.go:14, 16-17`
- **Код:** `if err := json.NewEncoder(w).Encode(body); err != nil {
	log.Error().Err(err).Msg("encode response failed")
}`
- **Влияние:** Error is logged but execution continues silently; client receives partial/malformed JSON response. More critically, in problem.go line 68, json.Encode error is completely ignored with '_' blank assignment. For HTTP error responses, failing to write the response body leaves the client without error details.
- **Фикс:** Log and either: (a) close the connection, (b) write a fallback error response if headers not sent yet, or (c) return early. For problem.WriteTo(), at minimum log the error before the blank assignment.
- **Проверка:** The finding correctly identifies inadequate error handling for JSON encoding failures. However, response.go does NOT silently swallow errors—it logs them (line 16). The real issue is that logging is insufficient after WriteHeader() is called, since the HTTP status cannot be changed. problem.go line 68 is worse, completely ignoring the error with blank assignment. The core problem is real: clients receive incomplete responses without error details when JSON encoding fails. But the response.go characterization as "silently swallowed" overstates the severity—the error IS logged. A fair severity is medium (real gap, but limited operational impact beyond malformed response bodies).

#### 32. 🟠 Handler nil checks are inconsistent and incomplete
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** footgun
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/linkbuilding.go:50-52`
- **Код:** `if isNil(h.svc) {
	problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
	return
}`
- **Влияние:** LinkbuildingHandler uses custom isNil() (line 19-25) for nil checking, while WordpressSitesHandler, ArticlesHandler, ApiTokensHandler use direct nil checks (h.svc == nil, h.sites == nil). The custom isNil() using reflect.ValueOf is slower and less idiomatic. LoginHandler doesn't check at all (line 34).
- **Фикс:** Use consistent nil checks across all handlers: if h.svc == nil { ... }. Remove the isNil() helper function entirely as it's unmaintainable and adds no value over direct nil comparison.
- **Проверка:** The finding correctly identifies that linkbuilding.go uses a custom isNil() helper with reflect.ValueOf() while other handlers (auth.go, wordpress_sites.go, articles.go, apitokens.go) use direct nil checks like `h.svc == nil`. This is a real inconsistency. However, the finding exaggerates by claiming LoginHandler doesn't check at all - it actually has a proper direct nil check at line 34 (`if h.auth == nil`). The severity is overestimated: while the style inconsistency exists and the reflection-based approach is less idiomatic, it's not a high-severity footgun. The custom isNil() function works correctly and only appears in one file. The actual issue is minor maintenance inconsistency, not a functional problem.

#### 33. 🟠 Problem.With() panics on reserved keys but is public API
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** footgun
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/problem/problem.go:34-37`
- **Код:** `func (p *Problem) With(key string, value any) *Problem {
	switch key {
	case "type", "title", "status", "detail", "instance":
		panic(fmt.Sprintf("problem: extension key %q must not redefine a core RFC 9457 member", key))`
- **Влияние:** With() panics on invalid input, but it's a public method. If called with untrusted input (e.g., from user-provided error context), it causes server crash, not a controlled HTTP error response. Panics in handlers are caught by middleware but this is a poor API design.
- **Фикс:** Return an error instead: func (p *Problem) With(key string, value any) (*Problem, error). Callers must then handle validation, or add a compile-time check. Alternatively, validate at call sites before invoking With().
- **Проверка:** The code is correctly cited. Lines 34-37 of /Users/user/work/multiagent-seo/backend/internal/infrastructure/http/problem/problem.go define a public With(key string, value any) *Problem method that unconditionally panics when key is one of the reserved RFC 9457 members ("type", "title", "status", "detail", "instance"). The method has no error return and is public (exported). However, the severity is appropriately rated as medium (not high) because: (1) the panic is intentional design to prevent RFC 9457 violations, not accidental behavior; (2) the invalid inputs (reserved key names) are known at development time and not arbitrary untrusted input; (3) the method appears unused in the actual codebase based on grep searches, suggesting it may be dead code or a planned API. The API design criticism is valid (public methods should return errors rather than panic), but the practical risk is lower than a function that panics on arbitrary untrusted input.

#### 34. 🟡 Missing maxBodyBytes definition causes undefined variable for token body decoding
- **Severity:** critical → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/loglevel.go:20`
- **Код:** `if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {`
- **Влияние:** While this handler explicitly uses 1<<10 (1 KiB), the absence of a shared constant across all handlers creates inconsistency and maintenance burden. Different handlers have different undocumented limits with no rationale.
- **Фикс:** Define a shared constant file with documented body size limits per endpoint type (e.g., maxBodyBytesSmall, maxBodyBytesMedium) based on expected payload sizes, and use consistently across all handlers.
- **Проверка:** At line 20 of loglevel.go, the code uses http.MaxBytesReader(w, r.Body, 1<<10) with a hardcoded literal 1<<10 (1024 bytes). There is no undefined variable — the code is syntactically correct. The finding's title claiming "undefined variable for token body decoding" is inaccurate. The actual legitimate concern is inconsistency: this handler uses a 1 KiB limit while other handlers in the handlers subpackage use 64 KiB (from maxBodyBytes constant in wordpress_sites.go). However, this inconsistency is intentional (small log-level request vs large data payloads elsewhere) and reasonable, not a critical bug. The severity should be low because: (1) no undefined variable exists, (2) the hardcoded limit is appropriate for a small JSON payload with just a "level" string field, and (3) the code compiles and runs successfully.

#### 35. 🟡 LoginHandler missing nil check for auth service
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/auth.go:33-37`
- **Код:** `func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		problem.Write(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}`
- **Влияние:** LoginHandler correctly checks h.auth in Login() but this check is already present. However, ListUsers() (line 64) also checks it. The pattern is correct but if someone adds a new handler method, they may forget this check. No compile-time guarantee enforces it.
- **Фикс:** Document the pattern or consider wrapping service access in a helper method like h.requireService() to centralize and make the nil check mandatory.
- **Проверка:** The nil check for h.auth is present and correctly implemented in both Login() (lines 34-37) and ListUsers() (lines 65-68). The code does exactly what the evidence shows. The concern is not about a missing check in current code, but about the lack of a compile-time pattern to enforce the check in future handler methods. This is a valid code quality observation (suggesting a helper method wrapper) but labeling it as a "missing" check with "medium" severity is misleading since the checks already exist and are functional. The fair severity is low — it's a preventive suggestion, not an actual bug. The code is currently correct.

#### 36. 🟡 Problem.WriteTo() header order is correct but Content-Type charset missing
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/problem/problem.go:65-69`
- **Код:** `func (p *Problem) WriteTo(w http.ResponseWriter) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)`
- **Влияние:** Content-Type is set to 'application/problem+json' without charset parameter. While Go's json.NewEncoder defaults to UTF-8, RFC 9457 recommends including 'charset=utf-8' for explicitness. Response.WriteJSON correctly does this for 'application/json; charset=utf-8'.
- **Фикс:** Use contentType = "application/problem+json; charset=utf-8" (update const at line 9).
- **Проверка:** The finding correctly identifies that Problem.WriteTo() at line 66 sets Content-Type to "application/problem+json" without charset parameter. The code does indeed call w.Header().Set("Content-Type", contentType) where contentType is defined as "application/problem+json" at line 9. However, the finding's evidence claim about Response.WriteJSON is factually incorrect — WriteJSON() at line 12 of response/response.go sets only "application/json" without charset, not "application/json; charset=utf-8" as the finding claims. The actual issue is real (missing charset), but the severity is overstated because: (1) Go's json.Encoder defaults to UTF-8 encoding, (2) the inconsistency exists elsewhere in the codebase too (WriteJSON doesn't include charset), and (3) RFC 9457 doesn't mandate charset for problem+json, it merely recommends it for clarity. This is a minor consistency improvement, not a functional problem.

#### 37. 🟡 Bearer token middleware context value key collision risk
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/router.go:64-68`
- **Код:** `func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), oapigen.BearerAuthScopes, []string{})
		next.ServeHTTP(w, r.WithContext(ctx))`
- **Влияние:** Uses oapigen.BearerAuthScopes (generated code key) directly. If oapigen is regenerated or changes, the key reference breaks silently. No type safety. Also, the empty []string{} value is a sentinel meaning 'auth required' — this is implicit and fragile.
- **Фикс:** Define a custom type for the context key in the http package (not generated code) and document the meaning. Use a named constant or unexported type to prevent collisions: var ctxKeyAuthRequired = struct{}{}.
- **Проверка:** The code at lines 64-68 of router.go correctly calls context.WithValue(r.Context(), oapigen.BearerAuthScopes, []string{}), where oapigen.BearerAuthScopes is a typed constant of type bearerAuthContextKey defined in generated api.gen.go. The value is checked in auth.go line 17 via type assertion. While using a generated package constant as a context key is not ideal, the collision risk is low because: (1) it's a typed constant, not a raw string, providing type safety; (2) oapi-codegen regeneration is unlikely to break this constant; (3) the code functions correctly. The real issue is code clarity and the implicit sentinel pattern (empty slice means "auth required"), not a live collision bug. Adjusted severity from medium to low.

#### 38. 🟡 Sentry scope enhancement doesn't handle missing context keys gracefully
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/middleware/sentry.go:31-32`
- **Код:** `hub.Scope().SetTag("trace_id", stringFromCtx(ctx, logger.ContextKeyTraceID))
hub.Scope().SetTag("span_id", stringFromCtx(ctx, logger.ContextKeySpanID))`
- **Влияние:** stringFromCtx() returns empty string if key not found (silent degradation). If RequestLogger middleware doesn't run before SentryScopeEnhancer (e.g., due to middleware ordering), trace_id and span_id will always be empty strings in Sentry. No error/warning about missing context.
- **Фикс:** Log a debug message in stringFromCtx() if value is empty, or enforce middleware order with comments/documentation. Better: assert context keys are set in RequestLogger and fail fast if not.
- **Проверка:** The code at sentry.go lines 31-32 calls stringFromCtx() which returns empty string on missing keys. However, the finding overstates the actual risk: RequestLogger middleware (infrastructure/http/router.go line 34) DOES run before SentryScopeEnhancer (line 35), so the context keys ARE set correctly. The stringFromCtx() function is at context.go lines 9-12 and returns `v, _` from a type assertion with no error handling. The real issue is architectural fragility—the code silently depends on middleware ordering without enforcement—but the current ordering in router.go is correct, so trace_id and span_id will not be empty in practice. The risk is valid only if someone reorders the middleware, which would be a configuration mistake, not a code bug.

#### 39. 🟡 LogLevel endpoint overrides status code for validation errors
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/loglevel.go:24-25`
- **Код:** `if err := logger.SetLevel(body.Level); err != nil {
	problem.Write(w, http.StatusBadRequest, err.Error())`
- **Влияние:** If logger.SetLevel returns any error, it's written as-is (err.Error()) into the detail field. If the error message is too long or contains control characters, it may cause issues. The error message is exposed directly to clients without sanitization.
- **Фикс:** Validate body.Level is a known level before calling SetLevel, or define a controlled set of errors and map them to user-friendly messages.
- **Проверка:** At lines 24-25, the code calls `logger.SetLevel(body.Level)` and writes any error via `problem.Write(w, http.StatusBadRequest, err.Error())`. The error comes from `ParseLevel()` in logger.go which only returns `fmt.Errorf("unknown log level %q", level)` — the error message is generated with a safe format string using `%q` escaping, not arbitrary user input. The error message format is well-controlled and will always be "unknown log level \"<value>\"". While exposing the invalid level to the client is defensible API behavior, the finding overstates the risk by claiming lack of sanitization and potential control character issues when the error format is actually safe.

#### 40. ⚪ uuid.Nil comparisons in handlers are verbose and fragile
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** duplication
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/articles.go:51-53, 82-84`
- **Код:** `if body.SiteId == uuid.Nil {
	problem.Write(w, http.StatusBadRequest, "site_id")
	return
}
...
if a.SiteID != uuid.Nil {`
- **Влияние:** uuid.Nil comparisons repeated throughout handlers. No helper function to encapsulate validation logic. If uuid zero value changes (unlikely but possible in API evolution), all call sites break.
- **Фикс:** Define helper validators: func isValidUUID(id uuid.UUID) bool { return id != uuid.Nil } and use consistently.
- **Проверка:** The code does contain uuid.Nil comparisons at the cited lines, but the finding mischaracterizes their purpose and severity. Lines 51-53 validate that a required input field is present (rejecting uuid.Nil). Lines 82-84 and 185-187 (also found in toArticle) conditionally include optional fields in responses when they're not the zero value. These serve fundamentally different purposes: reject vs. conditionally include. A single helper validator would be misleading because the logic needs inversion in different contexts. The comparisons are explicit and clear, not fragile—uuid zero values are stable in Go. This is a case of mild code duplication with a pattern that's simple enough that abstraction adds more confusion than benefit. Not a real issue worth fixing.


### persistence/postgres + db  (11)

#### 41. 🟠 Pool creation uses cancellable context - connection may fail to initialize on shutdown
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** correctness
- **Локация:** `/Users/user/work/multiagent-seo/backend/cmd/server/main.go:69, 82`
- **Код:** `ctx, stop := signal.NotifyContext(...); pool, err := db.NewPool(ctx, cfg.Database)`
- **Влияние:** If SIGINT/SIGTERM arrives during pool initialization (lines 82-86), the context is cancelled and pgxpool.New() will fail even if the database is available. This causes graceful startup to fail unpredictably.
- **Фикс:** Create pool with a background context, not the cancellable signal context. Use context.Background() for initialization: pool, err := db.NewPool(context.Background(), cfg.Database)
- **Проверка:** The code at lines 69 and 82 correctly shows the pool being created with a signal-aware context (created at line 69 with signal.NotifyContext, passed to db.NewPool at line 82). The db.NewPool function does use this context for pgxpool.New() and pool.Ping() at lines 16-21 of pool.go, making both synchronous operations cancellable. However, the practical risk is lower than "high" severity: (1) the startup window is brief, making signal arrival unlikely; (2) the error is already handled gracefully at lines 83-84 with a warning log; (3) the app continues functioning without DB services rather than crashing. The fix (using context.Background() for init) is valid best practice, but the real-world impact is moderate, not critical.

#### 42. 🟠 DSN construction allows empty port - results in malformed connection string
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/config/config.go:41-52`
- **Код:** `Host:   c.Host + ":" + c.Port, // blindly concatenates, no validation`
- **Влияние:** If Port is empty string (though envDefault is set, dynamic override could miss it), resulting DSN will have host as 'localhost:' which pgx will parse incorrectly. No validation of Port format occurs.
- **Фикс:** Validate Port is non-empty and numeric in DatabaseConfig before use. Add validation in Load(): if cfg.Database.Port == "" || strings.TrimSpace(cfg.Database.Port) == "" { return cfg, fmt.Errorf(...) }
- **Проверка:** The code at lines 41-52 does concatenate Host and Port without local validation in the DSN() method. However, the DatabaseConfig.Port field has envDefault:"5432" and validate:"required" tags (line 33), and Load() applies struct validation before any config is used (lines 227-229). An empty Port would only result from: (1) bypassing the validator, or (2) directly constructing DatabaseConfig without Load(). The finding correctly identifies the concatenation pattern but overstates the risk by not accounting for the validate:"required" constraint that guards this field in normal usage. Fair severity is medium—real issue if validation is bypassed, but not a high-severity bug in the normal code path.

#### 43. 🟠 Missing nil pool check before repository creation leads to nil pointer dereference
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/cmd/server/main.go:82-106`
- **Код:** `pool, err := db.NewPool(...); if err != nil { ... } else { defer pool.Close(); healthRepo = ... }`
- **Влияние:** If pool creation fails, all repositories (WordpressSiteRepository, UserRepository, etc.) remain nil but are used in handlers. When handlers call methods on nil repositories, they panic. The error-handling branch at line 84 only logs a warning but doesn't prevent handlers from executing.
- **Фикс:** Check pool != nil before passing to repositories. Wrap all handler initialization in 'if pool != nil' blocks or return early from main if pool creation fails in non-dev environments.
- **Проверка:** The code does allow repositories to be nil when pool creation fails (lines 82-106), but this is not a critical nil pointer dereference bug. The health service explicitly checks for nil repo at /Users/user/work/multiagent-seo/backend/internal/domain/health/service.go lines 13-16 and returns DBStatusUnknown. All handler endpoints (ArticlesHandler, AuthHandler, etc.) include explicit nil checks via the unavailable() pattern (articles.go line 129-135) or nilableX wrapper functions, returning HTTP 503 (ServiceUnavailable) when the service is nil. The app degrades gracefully to a degraded state rather than panicking. Severity should be medium as a design issue (incomplete graceful degradation) rather than high/critical.

#### 44. 🟠 Article.exec() silently treats 'no rows affected' as ErrNotFound without verifying existence first
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** logic-bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/article_repository.go:176-185`
- **Код:** `if tag.RowsAffected() == 0 { return articles.ErrNotFound }`
- **Влияние:** When UpdateDraft/MarkFailed/MarkPublished/SaveImageStats fail due to concurrent deletion, the error is indistinguishable from the article never existing. Callers cannot tell if the article was deleted or never found. For critical operations like publishing, this causes silent failures.
- **Фикс:** Verify article exists before updating: Add a preliminary SELECT to check existence, or add a WHERE clause that returns CASE to differentiate (e.g., 'RETURNING id, EXISTS(SELECT 1 FROM articles WHERE id=@id)' to detect if row was soft-deleted).
- **Проверка:** The code at lines 176-185 does return articles.ErrNotFound when RowsAffected() == 0, creating the semantic ambiguity described. However, the impact is less severe than claimed: MarkPublished() errors (line 420 in service.go) ARE treated as fatal and properly fail the publish operation with user-visible error logging. Other methods like SaveImageStats() (line 322) only log warnings and don't fail the pipeline. The UPDATE statements have WHERE id = @id without soft-delete logic, so 0 rows affected could legitimately mean either the article was deleted or never existed. The real issue is semantic ambiguity in error reporting rather than silent failures in critical paths, since the publish operation does fail loudly when MarkPublished returns an error.

#### 45. 🟠 WordPress site repository Credentials() doesn't distinguish between 'site disabled' and 'site not found'
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/wordpress_site_repository.go:137-152`
- **Код:** `SELECT ... WHERE id = @id AND deleted_at IS NULL AND enabled = true; ... if errors.Is(err, pgx.ErrNoRows) { return ..., wordpress.ErrNotFound }`
- **Влияние:** Callers cannot distinguish between 'site doesn't exist' and 'site exists but is disabled'. This forces callers to always treat disabled sites as missing, preventing audit/debugging of why a site is unavailable.
- **Фикс:** Define separate error types: ErrSiteDisabled and ErrSiteNotFound. Query only by id and deleted_at, then check enabled in application layer or return enabled status as part of Credentials.
- **Проверка:** The code at lines 137-152 in wordpress_site_repository.go contains a SQL query with three WHERE conditions (id = @id AND deleted_at IS NULL AND enabled = true), but returns the same wordpress.ErrNotFound for all three failure cases. When pgx.ErrNoRows occurs, callers cannot distinguish whether the site doesn't exist, is soft-deleted, or is disabled. The provider.go code at lines 23-29 shows a real caller that wraps the error without special handling, making it impossible to debug why credentials are unavailable. The test suite lacks coverage for the disabled-site scenario, masking this issue. The finding is accurate and the severity of 'medium' is fair — it prevents proper error context and debugging, though it doesn't cause data loss.

#### 46. 🟡 Pool initialization logs without proper error handling, hard-codes 'localhost' default allowing misconfiguration
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/db/pool.go:13-28`
- **Код:** `log.Info().Str("host", cfg.Host).Str("dbname", cfg.Dbname).Msg(...) at line 26 - logs after successful Ping, but cfg.Host may be redacted/unsafe to log if it's a secret.`
- **Влияние:** Sensitive database host information may be logged to observability systems. If the host is actually a secret (e.g., RDS endpoint with auth token), it gets exposed in logs.
- **Фикс:** Don't log cfg.Host or cfg.Dbname directly. Log only sanitized/safe identifiers. Alternatively, use a log redaction library or middleware.
- **Проверка:** The code at line 26 logs cfg.Host and cfg.Dbname after a successful database connection. These fields (DatabaseConfig.Host and DatabaseConfig.Dbname from config.go lines 32 and 36) are infrastructure identifiers with defaults "localhost" and "contentflow" respectively—not secret values. The actual sensitive field, cfg.Password, is correctly NOT logged. Logging host and database name for a successful connection is a standard practice. While a defense-in-depth policy could justify not logging ANY config values, characterizing this as exposing "sensitive database host information" is overstated when the Password field is excluded.

#### 47. 🟡 Encryption key stored in plain-text config struct with 'dev-insecure-change-me' fallback
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/config/config.go:193-196`
- **Код:** `const devEncryptionKey = "dev-insecure-change-me"; type WordPressConfig struct { EncryptionKey string ˗env:"WP_ENCRYPTION_KEY" envDefault:"dev-insecure-change-me"˗ }`
- **Влияние:** EncryptionKey is stored in plaintext in memory and could be dumped via profiling tools, heap inspection, or panics. The hardcoded dev default is weak and easily guessed. Encrypted credentials are only as secure as the key.
- **Фикс:** Read EncryptionKey from separate secure config (HashiCorp Vault, AWS Secrets Manager, .env file with restricted permissions). Never embed in source or commit defaults. Consider rotating key without re-encryption (use versioned keys).
- **Проверка:** Code at lines 193-197 does have a hardcoded insecure default ("dev-insecure-change-me") in both the const and struct tag. However, the finding overstates the risk: lines 231-238 implement runtime validation that explicitly rejects this default value in non-local environments, returning an error if Sentry.Environment != "local" and the key still equals devEncryptionKey. This prevents the vulnerability in production. The plaintext in-memory storage is standard Go config practice. The actual vulnerability is low because the code has explicit runtime checks preventing misuse outside development.

#### 48. 🟡 ArticleRepository.List() doesn't pre-allocate slice capacity, causing repeated allocations
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** perf
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/article_repository.go:95-101`
- **Код:** `list := make([]articles.Article, 0); for rows.Next() { list = append(list, a) }`
- **Влияние:** For large article counts (1000+), repeated slice appends trigger O(n) reallocations. Minor performance regression under load.
- **Фикс:** Count rows first or estimate capacity: count rows with COUNT(*), or use list := make([]articles.Article, 0, 100) if a reasonable upper bound is known. Alternatively, use a buffer pool.
- **Проверка:** The code at lines 95-101 does create a slice with zero capacity and appends in a loop, which is inefficient. However, the impact description is technically misleading. Go uses exponential slice growth (roughly doubling), resulting in logarithmic reallocations (not O(n))—for 1000 items you'd see ~10 reallocations, not thousands. The finding is real but the performance impact is much smaller than described. Severity remains low since this only matters for very large result sets (10k+), and even then the overhead is modest. Fair severity: low.

#### 49. 🟡 User repository List() doesn't check rows.Err() before returning
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/user_repository.go:39-56`
- **Код:** `var out []user.User; for rows.Next() { ... }; return out, rows.Err()`
- **Влияние:** Inconsistency: ArticleRepository checks rows.Err() after loop (line 103), but UserRepository does not check until return. If iteration is interrupted (OOM, context cancel), UserRepository returns partial results silently.
- **Фикс:** Move rows.Err() check inside the loop or immediately after, not at return: if err := rows.Err(); err != nil { return nil, fmt.Errorf(...) }
- **Проверка:** UserRepository.List() at line 55 does call rows.Err() and returns it, so it IS checking for errors. However, it returns the raw error without wrapping in an error message like ArticleRepository does (line 104: fmt.Errorf("list articles: %w", err)). The real issue is inconsistent error context/wrapping, not a missing check. Neither implementation silently returns partial results—both will return the error if iteration fails. The finding conflates a style inconsistency with a functional bug.

#### 50. 🟡 ApiTokenRepository.ListByUser() pre-allocates zero-capacity slice, triggering repeated allocations
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** perf
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/apitoken_repository.go:42-64`
- **Код:** `var out []apitoken.Token; for rows.Next() { ... out = append(out, t) }`
- **Влияние:** Same as ArticleRepository: repeated slice growth for users with many tokens (50+). Minor allocation overhead.
- **Фикс:** Count tokens first, or initialize with estimated capacity: out := make([]apitoken.Token, 0, 10)
- **Проверка:** Lines 42-64 confirmed: var out []apitoken.Token (line 55, zero-capacity nil slice) followed by out = append(out, t) in loop (line 61). This causes repeated allocations as the slice grows. The finding accurately describes the code pattern. Severity remains low — real but only impactful for users with 50+ tokens; typical users see minimal overhead.

#### 51. 🟡 Test seeds password hash as plaintext hex string instead of valid bcrypt hash
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/persistence/postgres/user_repository_integration_test.go:24`
- **Код:** `const hash = "$2a$10$abcdefghijklmnopqrstuv" // truncated/invalid bcrypt`
- **Влияние:** Test hash is syntactically invalid bcrypt (too short, fake payload). If test is copy-pasted to seed real data, auth will fail silently. Does not test actual password verification flow.
- **Фикс:** Use real bcrypt hash: hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.DefaultCost); use hash in both tests and docs.
- **Проверка:** The test hash "$2a$10$abcdefghijklmnopqrstuv" (24 chars) is indeed syntactically invalid bcrypt (valid hashes are 60 chars). However, the test at TestUserRepository_FindByEmail only verifies that the repository retrieves data correctly (lines 32-40 call FindByEmail and assert Email and PasswordHash match). It never calls bcrypt.CompareHashAndPassword, so the invalid hash doesn't affect test logic. The reviewer's concern about "copy-pasted to seed real data" and "auth will fail silently" overstates the risk—this is test-isolated data, not a logic bug. The actual minor issue is that this test uses fake credentials that wouldn't work in authentication tests, making it poor practice for documentation or reuse. Severity should be low (test quality), not medium.


### llm / topicclassifier / checker  (14)

#### 52. 🟠 Groq vs Claude: Asymmetric maxTokens Handling
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/groq/client.go:31-40`
- **Код:** `Groq: if maxTokens > 0 { reqBody["max_tokens"] = maxTokens }
Claude (claude/client.go:35-36): if maxTokens <= 0 { maxTokens = defaultMaxTokens }`
- **Влияние:** Groq omits max_tokens if maxTokens ≤ 0, letting the API choose. Claude always sends max_tokens, defaulting to 4096. This means Groq is unbounded by default (API decides), but Claude is capped. Callers passing maxTokens=0 get different behavior. Inconsistent contract violates the principle of least surprise.
- **Фикс:** Either: (1) both providers default to the same strategy (both hardcode defaults, or both omit), or (2) document that maxTokens=0 has different semantics per provider. Recommend: apply Claude's strategy to Groq—default to 4096, ensuring consistent behavior.
- **Проверка:** Groq's BuildRequest omits max_tokens from the request body when maxTokens <= 0 (line 38-40), allowing the Groq API to set its own default. Claude's BuildRequest always includes max_tokens, defaulting to 4096 when maxTokens <= 0 (lines 35-40). This creates an asymmetric contract where callers get different effective behavior across providers for the same input—Groq unbounded vs Claude capped at 4096. The finding accurately describes the code behavior and the inconsistency is real.

#### 53. 🟠 Huggingface: Ignored Read Error on Response Body
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/checker/huggingface/client.go:128`
- **Код:** `respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`
- **Влияние:** io.ReadAll error is silently ignored. If network fails mid-read, respBody is a partial buffer and json.Unmarshal will fail downstream, but with a misleading error (decode failure, not read failure). Logs don't capture the actual error. Debugging is harder. On 503 cold-start retry, if the read fails, we lose the original error context.
- **Фикс:** Capture the error: respBody, readErr := io.ReadAll(...). Check readErr before JSON decode, log it explicitly, or treat read error same as non-200 status.
- **Проверка:** Line 128 does ignore the io.ReadAll error: respBody, _ := io.ReadAll(...). The error is discarded via the blank identifier. If a network read fails mid-transfer, respBody becomes a partial buffer, and subsequent json.Unmarshal (line 146) fails with a decode error that masks the underlying I/O failure. The original read error is never logged or examined, making debugging harder. However, the impact is somewhat mitigated by: (1) the 1MB limit cap preventing unbounded reads, (2) typical small API response bodies, and (3) the response body being included in error messages for some debugging context. The finding's description is accurate; medium severity is fair (though some might argue low if weight is placed on practical rarity of this failure mode in HTTP API calls).

#### 54. 🟠 LLM Retry: Timing-Sensitive Test (TestDo_HonorsRetryAfterOverBackoff)
- **Severity:** low → medium  ·  **Verdict:** exaggerated  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/retry/retry_test.go:53-67`
- **Код:** `if elapsed := time.Since(start); elapsed < 25*time.Millisecond { t.Errorf(...) }`
- **Влияние:** Test expects ~30ms Retry-After delay but only checks that elapsed >= 25ms. On slow CI, system pause, or scheduler delay, elapsed could be 24ms and pass falsely, or 35ms+ and fail (flaky). No upper bound, so system hiccup fails the test. This reduces confidence in retry behavior validation.
- **Фикс:** Widen tolerances: elapsed >= 25ms && elapsed <= 50ms. Or mock time.After to remove timing dependency entirely (use a deterministic fake clock).
- **Проверка:** The test at lines 53-67 does perform a timing check: it records time.Now() before calling Do(), which internally calls time.After(30*time.Millisecond) when handling the retry-after header, then verifies elapsed >= 25ms. The finding correctly identifies this as timing-sensitive. However, severity is overstated: the 5ms tolerance (25-30ms window) is reasonable for system variance, and the concern about falsely passing at 24ms is theoretical. The real, valid criticism is that there's no upper bound check—a system hiccup causing 100ms+ elapsed time would still pass. This is a legitimate concern for flakiness on overloaded CI, making it medium severity (not low as stated). The fix suggestion (add elapsed <= 50ms upper bound, or mock time.After) is sound and would improve the test.

#### 55. 🟠 Huggingface: Goroutine Failure Not Blocking Caller
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/checker/huggingface/client.go:191-209`
- **Код:** `for i, s := range sentences {
  wg.Add(1)
  sem <- struct{}{}
  go func(i int, s string) {
    ...
    score, err := c.score(ctx, s)
    if err != nil {
      failures.Add(1)
      return  // <- fails silently
    }
    results[i] = scored{text: s, score: score}
  }(i, s)
}
wg.Wait()
if n := failures.Load(); n > 0 {
  c.log.WarnContext(ctx, "huggingface sentence scoring failed for some sentences", "failed", n, "total", len(sentences))
}`
- **Влияние:** If a goroutine fails to score a sentence, results[i] remains uninitialized (zero struct: text="", score=0.0). Later, the filter loop checks `if r.text != ""` to skip zero structs, but this silently drops ALL failed sentences from the output without returning an error or high-confidence notice. Caller is blissfully unaware. If all sentences fail to score (e.g., HF API down), returns empty flagged list, implying the content is human-written. Silent data loss.
- **Фикс:** Return an error if failures > 0. Or, better: store the error in results[i], check it in the filter, and propagate up. Mark failed results distinctly so they're not silently dropped.
- **Проверка:** The finding accurately describes the mechanism: goroutines fail silently on error, results[i] remains uninitialized (text="", score=0.0), and the filter at line 213 silently excludes failed entries. The code logs warnings but does not propagate error information to the caller. However, the severity is overstated. The core AI detection verdict depends on the full-text aiScore (line 81-89), which is returned successfully when c.score(ctx, input) succeeds. Sentence-level scoring is secondary metadata for flagging specific problem sentences. Silent loss here reduces diagnostic value and sentence-flagging completeness, but does not flip the core "AI-generated" vs "human-written" verdict. Fair severity: medium (partial feature loss + diagnostics gap), not high.

#### 56. 🟠 TopicClassifier: Missing Integration Test with Real LLM
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/topicclassifier/topicclassifier_test.go:21-131`
- **Код:** `All tests use fakeLLM with hardcoded replies; no integration test of actual Classify call with real transport.Client`
- **Влияние:** Tests validate normalization and matching logic but not the end-to-end flow. Real LLM calls may return unexpected formats (e.g., "The topic is Tech" instead of just "Tech"), exceed token budgets (causing truncation), or timeout. The crude maxReplyTokens and topicclassifier.LLM interface hide the transport layer. Problems only surface in production.
- **Фикс:** Add an integration test that instantiates a real (or mock HTTP) transport.Client and verifies Classify returns expected topics. Or at minimum, test buildPrompt output length against actual token counts.
- **Проверка:** The test file (lines 21-131) does use only fakeLLM with hardcoded replies across all five test functions (TestClassify, TestClassifyMultiWordCandidate, TestClassifyMalformedReply, TestClassifyNormalizeRobustness, TestClassifyEmptyCandidates). The tests thoroughly validate the normalize() and matchCandidate() logic but do not test: (1) buildPrompt() output length against token limits, (2) maxReplyTokens() calculation accuracy (crude char/3 heuristic), or (3) end-to-end integration with real transport.Client. The actual instantiation in main.go wraps domainarticles.LLMClient, hiding transport complexity. The gap is real — production responses could be more verbose than hardcoded test cases assume, potentially exceeding token budgets or being truncated. However, the matching/normalization logic (core functionality) is well-tested, so the gap is not critical but should be addressed via an integration test or at minimum a prompt length validation test.

#### 57. 🟠 Retry Logic: maxRetryAfter Hardcoded, Not Configurable
- **Severity:** low → medium  ·  **Verdict:** confirmed  ·  **Category:** complexity
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/retry/retry.go:22`
- **Код:** `const maxRetryAfter = 30 * time.Second`
- **Влияние:** If a provider returns Retry-After: 120s, we cap it to 30s and retry immediately, potentially hitting rate limits again. Config-driven retry backoff (Default()) is hardcoded; operators can't tune it without recompiling. High-traffic deployments may need looser backoffs.
- **Фикс:** Add maxRetryAfter to retry.Config struct, default to 30s. Expose in package config (LLMConfig.RetryMaxWait) and allow override via environment variable. Document the tradeoff: longer waits reduce rate-limit conflicts but increase latency.
- **Проверка:** Line 22 contains const maxRetryAfter = 30 * time.Second. This constant is used at lines 81-82 to cap any Retry-After header value returned by providers — if the provider returns a longer duration, it's truncated to 30s. The retry.Config struct (lines 10-13) has no field for this value, and Default() (lines 15-20) is hardcoded with fixed backoff schedules. Neither transport.go nor any call site can override maxRetryAfter. The finding is accurate: if a provider explicitly requests a 120s wait via Retry-After header, the code will retry after only 30s, potentially hitting rate limits again. Adjusted to medium severity because while this is a real configuration gap in high-traffic scenarios, the 30s cap is a reasonable defensive default for most use cases, and the issue requires both a rate-limited response AND a large Retry-After value to manifest.

#### 58. 🟡 LLM Transport: Missing Usage Metrics on Error Path
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/transport/transport.go:140-157`
- **Код:** `if err != nil {
    logFailure(ctx, "llm call done", ..., "input_tokens", u.InputTokens, "output_tokens", u.OutputTokens, ...)
    return "", usage.Usage{}, fmt.Errorf(...)
  }`
- **Влияние:** When an LLM request fails, usage metrics are logged with zero values (u is uninitialized). This obscures actual API consumption—if parsing fails partway through a successful response, we lose the token counts that determine billing. Monitoring systems can't correlate failures with actual costs.
- **Фикс:** Initialize u to zero, or better: populate it from successful non-OK responses that return error codes (429, 500, etc.) where providers still return usage info. Add a struct field to track partial parses. Document that usage is unreliable on errors.
- **Проверка:** The code does log zero-valued usage metrics on error paths (lines 148-156 log u.InputTokens and u.OutputTokens which are uninitialized at zero). However, the impact is overstated. On error paths (HTTP failures, non-200 status, parse errors), no valid usage data is available — the API doesn't return token counts in error responses. Logging zeros accurately reflects this unavailability rather than obscuring actual costs. The code structure prevents the "parsing fails partway through successful response" scenario the reviewer described — non-200 responses are caught before parsing. This is a logging clarity issue (zeros don't indicate "no data available") rather than a metric loss issue, and can be improved with better documentation or conditional logging, but doesn't constitute data loss for billing correlation as claimed.

#### 59. 🟡 TopicClassifier: Crude Byte-to-Token Approximation
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** complexity
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/topicclassifier/topicclassifier.go:53-65`
- **Код:** `longest := 0
  for _, cand := range candidates { if n := len(cand); n > longest { longest = n } }
  tokens := longest/3 + 8`
- **Влияние:** maxReplyTokens divides candidate string length by 3 to estimate tokens. This assumes 3 bytes per token—wildly inaccurate for UTF-8 text with non-ASCII. A 30-byte Japanese candidate (~10 characters) becomes 18 tokens by this formula but actually uses ~30+ tokens. Results: token budget too low (truncation), LLM requests fail, classifier flakes intermittently on non-English input.
- **Фикс:** Use a proper token counter (e.g., byte-pair encoding library) or conservative heuristic: char-count / 4 + 16. For safety, add headroom: max(minReplyTokens, tokens + margin). Or fetch tokenizer from the LLM's SDK.
- **Проверка:** The code at lines 53-65 divides the longest candidate string length by 3 (+8) to estimate a token budget for the LLM's *response* (not for tokenizing the candidates or prompt). This is a crude heuristic, but not severely problematic: (1) responses are single topic words requiring 1-3 tokens anyway, (2) the minReplyTokens floor of 32 ensures adequate budget, and (3) using byte length as a signal (even if UTF-8 inflates it for non-ASCII) conservatively increases the budget, which is safe. The finding mischaracterizes the function's purpose and overstates the practical impact. The heuristic is crude but acceptable for this narrow use case.

#### 60. 🟡 Claude API Version Out of Date
- **Severity:** medium → low  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/claude/client.go:17`
- **Код:** `const anthropicVersion = "2023-06-01"`
- **Влияние:** Using Anthropic API version 2023-06-01, released ~3 years ago (as of June 2026). Current versions offer bug fixes, performance improvements, and new features (e.g., extended context windows, improved instruction-following). Upgrading could reduce token costs and latency. Using outdated API is risk—vendor may deprecate it without notice.
- **Фикс:** Upgrade to the latest stable API version (e.g., 2024-06-01 or newer). Check Anthropic changelog for breaking changes. Consider parameterizing the version in config.
- **Проверка:** Line 17 correctly identifies anthropicVersion = "2023-06-01" as a constant used in the BuildRequest method (line 53). This is indeed a 3-year-old API version. However, the Anthropic API documentation confirms this version is still supported with backward-compatibility guarantees — new features use optional beta headers rather than requiring API version upgrades. The finding is real and an upgrade would be good practice, but the immediate risk is low since the version isn't currently deprecated/removed. Severity should be "low" rather than "medium" — recommend as a maintenance enhancement rather than a critical issue.

#### 61. 🟡 Factory: No Validation of Provider Aliasing
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** naming
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/factory.go:15-19, 25-32`
- **Код:** `var providers = map[string]func(...) *transport.Client{
  "groq":      groq.New,
  "claude":    claude.New,
  "anthropic": claude.New,  // <- alias
}
name := strings.ToLower(strings.TrimSpace(provider))
if name == "" { return nil, fmt.Errorf(...) }
ctor, ok := providers[name]
if !ok { return nil, fmt.Errorf("unknown provider %q", provider) }`
- **Влияние:** "anthropic" maps to claude.New, but config.go normalizeProvider also handles this separately. Two sources of truth for alias logic. If someone passes "Anthropic" (mixed case), factory.New normalizes to "anthropic" and finds it in providers. But if cfg.LLM.KeyFor("Anthropic") is called elsewhere, it goes through normalizeProvider again. Works by accident but fragile—code is not DRY.
- **Фикс:** Centralize alias resolution: move the "anthropic" -> "claude" mapping to a single normalizeProvider function exported from package llm, call it from both factory.New and config.go. Or use a const to define the canonical name.
- **Проверка:** The code at factory.go:15-19 defines the providers map with both "claude" and "anthropic" keys aliasing to claude.New. At factory.go:25, the New() function normalizes with strings.ToLower(strings.TrimSpace()) but does NOT apply the "anthropic"→"claude" alias conversion—that conversion exists only in config.go's normalizeProvider() at line 128-130. The current code works because the "anthropic" key exists in the providers map (line 18), but the finding is correct that alias logic is duplicated: factory.go handles it via explicit map entry, while config.go handles it via explicit if statement in normalizeProvider(). This is not DRY and creates a maintenance risk—if the alias is removed from one place but not the other, they become inconsistent. No actual functional bug in current code, but the concern about fragile naming logic is valid.

#### 62. 🟡 Checker Adapter: Error Handling Swallows First Error
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/checker/adapter.go:18-48`
- **Код:** `func New(provider, apiKey, model string, threshold float64, log *slog.Logger) (articles.ContentChecker, error) {
  switch provider {
  case "mock", "":
    mock := NewMock(threshold)
    return &adapter{check: func(ctx context.Context, content string) (*articles.CheckResult, error) {
      res, err := mock.Check(ctx, content)
      if err != nil { return nil, err }
      return toCheckResult(...), nil
    }}, nil
  case "huggingface":
    if apiKey == "" { return nil, fmt.Errorf("checker: huggingface provider requires an API key") }
    hf := huggingface.New(...)
    return &adapter{check: func(ctx context.Context, content string) (*articles.CheckResult, error) {
      res, err := hf.Check(ctx, content)
      if err != nil { return nil, err }
      return toCheckResult(...), nil
    }}, nil`
- **Влияние:** NewMock(threshold) and huggingface.New(...) both accept a threshold parameter. But NewMock returns a *MockClient directly; if threshold is invalid or triggers a panic (unlikely here, but possible with future changes), there's no factory validation. Also: NewMock returns a panic if its internal state is corrupt, but the factory doesn't catch it. Compare: cmd/server/main.go newChecker() catches errors and falls back to mock—good defensive programming. But the adapter itself doesn't validate that NewMock and huggingface.New actually succeed or return valid clients.
- **Фикс:** Add validation in adapter.New after creating mock/huggingface clients: verify non-nil, check basic invariants (threshold in [0,1], model non-empty). Or rely on constructors to return errors (change NewMock/huggingface.New signatures).
- **Проверка:** The finding mischaracterizes the constructors. NewMock (checker.go:27-32) and huggingface.New (client.go:51-68) both return valid objects directly without error interfaces. Neither has panic-prone operations. NewMock only adjusts threshold if zero; huggingface.New sets reasonable defaults. The adapter.New factory (adapter.go:18-48) properly validates apiKey before calling constructors and handles provider routing with explicit error cases. main.go's newChecker() demonstrates the intended defensive pattern at the factory level, not the constructor level. The reviewer invents hypothetical panics ("internal state corrupt") that cannot occur based on the actual code. The only legitimate minor point is that constructor validation (e.g., bounds-checking threshold in [0,1]) could be stricter, but the current pattern is intentional: constructors return valid objects, factories return errors. This is a valid design choice, not a bug.

#### 63. 🟡 Transport: Missing HTTP Client Configuration (connection pooling, idle timeout)
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** perf
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/llm/transport/transport.go:44`
- **Код:** `httpClient: &http.Client{Timeout: requestTimeout}`
- **Влияние:** http.Client is created with only a Timeout set. Default http.DefaultTransport is used, which has conservative connection pooling settings (MaxIdleConns=100, MaxIdleConnsPerHost=2). For high concurrency with few LLM endpoints, connection reuse is suboptimal. Also: no explicit DialTimeout, TLSHandshakeTimeout, or ExpectContinueTimeout. If servers are slow on connect, the global request timeout (180s) is enforced but granular timeouts aren't. This can hide network issues.
- **Фикс:** Configure http.Client with a custom Transport:
Transport: &http.Transport{
  MaxIdleConns: 50,
  MaxIdleConnsPerHost: 10,
  DialTimeout: 30 * time.Second,
  TLSHandshakeTimeout: 30 * time.Second,
  IdleConnTimeout: 90 * time.Second,
},
Or use a pool per provider if endpoints differ.
- **Проверка:** The code at line 44 correctly initializes http.Client with only Timeout field set, defaulting to http.DefaultTransport. This is accurate. However, the severity is overstated: MaxIdleConnsPerHost=2 is only suboptimal for high concurrency workloads making many simultaneous requests to the same LLM endpoint. For typical sequential or low-concurrency LLM client usage (the common case), the default transport is adequate. The lack of granular timeouts (DialTimeout, TLSHandshakeTimeout) is a minor observability gap, not a functional problem. Unless production telemetry shows connection establishment as a bottleneck, this is low priority.

#### 64. 🟡 Huggingface: Sentence Splitting Fragile on Edge Cases
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** complexity
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/checker/huggingface/client.go:233-260`
- **Код:** `for i := 0; i < len(runes)-1; i++ {
  r := runes[i]
  if r != '.' && r != '!' && r != '?' { continue }
  if runes[i+1] != ' ' { continue }
  if i+2 >= len(runes) || unicode.IsUpper(runes[i+2]) {
    seg := strings.TrimSpace(string(runes[start : i+1]))
    if len(seg) >= minSentenceChars {
      out = append(out, seg)
    }
    start = i + 2
  }
}`
- **Влияние:** Splits on punctuation + space + uppercase. Breaks on abbreviations ("Dr. Smith"), acronyms, and ellipsis ("..."). "The U.S. is large" splits incorrectly. "What...?" is not a sentence. Numbers with periods are not split (correct by accident). Edge case: final segment without punctuation is kept if len >= 40; but if text ends mid-sentence without period, it's included even though it's incomplete. Fragile heuristic reduces flagged sentence quality.
- **Фикс:** Use a proper sentence tokenizer library (e.g., go-nlp/sentence or similar) or a regex with negative lookahead for known abbreviations. Minimize false splits.
- **Проверка:** The code at lines 233-260 implements a naive sentence splitter that splits on punctuation + space + uppercase letter. The finding accurately identifies real fragility: it will false-split on abbreviations like "Dr. Smith" (because 'S' is uppercase), mishandle ellipsis ("...?"), and struggle with acronyms ("U.S."). The final segment handling (lines 253-258) does append incomplete text >= 40 chars without punctuation. However, the impact is constrained: the minimum sentence length is 40 chars, and this is only used for flagging sentences in SEO content. The severity of "low" is appropriate—this is a legitimate quality issue that would benefit from a proper sentence tokenizer library, but it's not a critical correctness bug.

#### 65. ⚪ TopicClassifier: UTF-8 Truncation Logic Has Edge Case
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** complexity
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/topicclassifier/topicclassifier.go:94-103`
- **Код:** `for end > 0 && s[end]&0xC0 == 0x80 { end-- }`
- **Влияние:** Truncates at UTF-8 continuation byte boundary. Correct in principle—avoids splitting multi-byte chars. But the loop assumes valid UTF-8; if s is corrupt (e.g., has orphaned continuation bytes), it may back up excessively or loop infinitely. If end=0 before finding a safe cut, returns empty string. Go strings are *supposed* to be valid UTF-8, but no validation here.
- **Фикс:** Use utf8.ValidString(s) before truncation, or use RuneCountInString and proper rune slicing: s = s[:strings.SplitN(s, "", limit+1)[0]] or a validated UTF-8 substring function.
- **Проверка:** The trimSample function (lines 94-103) correctly implements UTF-8-safe truncation by backing up from the limit to avoid splitting multi-byte characters. The loop `for end > 0 && s[end]&0xC0 == 0x80` checks if the byte at position `end` is a UTF-8 continuation byte (0x80-0xBF) and backs up until finding a non-continuation byte or reaching end=0. The `end > 0` guard prevents infinite looping. While the code does not validate UTF-8 upfront, Go strings are guaranteed valid UTF-8 by the language spec, making this a non-issue in practice. The worst-case behavior (returning empty string) is safe. The reviewer's claim of "looping infinitely" is incorrect; the logic is sound and safe for all Go strings.


### infrastructure/sheets  (12)

#### 66. 🔴 Missing test coverage for critical Lookup function
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/client.go:61-137`
- **Код:** `websites_test.go has 5 tests covering utility functions (TestSuitableCell, TestResultRangeTargetsRow, TestParseAVerdicts, etc.), but ZERO tests for client.go's Lookup() function, which is critical to the articles module.`
- **Влияние:** The Lookup function (topics/keywords fetching) is untested. Regressions in normalization, keyword parsing (SplitSeq), title extraction, or header skipping could ship undetected.
- **Фикс:** Add test_client.go with tests: TestLookupEmptyTopic, TestLookupWithHeaderRow, TestLookupKeywordParsing, TestLookupTitleExtraction, TestLookupNormalization. Mock the sheets API or use integration tests with fixture data.
- **Проверка:** The Lookup function at lines 61-137 of client.go is the sole implementation of the TopicSource interface used by the articles domain. It contains complex logic: normalizing topics, fetching from Google Sheets API, iterating rows, handling headers, parsing comma-separated keywords with SplitSeq, deduplicating via normalization, extracting titles, and logging. websites_test.go contains 5 tests (TestSuitableCell, TestResultRangeTargetsRow, TestParseAVerdicts, TestParseECredentialsJoin, TestStaleEStatusRows) covering utility functions like resultRange, parseAVerdicts, and staleEStatusRows—but zero tests for Lookup itself. No other test file tests the client.Lookup method. The finding's description is accurate and the severity should be high (not medium) since this is the primary entry point for article keyword/title data and all critical parsing/deduplication logic is untested.

#### 67. 🟠 No timeout enforcement for batch read/write operations
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:74-106, 162-191, 254-288`
- **Код:** `ListCredentials, List, and ClearStaleStatuses perform Google Sheets API calls without timeouts:

resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, aRange).Context(ctx).Do()

Unlike client.go Lookup (line 67) which uses ˗context.WithTimeout(ctx, 10*time.Second)˗, these functions rely entirely on the parent context timeout.`
- **Влияние:** If the Google Sheets API becomes unresponsive or network is slow, these operations could block indefinitely (or until parent context timeout, which could be minutes). This causes resource exhaustion and poor user experience.
- **Фикс:** Add explicit timeouts for each API call: `ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second); defer cancel()` and pass ctxTimeout to the API call instead of ctx.
- **Проверка:** Confirmed: ListCredentials (162-191), List (74-106), and ClearStaleStatuses (254-288) all pass ctx directly to Google Sheets API calls without explicit timeout wrapping, unlike client.go Lookup (67) which uses context.WithTimeout. However, severity is overstated: these functions receive ctx from HTTP handlers, which have implicit parent timeouts. The real issue is lack of operation-level timeouts for batch reads/writes—a defensive hardening measure rather than true indefinite blocking, since parent context will eventually timeout.

#### 68. 🟠 Hardcoded magic column letters without constants or validation
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:75, 116-126, 152, 163, 170, 201, 234, 255, 261, 274`
- **Код:** `fmt.Sprintf("%s!A:A", sheet) // Line 75
fmt.Sprintf("%s!B%d:D%d", sheet, row, row) // Line 152
fmt.Sprintf("%s!H%d", sheet, r.Row) // Line 125
fmt.Sprintf("%s!I%d", sheet, r.Row) // Line 234

Columns are scattered throughout without clear documentation of schema.`
- **Влияние:** Column mappings are implicit and fragile. If the spreadsheet schema changes (e.g., columns shift), code breaks silently. No way to verify correct column usage. Makes refactoring risky.
- **Фикс:** Define constants at package level: `const colTopic = "B"; const colOutbound = "C"; const colSuitable = "D"; const colLoginStatus = "H"; const colPlacementStatus = "I"` and use them in fmt.Sprintf calls.
- **Проверка:** Verified: Line 75 reads column A (website URLs). Lines 116-126 write B:D via resultRange() helper. Lines 125, 201, 234, 273 hardcode column H and I for login/placement status. Lines 163, 170, 255, 261 hardcode A:D and E:I ranges. Column mappings are entirely implicit with no constants, no schema documentation, and no validation. If spreadsheet columns shift, code silently reads/writes to wrong cells. The finding accurately describes real fragility in the code architecture.

#### 69. 🟡 Missing nil pointer check for Google Sheets response in List function
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:74-89`
- **Код:** `resp, err := s.svc.Spreadsheets.Values.Get(...).Do()
if err != nil {
  return nil, fmt.Errorf(...)
}

for i, row := range resp.Values {`
- **Влияние:** Same as above: if resp is nil, accessing resp.Values would panic. Also applies to ListCredentials (lines 162-175) and ClearStaleStatuses (lines 254-265).
- **Фикс:** Add nil checks for resp after each API call: `if resp == nil { return ..., fmt.Errorf("sheets: unexpected nil response") }`
- **Проверка:** The finding is technically accurate — the code does access resp.Values without explicit nil checks at the cited locations: List() line 86, ListCredentials() lines 168 and 175, ClearStaleStatuses() lines 259 and 265. However, this is not a practical bug. The Google Sheets Go client library (.Do() calls) follows standard Go conventions where a non-nil response is guaranteed when error is nil. Explicit nil checks would be defensive programming but are not required for correctness. The code pattern (check error first, then use response) is idiomatic Go and safe. Severity should be downgraded from high to low.

#### 70. 🟡 No validation of Row field when constructing sheet ranges
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:115, 125, 201, 234`
- **Код:** `fmt.Sprintf("%s!B%d:D%d", sheet, r.Row, r.Row) // Line 152
fmt.Sprintf("%s!H%d", sheet, r.Row) // Line 125
fmt.Sprintf("%s!H%d", sheet, r.Row) // Line 201
fmt.Sprintf("%s!I%d", sheet, r.Row) // Line 234`
- **Влияние:** If Row is 0 or negative, invalid sheet ranges like "Sheet!B0:D0" or "Sheet!H-5" are generated, causing API errors or undefined behavior. While current code maintains Row >= 1 invariant, there's no defensive validation.
- **Фикс:** Add validation in each write function: `if r.Row <= 0 { return fmt.Errorf("invalid row number: %d", r.Row) }` before constructing ranges.
- **Проверка:** The code does use r.Row and row parameters directly in fmt.Sprintf calls at lines 152, 125, 201, and 234 without explicit validation. However, the Row values are always constructed as i+1 (starting from i=0) in List(), parseECredentialsJoin(), and staleEStatusRows(), guaranteeing Row >= 1. This invariant is maintained throughout the codebase. While adding defensive validation would be good practice, the current code does not have a real bug — Row is never 0 or negative in normal operation. The Google Sheets API would also reject invalid ranges as a safety net. This is a defensive coding improvement suggestion, not a medium-severity error-handling bug.

#### 71. 🟡 Excessive duplication in factory functions
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** duplication
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:23-45`
- **Код:** `NewWebsiteSource (lines 23-29), NewCredentialSource (lines 31-37), and NewPlacementSink (lines 39-45) are identical: they all call newSource() and cast the result. The code is repeated 3 times.`
- **Влияние:** Code duplication makes maintenance harder and increases bug surface. If newSource() needs updating, all three functions must be checked. Reduces readability.
- **Фикс:** Keep newSource() private and remove the three public wrappers. Instead, have newSource() return `(WebsiteSource, error)`, `(CredentialSource, error)`, and `(PlacementSink, error)` by type assertion on the same *websiteSource. Or, create a single factory function that returns the struct implementing all three interfaces.
- **Проверка:** The three factory functions (NewWebsiteSource, NewCredentialSource, NewPlacementSink) at lines 23-45 do indeed call newSource() identically and perform the same error handling. However, the reviewer overstates the severity. Each function returns a different interface type (WebsiteSource, CredentialSource, PlacementSink), and *websiteSource implements all three. The 7-line duplication is minimal and the interfaces represent intentional API boundaries. Consolidating would either force callers to assert types or require awkward generic patterns in Go. The maintenance burden is low; this is stylistic duplication, not a structural defect. Severity should be low, not medium.

#### 72. 🟡 Potential sensitive data exposure in info-level logging
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:177-189`
- **Код:** `included := make([]string, 0, len(out))
for _, c := range out {
  included = append(included, fmt.Sprintf("row=%d topic=%q base=%s", c.Row, c.Topic, c.BaseURL))
}
s.log.InfoContext(ctx, "sheets credentials list", ..., "included", included)`
- **Влияние:** BaseURL values (domain names + paths) from credentials are logged at INFO level. While not passwords, this leaks server/client information in application logs, potentially visible to non-admin users. If logs are shipped to external monitoring, this is a data leak.
- **Фикс:** Either: (1) Remove the 'included' field entirely, or (2) Log at DEBUG level, or (3) Hash/anonymize URLs: `fmt.Sprintf("row=%d", c.Row)` without BaseURL.
- **Проверка:** The code at lines 177-189 does log BaseURL values at INFO level (line 181: `s.log.InfoContext(..., "included", included)` where `included` contains formatted strings with `c.BaseURL`). However, BaseURL is not sensitive data — it's a domain name extracted from sheets (line 333: `BaseURL: base`). While logging URLs at INFO level is poor hygiene and should ideally be DEBUG, BaseURL is not a credential or secret. The finding conflates "operational information that shouldn't be logged loudly" with "sensitive data exposure," which is severity inflation. The actual concern is log chattiness, not credential leakage.

#### 73. 🟡 Unused Result.URL field in WriteResults
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** dead-code
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:108-149`
- **Код:** `WriteResults receives []linkbuilding.Result, and Result struct has URL field (internal/domain/linkbuilding/entity.go line 18), but WriteResults never accesses r.URL. Only uses r.Row, r.Topic, r.OutboundDomains, r.Suitable.`
- **Влияние:** Dead code indicates API mismatch or incomplete implementation. If URL should be written to spreadsheet (e.g., for debugging), it's silently dropped. If URL is not needed, the struct design is wrong.
- **Фикс:** Either: (1) Write URL to the spreadsheet (add to ValueRange values), or (2) Remove URL from Result struct definition and update all callers (application/linkbuilding/service.go line 180).
- **Проверка:** Verified by reading /Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go lines 108-149 and /Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go line 180. The Result struct is populated with URL at line 180 (URL: w.URL), but WriteResults() at lines 114-122 never accesses r.URL—it only writes r.Topic, r.OutboundDomains, and r.Suitable to columns B:D. The URL field is genuinely unused in the spreadsheet write operation. This is accurate dead code, though the URL remains accessible via r.Row (the row number references the original Website). Severity is appropriately low since there's implicit access to the URL through row reference.

#### 74. 🟡 No validation of sheet parameter in range construction
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/websites.go:75, 116, 125, 152, 163, 170, 201, 234, 255, 261, 274`
- **Код:** `fmt.Sprintf("%s!A:A", sheet) and variants use sheet parameter directly without validation. If sheet contains invalid characters (e.g., spaces, quotes, '!'), Google Sheets API will reject the range.`
- **Влияние:** Invalid sheet names crash the API call with confusing error messages. No upfront validation means errors surface at runtime in production.
- **Фикс:** Add helper function: `func validSheetName(s string) error { if s == "" || strings.ContainsAny(s, "'!\"\n") { return fmt.Errorf("invalid sheet name") } return nil }` and call it at the start of each public method.
- **Проверка:** The code at all cited lines does use the sheet parameter directly in fmt.Sprintf() without validation. This is accurately identified. However, the severity is overstated: Google Sheets API will reject invalid range strings with an error message, which is caught and returned to the caller with context (the problematic range is included). The practical impact depends on the source of sheet names—if they originate from trusted sources (config, database), this is a code quality improvement rather than a critical issue. If sheet names come from user input, it would be medium severity. Adjusting to low because errors surface clearly in the response with actionable context, and the API acts as a safety net.

#### 75. 🟡 Potential off-by-one error in columnOffset calculation
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/client.go:143-152`
- **Код:** `func columnOffset(base, col string) int {
  b, c := strings.ToUpper(base), strings.ToUpper(col)
  if len(b) != 1 || len(c) != 1 {
    return -1
  }
  return int(c[0]) - int(b[0])
}`
- **Влияние:** If col comes before base (e.g., titleCol="A", topicCol="C"), columnOffset returns negative value. While the code guards against this with `titleIdx >= 0` check on line 113, a developer might forget this guard in future code. No clear documentation of the invariant.
- **Фикс:** Add a comment: `// Returns offset of col relative to base (e.g., 'C' relative to 'A' = 2). Returns -1 if invalid. Negative return means col < base, which is invalid usage.` Optionally add `abs()`  handling or return `error` instead of `-1` to force explicit handling.
- **Проверка:** The function at lines 143-152 correctly returns `int(c[0]) - int(b[0])` which CAN be negative if col < base. However, the finding overstates severity: line 113 already includes a `titleIdx >= 0` guard that prevents misuse in the current codebase. The real issue is lack of documentation about negative return semantics and potential for future refactors to forget the guard—a maintainability concern rather than an active bug. The evidence quote is also incomplete (omits the empty-string checks on lines 144-145). Current code is defensive and working.

#### 76. ⚪ Context timeout applied too late in Lookup function
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/client.go:61-68`
- **Код:** `func (c *client) Lookup(ctx context.Context, topic string) (articles.Cluster, error) {
  topic = normalize(topic)
  if topic == "" {
    return articles.Cluster{}, nil  // No timeout needed here
  }
  ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
  defer cancel()`
- **Влияние:** The normalize() call and empty topic check happen before the timeout context is created. While this is a tiny overhead, it's inconsistent and could mask the intent that the entire Lookup should have a 10-second SLA.
- **Фикс:** Move `ctx, cancel := context.WithTimeout(ctx, 10*time.Second); defer cancel()` to line 62, immediately after entering the function, before any work.
- **Проверка:** The code at lines 61-68 does call normalize() and perform an empty-check before creating the timeout context. However, this is pragmatic design, not a bug. The timeout is correctly applied immediately before the expensive Google Sheets API call (lines 77-80), which is where the 10-second SLA actually matters. The normalize() call and empty-check are trivial O(1) operations that execute in microseconds. Applying the timeout after these quick validations is sensible and doesn't mask any intent—the 10-second limit correctly protects the actual I/O operation, not string processing. No fix is needed.

#### 77. ⚪ Missing cleanup of credentials data in memory
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/client.go:34-42`
- **Код:** `data, err := os.ReadFile(credentialsFile) // Line 34
creds, err := google.CredentialsFromJSON(ctx, data, ...) // Line 39
// data byte slice is never explicitly cleared`
- **Влияние:** The credentials file contents (data byte slice) remains in memory until garbage collection. In a long-running server, this could expose credentials if memory is dumped or swapped to disk. Not a critical issue (GC will clean up), but poor security practice.
- **Фикс:** After parsing, explicitly zero the sensitive data: `defer func() { for i := range data { data[i] = 0 } }()` immediately after os.ReadFile().
- **Проверка:** The code at lines 34-42 correctly reads credentials and immediately parses them. The byte slice is a local variable that is garbage collected after the function returns. This is initialization code (runs once), not a hot path. The proposed "fix" provides no meaningful security benefit — explicit byte zeroing of local variables doesn't prevent memory dumps and is not a real security practice. The code handles credentials properly by not retaining the raw bytes in any persistent structure.


### WordPress automation (wplogin/wppost/wordpress/backlinkplacer)  (17)

#### 78. 🔴 Silent error discarding in io.ReadAll() in wppost.go
- **Severity:** critical → high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wppost/wppost.go:60, 113`
- **Код:** `body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`
- **Влияние:** I/O errors are silently swallowed. Partial/empty JSON responses are returned to callers. json.Unmarshal will fail with misleading errors ("unexpected EOF") rather than reporting the true I/O failure. Post listing and updates fail silently.
- **Фикс:** Capture and return I/O errors: `body, err := io.ReadAll(...); if err != nil { return ..., fmt.Errorf("read body: %w", err) }`
- **Проверка:** The code at lines 60 and 113 in /Users/user/work/multiagent-seo/backend/internal/infrastructure/wppost/wppost.go both use `body, _ := io.ReadAll(...)` which silently discards I/O errors. Line 60 discards errors before calling json.Unmarshal() at line 72, which will report misleading "unexpected EOF" errors. Line 113 discards errors before using body in snippet() at line 115 for error reporting. This is a real bug: network errors are masked and callers see incorrect error messages instead of the true I/O failure. The severity is high (not critical) because the code still fails via json.Unmarshal or status code validation, just with poor error clarity that hampers troubleshooting.

#### 79. 🔴 Regex pattern uses character class with negation that doesn't handle nested braces safely
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/appauth.go:16`
- **Код:** `var nonceRe = regexp.MustCompile(˗wpApiSettings\s*=\s*\{[^}]*"nonce":"([a-zA-Z0-9]+)"˗)`
- **Влияние:** The pattern `[^}]*` will stop at the first `}`, even if it's inside a string value. If wpApiSettings object contains nested JSON with `}` in a string, the regex fails to match. Nonce extraction fails unpredictably on certain WP sites.
- **Фикс:** Use a more robust approach: parse the JSON structure properly or use a more specific lookahead pattern that handles quoted strings: `wpApiSettings\s*=\s*\{([^}]*"nonce":"([a-zA-Z0-9]+)"[^}]*)`
- **Проверка:** The regex pattern at line 16 is indeed vulnerable. It uses `[^}]*` which stops at the first closing brace, even if that brace is inside a quoted string value preceding the nonce field. This will cause nonce extraction to fail (return no match) if the wpApiSettings object contains any `}` character in a string field before the nonce property. This is a real parsing bug for certain WordPress site configurations. The evidence and impact description are accurate. The suggested fix pattern in the finding is flawed (uses double capture groups inefficiently), but the underlying issue is confirmed: using regex to parse JSON-like structures is error-prone and this pattern will fail on valid WordPress pages with certain field orderings.

#### 80. 🔴 Unsafe HTML truncation in backlinkplacer silently corrupts HTML
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/backlinkplacer/backlinkplacer.go:34-38`
- **Код:** `src := html
if len(src) > maxInputHTML {
  src = src[:maxInputHTML]
}`
- **Влияние:** Raw byte-level truncation at 20KB without respecting UTF-8 boundaries or HTML structure. Truncation can occur in the middle of a multi-byte UTF-8 character (producing invalid UTF-8) or mid-tag. The LLM receives malformed HTML and may produce invalid output. Valid backlinks may be rejected or placed incorrectly.
- **Фикс:** Use UTF-8 aware truncation: if len(src) > maxInputHTML { src = src[:maxInputHTML]; for !utf8.ValidString(src) { src = src[:len(src)-1] } } or better: truncate at the last closing tag boundary
- **Проверка:** The code at lines 36-39 performs raw byte-level truncation with src[:maxInputHTML] (20KB limit) without UTF-8 validation or HTML awareness. This is a real bug: truncation can break multi-byte UTF-8 characters mid-sequence (producing invalid UTF-8) and can cut HTML tags in the middle. The LLM receives malformed input. While downstream validation at lines 55-58 may reject some bad outputs, the truncation itself is unsafe and should respect UTF-8 boundaries. Severity is appropriately "high" — this is a data corruption bug affecting the HTML integrity passed to the LLM.

#### 81. 🔴 Non-atomic multi-step credential generation lacks transactional guarantees
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:232-242`
- **Код:** `appPwd, err := s.issuer.IssueAppPassword(ctx, c.BaseURL, c.Login, c.Password)
if err != nil { ... return res }
donor = domain.DonorCredential{DonorURL: c.BaseURL, Login: c.Login, AppPassword: appPwd}
if err := s.donors.Save(ctx, donor); err != nil {
  log.WarnContext(ctx, "save donor credential failed", ...)
}`
- **Влияние:** If Save() fails after IssueAppPassword() succeeds, the app password is lost. On retry, a new app password is issued, leaving the old one orphaned. If Save() fails due to network/DB issues while another goroutine is placing backlinks, credential inconsistency ensues. The error is logged but not propagated, causing silent data loss.
- **Фикс:** Propagate Save errors: `if err := s.donors.Save(ctx, donor); err != nil { res.Status = "failed: save app password: " + truncReason(err.Error()); return res }` to prevent orphaned credentials and ensure atomicity from caller's perspective
- **Проверка:** Code at lines 233-242 confirms the finding: IssueAppPassword() succeeds, the credential is constructed, but if Save() fails, the error is logged without propagation. The function continues using the in-memory credential in subsequent operations (line 245+), so the immediate request doesn't fail. However, on retry, the credential is not persisted, so Get() returns !ok, causing another IssueAppPassword() call and leaving the previous one orphaned. The finding correctly identifies the missing error propagation and the risk of orphaned credentials accumulating. The fix is appropriate: error should be propagated to return early and signal failure to the caller, preventing orphaned credentials and ensuring consistent state."

#### 82. 🟠 Silent error discarding in io.ReadAll() calls masks I/O failures
- **Severity:** critical → medium  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/wplogin.go:125`
- **Код:** `body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`
- **Влияние:** If io.ReadAll fails (disk/memory pressure, context cancellation), the error is silently dropped. The body will be empty/partial, causing silent failures in loginError() parsing and form validation. Login decisions are made on corrupted data.
- **Фикс:** Capture and check the error: `body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); if err != nil { return res, fmt.Errorf("read login response: %w", err) }`
- **Проверка:** Line 125 does discard the io.ReadAll error: `body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`. If the read fails, body is empty/partial. However, the actual impact is limited: (1) The login decision at line 128 uses hasLoggedInCookie(), which is NOT affected by body content. (2) The body is only used in loginError() at line 147, which already handles parse errors by returning "" (line 203). (3) An empty result causes a generic fallback message at line 149 instead of the specific error. The code degrades gracefully to a generic "no error message returned" rather than exposing the read failure. This is a real error-handling gap (missing error check) causing poor diagnostics, but NOT a critical vulnerability affecting login decisions—adjusted to medium severity.

#### 83. 🟠 Silent error discarding in io.ReadAll() calls in appauth.go
- **Severity:** critical → medium  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/appauth.go:101, 129`
- **Код:** `body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`
- **Влияние:** Same as wplogin.go line 125 issue. In fetchNonce(), missing nonce extraction is not distinguished from empty body due to I/O error. In createAppPassword(), empty/partial JSON responses from I/O errors are parsed, potentially returning empty password without reporting the true cause.
- **Фикс:** Check all io.ReadAll errors: `body, err := io.ReadAll(...); if err != nil { return "", fmt.Errorf("read response body: %w", err) }`
- **Проверка:** Line 101 and 129 both discard io.ReadAll() errors as described. In fetchNonce (line 101): if ReadAll fails, body is incomplete, regex fails to find nonce, and function returns "wpApiSettings nonce not found" — a misleading error message since the real problem was I/O. In createAppPassword (line 129): if ReadAll fails, the JSON unmarshal at line 138 will typically catch it and return a parse error, OR if a partial read produces valid JSON with empty password, the check at line 141 would report "empty password" instead of the I/O error. Both are error-handling gaps but not silent failures — the functions do return errors, just potentially with misleading root cause attribution. Severity should be medium (confusing error messages) not critical (silent failure).

#### 84. 🟠 Hardcoded list of English number words doesn't scale to other languages
- **Severity:** low → medium  ·  **Verdict:** confirmed  ·  **Category:** abstraction
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/form.go:22-28`
- **Код:** `var numberWords = map[string]int{ "zero": 0, "one": 1, ... "ninety": 90, }`
- **Влияние:** Math captcha solving only works for English sites. WordPress sites in non-English locales will have math challenges in their language (e.g., 'cinco más cuatro'), which won't be solved. The bot treats unsolved challenges as manual-captcha-required, silently failing on multilingual sites.
- **Фикс:** If multilingual support is needed, parametrize language or use a language-aware solver. For now, add comment: `// TODO: English-only solver; non-English WordPress sites require manual intervention` and log language detection attempt
- **Проверка:** The code at lines 22-28 defines a hardcoded English-only number word map used by normalizeWords() and a matching regex at line 34. The wordOperators replacer (lines 30-32) also only handles English operator words. Together, these prevent solving math captchas in non-English languages. On a Spanish site with "cinco más cuatro", the code would fail to normalize it (no match in maps), so the math regex would find no equation, returning (0, false). The bot would silently skip the math captcha without error. The finding is accurate. Severity is better described as medium rather than low—silent failures on multilingual sites represent meaningful functional loss for non-English WordPress deployments.

#### 85. 🟠 Context cancellation not properly propagated in backlink service batch writes
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:179-181`
- **Код:** `writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
err := s.placements.WritePlacementStatus(writeCtx, sheet, pending)
cancel()`
- **Влияние:** context.WithoutCancel(ctx) strips parent cancellation, so if the outer job context is cancelled, the timeout context ignores it. Write operations can continue indefinitely even after user requests cancellation. Partial writes and dangling operations accumulate.
- **Фикс:** Don't strip cancellation: `writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)` to respect user cancellation while adding deadline
- **Проверка:** Code at lines 179-181 creates a write context via context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second). The WithoutCancel call strips parent cancellation, so if the outer job context is cancelled (checked at lines 191 and 203), the flush operation still executes the write because writeCtx ignores parent cancellation. This allows writes to proceed even after user-initiated job cancellation, violating cancellation semantics. The loop attempts graceful shutdown by checking ctx.Err(), but flush() bypasses that check by using a cancellation-immune context. Fair severity is medium: write operations can continue after cancellation is requested, causing partial writes and state inconsistency in batch scenarios, but the 15-second deadline still provides eventual timeout protection.

#### 86. 🟡 Silent error discarding in json.Marshal() call
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/appauth.go:114`
- **Код:** `bodyJSON, _ := json.Marshal(map[string]string{"name": name})`
- **Влияние:** json.Marshal is called without error checking. Though unlikely to fail for simple strings, any failure silently produces an empty bodyJSON, causing the request body to be '{}' instead of the intended app password name. The WP API would reject the malformed request.
- **Фикс:** Check the error: `bodyJSON, err := json.Marshal(...); if err != nil { return "", fmt.Errorf("marshal app password request: %w", err) }`
- **Проверка:** Line 114 in appauth.go does discard the error from json.Marshal(): `bodyJSON, _ := json.Marshal(map[string]string{"name": name})`. The finding is real but the severity is overstated. For this specific code (marshaling a simple map[string]string), json.Marshal failure is virtually impossible in practice. If it somehow failed, the bodyJSON would be nil (not "{}"), resulting in an empty request body sent at line 116. The WP API would then reject this with an HTTP error (line 131-132), which is properly handled and propagated. This is poor Go practice (blanking error handling), but not a high-severity bug in this context. Fair severity: low (code smell / best practice violation), not high.

#### 87. 🟡 URL origin reconstruction doesn't validate input prevents injection
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/wplogin.go:44-60`
- **Код:** `u, err := url.Parse(base)
if err != nil || u.Scheme == "" || u.Host == "" { ... }
origin, err := url.Parse(u.Scheme + "://" + u.Host)
loginURL := origin.String() + "/wp-login.php"`
- **Влияние:** While url.Parse validates syntax, reconstructing with string concatenation `u.Scheme + "://" + u.Host` is safe. However, no validation that u.Host doesn't contain port numbers or userinfo. If Host contains `@`, string splitting could allow subtle host confusion. Not a direct injection but fragile.
- **Фикс:** Use url.URL fields consistently: `origin := &url.URL{Scheme: u.Scheme, Host: u.Host}; loginURL := fmt.Sprintf("%s/wp-login.php", origin.String())` to be explicit and avoid manual concatenation
- **Проверка:** The code at lines 44-60 uses url.Parse() which automatically validates and normalizes the input. At line 50, u.Host is reconstructed safely because url.Parse() already separates userinfo from the host field—if the input contained `user:pass@host`, the `@` and userinfo would be parsed into u.User, leaving u.Host containing only the hostname:port. The concern about Host containing `@` is unfounded. Line 50's string concatenation (u.Scheme + "://" + u.Host) is safe from injection. The finding conflates code style/elegance (manual concatenation vs. url.URL struct construction) with actual security risk. The risk severity should be low, not medium—this is a style preference, not a vulnerability.

#### 88. 🟡 WordPress API status code check uses range instead of explicit codes
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/appauth.go:131`
- **Код:** `if resp.StatusCode/100 != 2 { return "", fmt.Errorf("status %d: %s", resp.StatusCode, snippet(body)) }`
- **Влияние:** Accepts any 2xx status code (200-299). While appropriate for WP API, the code doesn't distinguish between 200 (success) and 204 (success but no content). If the API ever returns 204, json.Unmarshal on empty body will fail with confusing error. Makes debugging harder.
- **Фикс:** Check for specific expected codes: `if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK { ... }` or at minimum, add comment explaining why range is used
- **Проверка:** The code at line 131 correctly implements `resp.StatusCode/100 != 2` to accept all 2xx codes. The finding's concern that 204 No Content would cause json.Unmarshal to fail on empty body is technically valid but overstated: the WordPress REST API for creating application passwords is documented to return 201 Created with response body, not 204. A 204 response would already be non-standard behavior. While the code could fail with a confusing error message if 204 were returned, this is an edge case that violates API specification rather than a likely bug. Adding a clarifying comment would be good practice, but the actual risk is low. This is better classified as a minor code clarity/documentation issue rather than a medium-severity bug.

#### 89. 🟡 Missing context cancellation checks in recursive tree walks
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/form.go:68-86, 90-113, 148-169, 174-187, 214-228`
- **Код:** `var walk func(*html.Node)
walk = func(n *html.Node) { ... for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) } }`
- **Влияние:** HTML tree walks are unbounded recursive functions. If a malicious or malformed HTML document has pathologically deep nesting (e.g., 10000 levels), the walk will cause stack overflow or excessive memory/CPU use. No context cancellation checks allow stopping early.
- **Фикс:** Add depth limit and context checks: `func walk(n *html.Node, depth int) { if depth > 1000 { return }; if ctx.Err() != nil { return } ... walk(c, depth+1) }` or refactor to iterative tree walk with a queue
- **Проверка:** The code contains six recursive tree walk functions (findLoginForm lines 68-86, collectForm lines 90-113, hasCaptchaMarker lines 148-169, formContainsInput lines 174-187, elementByID lines 214-228, textContent lines 231-244) that traverse HTML DOM trees without depth limits or context cancellation. All recursively call `walk(c)` for each child node. The finding is accurate about the code structure, but severity is overstated: (1) these are internal utility functions processing WordPress login form HTML, which realistically has shallow nesting; (2) functions like hasCaptchaMarker, formContainsInput, and elementByID have early-exit logic that prevents unnecessary full traversals; (3) golang.org/x/net/html parser would handle pathologically nested input at the parser level, not these walk functions. The risk of stack overflow from typical login form HTML is minimal.

#### 90. 🟡 Unvalidated HTML string passed to LLM without escaping or encoding
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/backlinkplacer/backlinkplacer.go:66-77`
- **Код:** `func buildPrompt(html, targetURL string) string { ... b.WriteString(html) ...`
- **Влияние:** Raw HTML content from WordPress post is embedded into the LLM prompt without any escaping or encoding. If HTML contains special characters or prompt injection payloads (e.g., `\n---HTML---\nmalicious`), the LLM parsing will be confused. Attacker-controlled post content could manipulate anchor text or links.
- **Фикс:** Escape or validate HTML before embedding: add comment explaining prompt structure or sanitize HTML content with html.EscapeString or validate it contains no `---HTML---` separators
- **Проверка:** The code at lines 66-77 does embed raw, unvalidated HTML into the LLM prompt at line 71 via b.WriteString(html). However, the actual vulnerability is not prompt injection as described. The real risk is that if attacker-controlled HTML contains the separator strings "---ORIGINAL---" or "---MODIFIED---", the parseReply() function (line 81+) could fail to parse the LLM response correctly, causing a parsing error. This breaks the feature but does not allow manipulation of anchor text—the error is caught and returns an empty result. The impact is a denial-of-service-like failure, not a security bypass. Proper fix: use delimiters unlikely in HTML or escape separators in the input, not full HTML escaping.

#### 91. 🟡 Anchor parsing doesn't handle quoted anchor values robustly
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/backlinkplacer/backlinkplacer.go:109-120`
- **Код:** `func extractAnchor(head string) string {
  for _, line := range strings.Split(head, "\n") {
    line = strings.TrimSpace(line)
    if !strings.HasPrefix(strings.ToUpper(line), "ANCHOR:") { continue }
    v := strings.TrimSpace(line[len("ANCHOR:"):])
    v = strings.Trim(v, "\"'\˗")
    return strings.TrimSpace(v)
  }`
- **Влияние:** Anchor parsing doesn't handle escaped quotes. If LLM returns `ANCHOR: "he said \"hello\""`, the Trim() call removes only outer quotes, leaving inner escaped quotes in the anchor text. Anchor validation in backlink_service_test.go would fail or accept malformed anchors.
- **Фикс:** Use a more robust parser: `if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") { v = v[1:len(v)-1]; v = strings.ReplaceAll(v, "\\\"", "\"") }` to handle escaping properly
- **Проверка:** The code at lines 124-135 uses strings.Trim(v, "\"'`") which removes leading/trailing quote characters from both ends. The finding claims this doesn't handle escaped quotes, but this is not an actual bug for the intended use case. The LLM prompt doesn't instruct quote inclusion; most responses would be plain text like "ANCHOR: hello world". The Trim() correctly strips optional outer quotes if present. Escaped quotes within the anchor text (e.g., "he said \"hello\"") aren't a realistic concern since anchors are meant to be 2-4 words drawn from existing paragraph text. The real gap is lack of word-count validation per the prompt specification, not quote escaping handling.

#### 92. 🟡 Empty HTML response from WordPress is not distinguished from I/O error
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wppost/wppost.go:75-76`
- **Код:** `if len(arr) == 0 { return linkbuilding.DonorPost{}, fmt.Errorf("wppost: no published posts") }`
- **Влияние:** If the body read partially fails due to I/O error, arr will be empty from json.Unmarshal error or successful parse of incomplete JSON. The error message 'no published posts' is misleading when the true cause is an I/O error that was silently swallowed on line 60.
- **Фикс:** Check io.ReadAll error first (as per finding #4) so the true root cause is reported
- **Проверка:** Lines 75-76 correctly report "no published posts" when arr is empty after successful JSON parsing. The actual I/O error concern is on line 60 where io.ReadAll's error is silently discarded. However, in practice, a failed io.ReadAll typically causes json.Unmarshal to fail at line 72-74 (which is properly reported), so the misleading "no published posts" message would only appear if io.ReadAll succeeds but returns empty data—a legitimate scenario. The severity of the lines 75-76 finding is overstated; the real root cause is the silent discard at line 60.

#### 93. ⚪ No validation of extracted nonce format before use
- **Severity:** medium → none  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/appauth.go:105-109`
- **Код:** `m := nonceRe.FindSubmatch(body)
if len(m) < 2 { return "", fmt.Errorf("wpApiSettings nonce not found in profile page") }
return string(m[1]), nil`
- **Влияние:** Extracted nonce is not validated for format or length. If the regex matches an unexpected value (e.g., '0' or empty string after group), an invalid nonce is silently returned. The app password creation endpoint will reject it, but the error is not clear.
- **Фикс:** Add validation: `nonce := string(m[1]); if nonce == "" || len(nonce) < 8 { return "", fmt.Errorf("invalid nonce format from profile page") }; return nonce, nil`
- **Проверка:** The code at lines 105-109 uses a regex with a capturing group `([a-zA-Z0-9]+)` that already enforces format constraints: alphanumeric-only characters and a minimum of 1 character (the + quantifier). FindSubmatch will only populate m[1] with a non-empty string matching this pattern or return a shorter slice (caught by the len(m) < 2 check). The extracted nonce cannot be empty or contain invalid characters without the regex failing to match. The suggested validation (length < 8) is not a real constraint—WordPress nonces vary in length. No additional validation is needed; the regex already validates format and presence.

#### 94. ⚪ Division by zero in math solver returns (0, false) without clear signaling
- **Severity:** low → none  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/wplogin/form.go:137-141`
- **Код:** `case "/", "÷":
  if b == 0 { return 0, false }
  return a / b, true`
- **Влияние:** Division by zero returns 0 with ok=false. While correct, returning 0 as the answer is misleading if parsing logic elsewhere confuses it with a valid answer of 0. Code is correct but fragile; if future code checks `ok` incorrectly, it silently accepts the 0.
- **Фикс:** No change needed if properly tested, but add comment: `// Return (0, false) for div-by-zero; ok=false is required downstream` to prevent future confusion
- **Проверка:** The code at lines 137-141 correctly implements division-by-zero handling with the idiomatic Go pattern: returns (0, false) when b==0, and the caller at line 59 properly checks the boolean before using the value with `if n, ok := solveMath(...); ok`. The test suite explicitly validates this behavior (line 289: `{"5 / 0 =", 0, false}`). Returning 0 with ok=false is not misleading because downstream code never uses the value without checking ok first. The suggestion for a clarifying comment is optional style guidance, not a bug fix. There is no actual safety risk here.


### webfetch / pexels / dataforseo  (13)

#### 95. 🔴 Missing HTTP client redirect guard in pexels and dataforseo
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:36`
- **Код:** `http:    &http.Client{Timeout: 10 * time.Second}`
- **Влияние:** Pexels and DataForSEO HTTP clients lack CheckRedirect guards. While webfetch implements redirect capping (line 44), these clients silently follow all redirects. Malicious API responses could redirect to internal services (SSRF via open redirect), bypassing SSRF guards entirely.
- **Фикс:** Configure CheckRedirect on the http.Client to match webfetch's strategy: cap redirects at maxRedirects and validate redirect targets are HTTPS and not internal IPs.
- **Проверка:** Verified at /Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:36 and /Users/user/work/multiagent-seo/backend/internal/infrastructure/dataforseo/client.go:27. Both http.Client instances are initialized with only a Timeout field and no CheckRedirect handler. The comparison to webfetch (line 44 of webfetch.go) is accurate—webfetch's http.Client includes CheckRedirect: checkRedirect which caps redirects at maxRedirects (5). Pexels and DataForSEO clients lack this guard and will follow redirects without limit. Additionally, unlike webfetch's Transport which includes a dial guard blocking private/loopback IPs, these clients have no such guards. This creates a genuine SSRF-via-redirect vulnerability: a malicious or compromised Pexels/DataForSEO API response could redirect the client to internal services (e.g., http://localhost:8080 or 10.0.0.x ranges) without restriction. Severity remains high—this is a real security gap in production HTTP clients making external API calls.

#### 96. 🔴 Pexels client missing test coverage entirely
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:1-102`
- **Код:** `No pexels/client_test.go file exists; only resolver.go has tests indirectly`
- **Влияние:** The pexels/client.go SearchN function has zero unit tests. Edge cases like malformed JSON, missing src fields, empty response arrays, and HTTP errors are not tested. The fallback logic (landscape->large->skip) is untested.
- **Фикс:** Create pexels/client_test.go with tests for: (1) malformed JSON responses, (2) missing landscape/large URLs, (3) empty photos array, (4) HTTP error codes, (5) timeout handling.
- **Проверка:** The pexels/client.go file (lines 1-102) contains critical network I/O and JSON unmarshaling logic with ZERO unit test coverage. The `SearchN` function at line 55-101 handles: (1) HTTP requests with context and timeout (36ms), (2) JSON decoding with malformed response handling (line 79, 1MB limit), (3) status code checking (line 74-76), and (4) the fallback logic for missing image URLs (lines 85-91: tries landscape, then large, skips if both missing). All these error paths and edge cases are untested. The only mention of pexels in existing tests is in articles_integration_test.go where it's instantiated with an empty API key. The resolver.go file contains no direct unit tests either. The finding accurately identifies a missing test file (pexels/client_test.go does not exist) and the lack of coverage for malformed JSON, missing fields, empty arrays, HTTP errors, and timeout handling is real and concerning for production code that makes external API calls.

#### 97. 🟠 Insecure http.Client for DataForSEO with basic auth credentials
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/dataforseo/client.go:27, 101`
- **Код:** `http: &http.Client{Timeout: 30 * time.Second}, ... req.SetBasicAuth(c.login, c.password)`
- **Влияние:** DataForSEO client stores login/password as plaintext in the RealClient struct (lines 18-19). SetBasicAuth encodes them in base64 (not encryption) in the Authorization header. The client lacks Transport configuration for TLS/cipher suite enforcement. If combined with an open redirect, credentials could leak to a redirected endpoint.
- **Фикс:** Implement Transport with strict TLS configuration (MinVersion=1.2, enforced cipher suites). Add a CheckRedirect guard like webfetch. Do NOT store plaintext credentials; extract from environment only when creating the client.
- **Проверка:** The code at lines 17-19 stores login/password as plaintext struct fields (accurate), but they originate from environment variables, not hardcoded. Line 27 creates http.Client without custom Transport (accurate), but since it calls https://api.dataforseo.com, Go's default TLS handling is acceptable. Line 101 uses SetBasicAuth which base64-encodes (not encrypts) credentials (accurate). The real security issue is the missing CheckRedirect handler—the client can be redirected unlimited times via server-side open redirect, sending basic auth credentials to any HTTPS endpoint. This is a medium severity issue (not high) because: 1) credentials are from env vars not hardcoded, 2) HTTPS is enforced, 3) the redirect risk requires an open redirect on the DataForSEO API itself. Recommend adding a CheckRedirect guard like webfetch.go does, rather than full TLS reconfiguration.

#### 98. 🟠 webfetch Content-Type validation is incomplete
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/webfetch/webfetch.go:127-132`
- **Код:** `if ct := resp.Header.Get("Content-Type"); ct != "" { ... }`
- **Влияние:** When Content-Type header is missing, the fetcher silently accepts ANY content and passes it to html.Parse(). A server returning text/plain, application/json, or binary data will be parsed as HTML. While html.Parse is forgiving, this violates the intent of validating text/html. Additionally, Content-Type with uppercase 'text/HTML' or variants may not match due to the lowercase prefix check.
- **Фикс:** Default to rejecting missing Content-Type headers (return error). Use mime.ParseMediaType() to properly parse the header and validate the full type/subtype, not just prefix matching.
- **Проверка:** The code at lines 127-132 validates Content-Type only when the header is present and non-empty. When Content-Type is missing, the validation block is skipped entirely and the response body is passed directly to html.Parse() without validation. The html.Parse() function is permissive and will attempt to parse any input as HTML, including text/plain, application/json, or binary data. The prefix matching logic (strings.HasPrefix with strings.ToLower) is actually adequate for handling case-insensitive matching and parameter removal via strings.Cut(). The core issue is the silent acceptance of missing Content-Type headers, which bypasses the intent of content-type validation. The finding is accurate in identifying this as a validation gap.

#### 99. 🟠 DataForSEO response parsing silently drops malformed subItems
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/dataforseo/client.go:67-76`
- **Код:** `func subItems(raw json.RawMessage) []serpSubItem { ... if err := json.Unmarshal(raw, &out); err != nil { return nil } ... }`
- **Влияние:** If json.Unmarshal fails (e.g., corrupted response from the API), the function returns nil instead of propagating the error. Callers at lines 149 and 159 have no way to know if the PAA/FeaturedSnippet data is missing due to: (1) API returning no data, or (2) parse failure. This masks data integrity issues.
- **Фикс:** Return an error from subItems() and propagate it in GetSERP(). Log parse failures with the raw JSON for debugging. Consider returning an error+data tuple to preserve partial results.
- **Проверка:** The subItems function at lines 67-76 catches json.Unmarshal errors and returns nil without logging or propagating errors. Callers at lines 149 and 159 iterate over the returned nil slice with no way to detect parse failures. This silently drops PAA and FeaturedSnippet data when the API response Items field is malformed. The error masking makes debugging difficult and hides data integrity issues. Severity is fairly assessed as medium — not a crash, but silent data loss in important competitive intelligence fields.

#### 100. 🟠 Pexels SearchN request URL construction is unsanitized
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:59-60`
- **Код:** `u := fmt.Sprintf("%s/search?query=%s&orientation=landscape&per_page=%d", c.baseURL, url.QueryEscape(query), n)`
- **Влияние:** While url.QueryEscape properly escapes the query, the baseURL is directly interpolated without validation. If baseURL is configured to a malicious value (e.g., attacker-controlled config or via the newClientWithBaseURL function), the client will make requests to any URL. Additionally, n is not validated for negative values (if n < 1, n is set to 1 on line 56-57), but there's no upper bound, so n could be millions, creating a request for a huge page size.
- **Фикс:** Validate baseURL is https://api.pexels.com or a whitelisted domain on client creation. Add upper bound on n (e.g., n = min(n, 100)) and validate n > 0.
- **Проверка:** The code constructs a URL at lines 59-60 using fmt.Sprintf with c.baseURL (unchecked), url.QueryEscape(query) (properly escaped), and n (unbounded integer). The baseURL is directly interpolated without validation; if newClientWithBaseURL() is called with a malicious URL, requests will go to any domain. The n parameter has only a lower bound (n < 1 → n = 1) with no upper limit, allowing potentially massive per_page values. Both issues are real; severity is medium because the standard newClient() hardcodes the legitimate baseURL, so vulnerability depends on external use of newClientWithBaseURL().

#### 101. 🟠 dataforseo mock client ignores error case in limit boundary
- **Severity:** low → medium  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/dataforseo/client.go:179-195`
- **Код:** `func (m *MockClient) GetSERP(_ context.Context, keyword, _ string, limit int) (*articles.CompetitorData, error) { ... if limit < len(items) { items = items[:limit] } ... }`
- **Влияние:** The mock never returns an error, even when limit is 0 or negative. The real client (line 78-90) accepts any limit > 0 without bounds checking (location code is hardcoded, depth=limit). If limit is -1, the mock will panic on slice assignment (items[:-1]). The real client will silently request depth=-1 to the API.
- **Фикс:** Validate limit > 0 in both mock and real GetSERP. Return an error if limit is invalid instead of silently accepting it.
- **Проверка:** The finding is accurate. The code at lines 179-195 in the MockClient.GetSERP method performs slice assignment `items = items[:limit]` without validating that limit is a valid slice index. When limit is negative, Go will panic immediately with "runtime error: slice bounds out of range". When limit is 0, it correctly returns an empty slice. The real client (lines 78-90) also accepts any limit value without validation and passes it directly to the API as the Depth parameter. However, the severity should be elevated to MEDIUM because a panic in the mock client would cause test failures and crashes, not just silent incorrect behavior. The finding correctly identifies that both the mock and real client lack input validation for the limit parameter, which is a real error-handling deficiency.

#### 102. 🟡 No resource limit on DataForSEO response body
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** resource-leak
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/dataforseo/client.go:113`
- **Код:** `body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))`
- **Влияние:** The 1MB limit is applied to LimitReader but io.ReadAll still allocates a buffer that could consume significant memory if the response is near 1MB. No DoS protection on concurrent requests; multiple slow responses could exhaust memory.
- **Фикс:** Add a configurable per-request memory budget and monitor cumulative memory usage. Consider switching to streaming JSON parsing for large SERP responses instead of buffering the entire body.
- **Проверка:** The code at line 113 uses io.ReadAll(io.LimitReader(resp.Body, 1<<20)) which DOES enforce a 1MB per-request limit on the response body. The LimitReader cap is effective — io.ReadAll respects it and will not allocate beyond 1MB. The finding correctly identifies that io.ReadAll allocates memory in the 1MB range, but this is not a "resource leak" since the limit is enforced. The concern about concurrent requests exhausting memory is valid but orthogonal to this specific code location and would require concurrency/rate-limiting fixes at a higher level, not changes to this line. The suggested fix (streaming JSON parsing) is an optimization, not addressing an actual leak. Severity should be low, not medium — there IS resource protection in place.

#### 103. 🟡 Pexels Authorization header may contain unescaped API key in error logs
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:64-70`
- **Код:** `req.Header.Set("Authorization", c.apiKey); ...if err != nil { return nil, fmt.Errorf("pexels request: %w", err) }`
- **Влияние:** If the HTTP request fails (network error, timeout), the error wrapping at line 70 will not log the header. However, if an attacker intercepts logs or if Go's net package logs requests internally, the API key travels through memory as plaintext. More critically, if the request body or URL construction fails (lines 59-60), errors are logged without sanitization.
- **Фикс:** Ensure API keys are never included in error messages. Use context.WithValue to pass credentials separately from the request and implement a SafeError wrapper that strips sensitive headers before logging.
- **Проверка:** The error handling at lines 62-70 is safe. Line 64's error wrapping (for request creation failure) occurs before the Authorization header is set at line 66, so the API key is not included. Line 70's error wrapping (for request execution failure) uses fmt.Errorf with %w, which only wraps the underlying network error—not the request object or headers. The API key is not leaked through standard error logging at this location. The only real concern is plaintext storage of the apiKey field in the Client struct and potential leakage through external logging/middleware, not the error handling shown here.

#### 104. 🟡 No TLS transport configuration for HTTP clients
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:36`
- **Код:** `&http.Client{Timeout: 10 * time.Second}`
- **Влияние:** Pexels client is created with a bare http.Client that uses default Transport. No TLS configuration for certificate pinning, minimum version enforcement, or cipher suite restrictions. An attacker on the network (MITM) could downgrade the connection or intercept the API key.
- **Фикс:** Create a Transport with strict TLS config: TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, CipherSuites: [...]}, and optionally implement certificate pinning for api.pexels.com.
- **Проверка:** Line 36 creates an http.Client with only a timeout, using Go's default Transport. The code does use HTTPS (line 13: https://api.pexels.com/v1) and includes certificate validation automatically via Go's default TLS stack. Go 1.21+ enforces TLS 1.2+ and reasonable cipher suites by default. The finding overstates the MITM risk (downgrade attacks are not possible with default Go TLS settings). The real missing hardening is certificate pinning for the Pexels API endpoint, which would be defense-in-depth for protecting the API key. This is better characterized as a "nice-to-have" hardening rather than a medium-severity vulnerability.

#### 105. 🟡 webfetch allows empty href attributes in links
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** footgun
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/webfetch/webfetch.go:167-169`
- **Код:** `if href, ok := attr(n, "href"); ok { page.Links = append(page.Links, href) }`
- **Влияние:** If an HTML anchor tag has href="" (empty string), it is still added to page.Links. While the test (line 32 of webfetch_test.go) checks for missing href, it does not test empty href. This pollutes the links array with invalid URLs that callers must filter.
- **Фикс:** Validate that href is non-empty before appending: if href, ok := attr(n, "href"); ok && href != "" { ... }
- **Проверка:** Lines 167-169 correctly show the code appends href without checking if it's empty. The attr() function returns (value, ok) and the code only checks ok (attribute exists), not whether the value is non-empty. Test line 32 verifies missing href is filtered, but there's no test for href="" (empty string). The finding is accurate: empty strings would pollute page.Links. The suggested fix to add && href != "" validation is appropriate. Severity remains low since downstream callers typically validate URLs anyway, but it's a real inefficiency.

#### 106. 🟡 pexels and dataforseo HTTP clients use singleton pattern without cleanup
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** resource-leak
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/client.go:34-38`
- **Код:** `return &Client{ apiKey: apiKey, http: &http.Client{Timeout: 10 * time.Second}, baseURL: baseURL }`
- **Влияние:** Each client instantiation creates a new http.Client. In main.go (line 100), a single pexels.Resolver is created, but if the client is recreated or multiple instances are spawned, each http.Client holds TCP connection pools that are never explicitly closed. Go's http.Client does not expose CloseIdleConnections() to the Resolver, so callers cannot drain connections during graceful shutdown.
- **Фикс:** Add a Close() method to both pexels.Client and dataforseo.RealClient that calls CloseIdleConnections(). Wire these into the server shutdown sequence in main.go.
- **Проверка:** The code at lines 34-38 does create http.Client instances, but these are wrapped in singleton Resolver/RealClient objects that are instantiated exactly once at server startup (main.go line 100) and reused throughout the server lifetime. This is correct Go practice for http.Client. While the idle TCP connections in http.Client's transport are not explicitly drained during graceful shutdown, this is a minor issue (low severity) not a medium resource leak. Go's http.Client connection pooling is designed to be left alone; connections will timeout naturally. Adding CloseIdleConnections() calls would be a marginal improvement but not critical.

#### 107. 🟡 Pexels resolver returns empty error instead of nil on missing query
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/pexels/resolver.go:25-29`
- **Код:** `if query == "" { return articles.ResolvedImage{}, nil }`
- **Влияние:** When buildQuery() returns an empty string (all inputs are blank), the resolver returns an empty ResolvedImage with NO error. Callers cannot distinguish between 'no image available' and 'invalid inputs'. This is semantically odd: an empty query should either be an error or return a specific sentinel value, not silently succeed.
- **Фикс:** Return an explicit error (e.g., errors.New("empty search query")) when buildQuery returns empty string. Alternatively, document that ResolvedImage{} means 'no match found' and update callers to handle it consistently.
- **Проверка:** The code at lines 25-29 does exactly as cited: when buildQuery() returns empty string, Resolve() returns (articles.ResolvedImage{}, nil) with no error. The finding is factually correct. However, severity is overstated: the codebase consistently treats "no result" scenarios (empty query, no photos found, no topical match) as non-error cases all returning the same (ResolvedImage{}, nil). This is a design pattern choice, not a correctness bug. While callers cannot distinguish "empty input" from "no match," the code is semantically coherent within its own logic. Fair severity: low (design consistency note), not medium/high.


### pkg/* + cmd/server  (9)

#### 108. 🔴 config: LLM environment variable parsing doesn't validate provider names
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** api
- **Локация:** `backend/pkg/config/config.go:61-75`
- **Код:** `LLMConfig has Provider, QualifyProvider, BacklinkProvider as bare strings with no validation.`
- **Влияние:** User can set LLM_PROVIDER=invalid_provider and normalizeProvider() will return the lowercased invalid name. This causes ModelFor() and KeyFor() to silently return empty string or fallback to non-functional defaults, with no error signal. Example: LLM_PROVIDER=typo would silently fall through all cases in ModelFor() and return "".
- **Фикс:** Add validate:"oneof=groq claude anthropic" to Provider fields, or create a custom validator that rejects unknown providers at Load() time.
- **Проверка:** The finding is accurate. Lines 61-75 define LLMConfig with Provider, QualifyProvider, and BacklinkProvider as bare strings with NO validation tags. There is no validate:"oneof=..." or similar. The Load() function (lines 220-241) calls validate.Validate(cfg) but since LLMConfig fields lack validation constraints, invalid provider names pass through without error. The evidence is confirmed: ModelFor() at line 113-124 does exactly what the finding describes — given an invalid provider like "typo", normalizeProvider("typo") returns "typo" (lowercased), which doesn't match any case in the switch (lines 115-119), and the fallback check at lines 120-122 also fails to match (since normalizeProvider("typo") != normalizeProvider("groq") or other valid providers), so it returns empty string at line 123. KeyFor() behaves identically (lines 134-150). The test at line 94-95 explicitly documents this: KeyFor("unknown") returns empty string with no error. This is a silent failure — callers get empty string and must handle it. The severity is actually HIGH not medium because this silently causes model/key lookups to fail with no error signal, potentially leading to runtime failures downstream when code tries to use an empty API key or model name."

#### 109. 🔴 sentry: sync.Once prevents re-initialization; single Initialize() call error not retried
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** bug
- **Локация:** `backend/pkg/sentry/sentry.go:31-61`
- **Код:** `var once sync.Once at line 16; once.Do(func() { err = sentry.Init(...) })`
- **Влияние:** If sentry.Init() fails on first call to Initialize(), future calls are no-ops and return the original error, not the current state. This prevents recovery if Sentry DSN becomes available later. Also, Initialize() can be called multiple times; only first call executes, but if Sentry is disabled on first call, enabling it later has no effect.
- **Фикс:** Remove sync.Once. Either initialize Sentry eagerly in main (not lazily), or track initialization state as a field: if s.initialized { return nil } before calling sentry.Init().
- **Проверка:** The code uses a package-level `sync.Once` guard (line 16) that persists across all `sentryClient` instances. The Initialize() method (lines 31-61) calls once.Do() with a closure that may return early if Sentry is disabled (line 36-38). Because `once` is shared across instances, if the first Initialize() call has `s.cfg.Enabled == false`, the closure executes and returns early; all subsequent Initialize() calls from any sentryClient instance are no-ops. This prevents Sentry initialization even if a later sentryClient instance has `Enabled: true`. The finding is accurate. Severity is high (not just medium) because it can silently break Sentry integration depending on instantiation order and config state."

#### 110. 🟠 jobrunner: No test coverage for AsyncRunner or critical Wait() race condition
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** testing
- **Локация:** `backend/pkg/jobrunner/jobrunner.go:1-75`
- **Код:** `No jobrunner_test.go file exists. AsyncRunner has zero test coverage.`
- **Влияние:** Critical concurrency primitive has no tests. The Wait() method at lines 52-64 has potential race: if all goroutines complete and close(done) happens before <-ctx.Done() enters select, the unbuffered channel is closed but select still unblocks—appears safe but untested. More critically: Go() spawns goroutines that use context.WithoutCancel() which ignores signal cancellation; Wait() uses separate wg.Wait() pattern that doesn't guarantee queued jobs complete before timeout.
- **Фикс:** Create jobrunner_test.go with tests for: (1) concurrent Go/Wait with timeout, (2) panic recovery, (3) proper cleanup on ctx.Done(), (4) verify jobs are actually awaited and don't leak.
- **Проверка:** The lack of test coverage is confirmed (no jobrunner_test.go exists). However, the Wait() race condition claim is incorrect—the code correctly implements the receive-from-closed-channel pattern which is safe in Go. The reviewer misunderstands this as a race when it's actually sound idiom. The real issues are: (1) AsyncRunner uses context.WithoutCancel() on line 30, which means parent context cancellation is silently ignored—jobs won't respect parent cancellation signals, only individual timeouts; (2) Wait() returning due to ctx.Done() doesn't actually stop the background jobs, they keep running. These are design concerns, not race conditions. Test coverage is genuinely missing and valuable, so medium severity for untested concurrency code is fair, but the specific Wait() race claim is incorrect.

#### 111. 🟠 jwt: RFC 7519 clock skew not handled; no iat validation
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `backend/pkg/jwt/jwt.go:13-24`
- **Код:** `IssuedAt is set but never validated in Parse(). No clock skew tolerance or iat-in-future check.`
- **Влияние:** A token with IssuedAt far in the future is accepted. Attackers could issue tokens with exp in the distant future. No test validates iat correctness. golang-jwt library does not validate iat by default.
- **Фикс:** In Parse(), after parsing, validate claims.IssuedAt is not after time.Now() (allow small skew like 1min). Add test: TestParse_FutureIssuedAt to verify rejection.
- **Проверка:** The code at lines 27-42 (Parse function) does not validate the IssuedAt claim. The golang-jwt/jwt library only automatically validates ExpiresAt; iat validation requires explicit calls to claims.Valid() or manual validation. A token with IssuedAt far in the future would be accepted. The finding's line numbers cite 13-24 (the Sign function) but the actual vulnerability is in Parse() at lines 27-42. The severity is fair for medium because while this is a real issue, it only matters if the application's threat model requires iat validation; the more critical expiration check is performed automatically by the library.

#### 112. 🟡 main.go: Deferred pool.Close() inside conditional will not execute on nil pool
- **Severity:** high → low  ·  **Verdict:** exaggerated  ·  **Category:** bug
- **Локация:** `backend/cmd/server/main.go:82-86`
- **Код:** `pool, err := db.NewPool(...); if err != nil { log.Warn(...) } else { defer pool.Close() }`
- **Влияние:** If database is unavailable at startup, the else branch is skipped and no defer is set. But then at line 108 and beyond, buildLinkbuilding() receives a nil pool. This works because nil checks exist downstream, but it's a latent crash waiting to happen if any path tries to use pool without nil check.
- **Фикс:** Set defer outside the conditional: pool, err := db.NewPool(...); if pool != nil { defer pool.Close() } OR check in buildLinkbuilding() explicitly and return error instead of silently degrading.
- **Проверка:** Lines 82-86 show: pool is declared and db.NewPool() is called. If err != nil, the warn log is executed but defer pool.Close() is NOT set (it's only in the else block, line 86). If db.NewPool() fails, pool remains nil. The pool is passed to buildLinkbuilding() at line 108. However, the actual risk is minimal because buildLinkbuilding() has an explicit nil check at line 275: if pool == nil || wordpressRepo == nil { return ... }, and no code path dereferences pool without this check first. The reviewer's claim of "a latent crash waiting to happen if any path tries to use pool without nil check" is false — the nil checks exist and work correctly. The real (minor) issue is that if db.NewPool() succeeds but an error occurs in the else block (lines 87-105), the pool might not be closed properly if that error is never handled, but the code doesn't error out in those lines. This is low severity, not high.

#### 113. 🟡 logger: Dual logging system (zerolog + slog) have decoupled log level state
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** complexity
- **Локация:** `backend/pkg/logger/logger.go:23-43`
- **Код:** `Init() calls both zerolog.SetGlobalLevel(zl) and slogLevel.Set(sl). SetLevel() does the same. Two independent log level variables.`
- **Влияние:** Log levels can diverge: zerolog and slog may log at different levels if only one path is updated. CurrentLevel() only reads slogLevel, not zerolog level. New slog logger created with NewSlog() uses slogLevel; New() creates zerolog logger—they use different level variables. Calling SetLevel() updates both, but if code only reads one, state is inconsistent.
- **Фикс:** Pick one logging strategy: either use zerolog exclusively, or slog exclusively. If both must coexist, sync them: encapsulate both in a type that ensures level changes are atomic.
- **Проверка:** The code at lines 23-43 does have two independent level variables: zerolog.SetGlobalLevel() (lines 30, 41) and slogLevel.Set() (lines 31, 42). However, the severity is overstated. The zerolog global level is write-only - it's set but never read back anywhere in the codebase (grep shows only the two SetGlobalLevel calls). CurrentLevel() (line 47) only reads slogLevel, making it the source of truth for the application's level API. Both are synchronized when SetLevel() is called. The slog logger is also the default logger (slog.SetDefault), making it the primary logging path. While having two independent level variables is technically a code smell, the practical impact is negligible because: (1) they're always updated together, (2) only slogLevel's state is queried by the application, and (3) the zerolog level doesn't affect observable behavior. This is a minor architectural inconsistency rather than a functional bug.

#### 114. 🟡 validate: Custom nonzero_uuid validator not exported; hidden test-only functionality
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `backend/pkg/validate/validate.go:23-34`
- **Код:** `Validation registered in init() via _ = v.RegisterValidation("nonzero_uuid", ...). Validator uses reflect.Len() without bounds check.`
- **Влияние:** Custom validator is internal and only discoverable via tests. Field checking loop at line 28-32 iterates reflect.Len() times but doesn't validate field is actually a [16]byte UUID. If field is [32]byte or wrong type, validator silently passes or fails unpredictably.
- **Фикс:** Rename to exported Nonzero, add explicit type check: if field.Kind() != reflect.Array || field.Type() != reflect.TypeOf([16]byte{}) { return false }
- **Проверка:** The code at lines 23-34 registers a "nonzero_uuid" custom validator that checks if a field is a 16-element array with at least one non-zero byte. Line 25 explicitly validates field.Kind() == reflect.Array AND field.Len() == 16, so there IS a bounds check—the reviewer's claim of "no bounds check" is inaccurate. However, the validator lacks explicit type checking for [16]byte specifically; it will accept any 16-element array type. The actual issue is weaker than stated: the validator will work correctly on [16]byte fields but may incorrectly validate other 16-element array types. The "silently passes or fails unpredictably" claim is overstated—the loop logic is sound given the bounds validation at line 25.

#### 115. 🟡 main.go: buildLinkbuilding() receives nil pool without documented error handling contract
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** api
- **Локация:** `backend/cmd/server/main.go:236-242, 275`
- **Код:** `buildLinkbuilding(..., pool *pgxpool.Pool, ...) at line 236; called at line 108 with pool that may be nil; at line 275: if pool == nil || wordpressRepo == nil { return qualifySvc, loginSvc, nil, runner }`
- **Влияние:** Function signature doesn't indicate pool can be nil. Parameter is *pgxpool.Pool with no nil guard in signature. Line 275 silently degrades when pool is nil, but earlier lines 243-248 don't nil-check pool before passing to sheets.NewWebsiteSource(). If sheets init fails and pool is nil, lbRunner is leaked (allocated but never awaited).
- **Фикс:** Either: (1) Add error return to buildLinkbuilding() and propagate, or (2) Document that pool can be nil and add explicit check at start of function: if pool == nil { return nil, nil, nil, nil }, or (3) Make pool non-optional and fail fast.
- **Проверка:** The code review misread the actual nil concern. Pool CAN be nil when passed to buildLinkbuilding (valid concern), and there is a nil check at line 275 for the backlinks path. However, the reviewer's specific claim about "lbRunner leaked" is incorrect: the runner is allocated at line 248, AFTER the early return at line 244-246 when sheets.NewWebsiteSource fails. So if sheets fails, lbRunner is never allocated — there is no leak. Additionally, the reviewer claims pool is "passed to sheets.NewWebsiteSource()" at line 243, but it is not — the function only receives ctx, three config parameters, and log. The lack of nil guards earlier is moot because pool isn't used before the explicit check at line 275. The actual issue is merely that the function signature doesn't document that pool can be nil and early returns with nil services when pool is unavailable, but this is handled correctly by the callers (line 150-157 shows proper nil-checks on lbRunner). Adjusted severity: low.

#### 116. 🟡 jwt: ErrInvalidToken wrapping loses original error context in some paths
- **Severity:** low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `backend/pkg/jwt/jwt.go:35-39`
- **Код:** `if err != nil { return "", fmt.Errorf("%w: %v", ErrInvalidToken, err) }`
- **Влияние:** Error formatting is non-standard: %w and %v on same error. This causes double-wrapping and ugly output: "invalid token: token signature is invalid". The %w should be used alone. Plus, line 38-39 returns ErrInvalidToken again if Subject is empty, which is redundant since all Parse errors should return ErrInvalidToken.
- **Фикс:** Use: return "", fmt.Errorf("invalid token: %w", err) on line 36. Remove separate check at 38-39; let jwt.ParseWithClaims error be the source of truth.
- **Проверка:** Line 36 uses fmt.Errorf("%w: %v", ErrInvalidToken, err) which is a valid Go idiom combining error wrapping (%w for chain semantics) with formatting (%v for readable output). Lines 38-39 check for empty Subject, which is a separate business logic validation—not redundant with the parse error check. The two checks handle different error conditions: JWT validation failures vs. empty required field. No bug here.


### CROSS-CUTTING: concurrency & error handling  (13)

#### 117. 🔴 Silent error swallowing in auto-publish path causes failed publishes to go unnoticed
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:237-242`
- **Код:** `if settings.autoPublish {
			if !checkPassed {
				log.WarnContext(ctx, "auto-publish blocked: originality check failed")
			} else {
				_, _ = s.Publish(ctx, articleID)
			}
		}`
- **Влияние:** If Publish() fails (network errors, database errors, publisher service down), the error is silently discarded. The operator has no visibility into why auto-publish failed. The article gets generated but never published despite the expectation.
- **Фикс:** Log the error and consider retrying or re-queueing: `if err := s.Publish(ctx, articleID); err != nil { log.ErrorContext(ctx, "auto-publish failed", "article_id", articleID, "err", err) }`
- **Проверка:** Line 241 calls s.Publish(ctx, articleID) which returns (articles.Article, error) per line 389, but both return values are discarded with `_, _`. The Publish method can fail with network/database/publisher service errors (lines 410-417), but these failures are silently ignored in the auto-publish path. The internal logging within Publish provides some visibility, but this represents a control flow problem: the article is left in a generated state without the expected publish side effect, and the runGeneration function has no signal that auto-publish failed. The finding accurately identifies this as a silent error swallowing issue where publish failures go unnoticed at the orchestration level.

#### 118. 🔴 HTTP response write errors silently swallowed in response.WriteJSON encoding
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/response/response.go:11-18`
- **Код:** `func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log := logger.New(ctx, "response")
		log.Error().Err(err).Msg("encode response failed")
	}
}`
- **Влияние:** While this does log the error, it's a warning symptom: after WriteHeader() is called, the status code cannot be changed. If encoding fails, the client receives a success header but no body—bad user experience. The function should fail fast before calling WriteHeader().
- **Фикс:** Encode to a buffer first, check for errors, then write headers and buffer in one operation to ensure atomicity.
- **Проверка:** Code at lines 11-18 implements WriteJSON by calling w.WriteHeader(status) before attempting to encode the body. Once WriteHeader() is called, the HTTP status code is committed to the response and cannot be changed. If json.NewEncoder(w).Encode(body) fails on line 14, the error is logged but the function silently returns, leaving the client with the success status code but no body. This violates HTTP atomicity — either status+body should succeed together, or both should fail and allow the caller to send an error response. The fix of pre-encoding to a buffer before WriteHeader() is sound.

#### 119. 🔴 Unbuffered resultsCh in qualifyAll risks goroutine leak if receiver blocked
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** goroutine-leaks
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:122-147`
- **Код:** `resultsCh := make(chan domain.Result)
...
go func() {
	for _, w := range sites {
		select {
		case <-ctx.Done():
			wg.Wait()
			close(resultsCh)
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(w domain.Website) {
			defer wg.Done()
			defer func() { <-sem }()
			if res, ok := s.qualifyOne(ctx, log, w, candidates, accepted, classifier); ok {
				resultsCh <- res
			}
		}(w)
	}`
- **Влияние:** If resultsCh reader goroutine blocks or exits early (rare but possible on error), sender goroutines get stuck on `resultsCh <- res`. The WaitGroup will never drain because senders are blocked waiting for the channel, creating a deadlock or leak.
- **Фикс:** Use a buffered channel: `resultsCh := make(chan domain.Result, maxConcurrentSites)` to ensure senders can complete even if receiver blocks.
- **Проверка:** The code at lines 122-147 creates an unbuffered channel `resultsCh := make(chan domain.Result)` and spawns worker goroutines that send on it via `resultsCh <- res` (line 141). The receiver loop is at line 163 in the same function. The finding is accurate: if the main execution path encounters an error or panic before reaching line 163, or if the receiver exits early, sender goroutines will block indefinitely on the unbuffered channel send. The WaitGroup cannot complete because senders are blocked waiting to send their results. The suggested fix (buffering the channel to maxConcurrentSites=2) is correct and would prevent this deadlock scenario.

#### 120. 🔴 Background job errors not surface-visible to operator when publish fails async
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:154-156`
- **Код:** `s.runner.Go(ctx, func(bg context.Context) {
	s.runGeneration(bg, jobLog, id, settings)
})`
- **Влияние:** The entire article generation pipeline runs in a background job via AsyncRunner. If generation fails (which it does, as seen in runGeneration at line 226-234), the only notification is a log line. An operator monitoring article status via the /articles endpoint will see the article status transition from generating → failed, but has no alerting mechanism or structured error data in the API response.
- **Фикс:** Store failure reasons in the article entity (already there as competitor_data and check_result fields are JSON), expose them in the GET /articles/{id} endpoint so clients can poll and react to failures.
- **Проверка:** At lines 154-156 in service.go, the code spawns a background job via s.runner.Go() that calls s.runGeneration(). When runGeneration encounters an error in the pipeline (line 226), it logs the error (line 228) and calls s.repo.MarkFailed() to transition status to "failed" (line 231). However, MarkFailed() (article_repository.go:123-129) only updates the status column and does not accept or store any error details. The Article entity has no field for storing error reasons, and the HTTP handler's toArticle() function (articles.go:177-214) does not expose any error data. Operators can only observe status=failed via polling /articles/{id}, with no structured error details available. The finding is accurate: background job failures are not surfaced with their reasons. The severity is correctly high since this breaks observability of the entire generation pipeline. However, the proposed fix suggesting CompetitorData and CheckResult fields "already there" is misleading - those fields store specific pipeline outputs (competitor analysis and originality scores), not general failure reasons. A proper fix would require adding a dedicated error field to the Article entity and repository."

#### 121. 🔴 Context without cancellation in AsyncRunner creates unresponsive background jobs
- **Severity:** medium → high  ·  **Verdict:** confirmed  ·  **Category:** context-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/jobrunner/jobrunner.go:29-39`
- **Код:** `func (r *AsyncRunner) Go(parent context.Context, fn func(context.Context)) {
	base := context.WithoutCancel(parent)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx := base
		var cancel context.CancelFunc
		if r.timeout > 0 {
			ctx, cancel = context.WithTimeout(base, r.timeout)
			defer cancel()
		}`
- **Влияние:** Jobs run on a context detached from cancellation signals (WithoutCancel). If the parent context is cancelled (server shutdown, request timeout), jobs continue running and ignore the signal. Only the timeout (if configured) stops them. This can cause graceful shutdown to hang waiting for jobs that won't respond to cancellation.
- **Фикс:** Use `context.WithTimeout(parent, r.timeout)` to inherit cancellation from parent while still adding timeout. Or, if you must detach from cancellation, make it explicit and add shutdown hooks.
- **Проверка:** The code at lines 29-39 correctly detaches jobs from parent cancellation via `context.WithoutCancel(parent)` on line 30. Jobs only respond to their configured timeout (line 37), not to parent context cancellation. If timeout is 0 or not configured, jobs run indefinitely and ignore shutdown signals. The `Wait()` method will block indefinitely waiting for such jobs. This breaks graceful shutdown and is a real, high-severity design flaw. The fix is accurate: use `context.WithTimeout(parent, r.timeout)` to inherit parent cancellation. The original reviewer was not exaggerating; severity should be high, not medium.

#### 122. 🟠 HTTP response write errors silently swallowed in problem.Write and response.WriteJSON
- **Severity:** high → medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/problem/problem.go:65-69`
- **Код:** `func (p *Problem) WriteTo(w http.ResponseWriter) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}`
- **Влияние:** If the client closes the connection during the write, or if Encode fails, the error is discarded. This can hide I/O issues and makes it impossible to monitor response delivery failures.
- **Фикс:** Return the error from WriteTo: `return json.NewEncoder(w).Encode(p)`. Update callers to handle the error.
- **Проверка:** The WriteTo function at lines 65-69 does have error-handling issues: it returns void and discards the error from json.Encode with _ =. The wrapper function Write() also discards return value, and all 40+ callers throughout the codebase ignore errors. This is real and verified. However, severity should be medium not high — while it prevents error monitoring, it's not a critical failure path; write failures would typically be caught by connection-level error handling. The pattern is common in Go HTTP handlers but still represents a monitoring gap.

#### 123. 🟠 Context deadline not respected when flushing results in background job
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** context-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/service.go:149-160`
- **Код:** `batch := make([]domain.Result, 0, resultFlushBatch)
flush := func() {
	if len(batch) == 0 {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resultWriteTimeout)
	defer cancel()
	if err := s.sites.WriteResults(writeCtx, sheet, batch); err != nil {
		log.ErrorContext(ctx, "write results failed", "err", err, "batch", len(batch))
	}
	batch = batch[:0]
}`
- **Влияние:** The flush function creates a new context with `context.WithoutCancel(ctx)`, detaching from the parent's cancellation. If the server is shutting down and ctx is cancelled, WriteResults will still have 30 seconds to complete (resultWriteTimeout), potentially blocking the shutdown. The job should respect the parent context's cancellation.
- **Фикс:** Use `context.WithDeadline(ctx, time.Now().Add(resultWriteTimeout))` instead to inherit parent cancellation while adding a timeout.
- **Проверка:** The code at lines 154 uses `context.WithoutCancel(ctx)` which detaches the flush context from parent cancellation, then applies a 30-second timeout. This is accurate — WriteResults will not be interrupted if the parent context is cancelled during a flush operation. The context explicitly ignores parent cancellation. The severity is fairly assessed as medium: it can delay shutdown, but the issue is constrained to active write operations, not indefinite blocking.

#### 124. 🟠 Sheets client timeout bypasses request context propagation
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** context-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/sheets/client.go:61-68`
- **Код:** `func (c *client) Lookup(ctx context.Context, topic string) (articles.Cluster, error) {
	topic = normalize(topic)
	if topic == "" {
		return articles.Cluster{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()`
- **Влияние:** The Lookup method always imposes a 10-second timeout, even if the caller has a shorter deadline (e.g., remaining time in a 5-minute job timeout). This can mask context deadline exhaustion in the caller.
- **Фикс:** Check if ctx already has a deadline shorter than 10s and respect it: `d, ok := ctx.Deadline(); if !ok || d.After(time.Now().Add(10*time.Second)) { ctx, cancel = context.WithTimeout(ctx, 10*time.Second); defer cancel() }`
- **Проверка:** The code at lines 67-68 unconditionally wraps the context with a 10-second timeout via context.WithTimeout(ctx, 10*time.Second), with no prior check for an existing, shorter deadline. The finding is accurate: if a caller provides a context with a 5-second remaining deadline (from an outer timeout), this method silently extends it to 10 seconds, violating deadline inheritance expectations. The timeout is then passed to c.svc.Spreadsheets.Values.Get() at line 79, so it does propagate to the actual API call. The suggested fix (checking ctx.Deadline() first) is the correct Go pattern for respecting caller-supplied deadlines. Medium severity is appropriate—this is a real context semantics bug that can cause timeout boundary violations in orchestration scenarios, though the practical impact depends on caller patterns.

#### 125. 🟠 Graceful shutdown timeout doesn't account for database pool drain time
- **Severity:** low → medium  ·  **Verdict:** confirmed  ·  **Category:** context-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/cmd/server/main.go:144-157`
- **Код:** `shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownWaitTimeout)
defer cancel()

if err := srv.Shutdown(shutdownCtx); err != nil {
	log.Error().Err(err).Msg("graceful shutdown failed")
}
for name, rn := range map[string]*jobrunner.AsyncRunner{"articles": runner, "linkbuilding": lbRunner} {
	if rn == nil {
		continue
	}
	if err := rn.Wait(shutdownCtx); err != nil {
		log.Warn().Str("runner", name).Err(err).Msg("background jobs did not drain before timeout")
	}
}`
- **Влияние:** The shutdownCtx applies to both HTTP server shutdown AND background job draining sequentially. The HTTP server gets the first slice of the timeout, and jobs get whatever is left (potentially very little). If HTTP shutdown takes 5s and the timeout is 10s, jobs only get 5s to finish, even if they had more time.
- **Фикс:** Create separate timeout contexts for HTTP shutdown and job draining, or use the same context but with a longer timeout to account for both phases.
- **Проверка:** The code at lines 144-157 creates a single shutdownCtx with timeout on line 144, then passes it to srv.Shutdown() on line 147 and later to rn.Wait() on lines 150-157. Since context.WithTimeout() sets an absolute deadline (not a relative duration), both operations share the same deadline. HTTP server shutdown executes first and consumes time from the deadline; job draining then receives the same context with whatever time remains. If HTTP shutdown takes significant time, job runners have correspondingly less time to drain. This is a real sequential timeout-sharing problem. Severity adjusted to medium (not low) because graceful shutdown timeouts are operationally important for production — slow HTTP shutdowns can starve background jobs of draining time, potentially leaving in-flight work incomplete.

#### 126. 🟠 Incomplete error recovery in checkAndHumanize loop hides AI score escalation
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:337-387`
- **Код:** `for cycle := 1; cycle <= maxCycles; cycle++ {
	checkRes, err := s.checker.Check(ctx, content)
	if err != nil {
		log.WarnContext(ctx, "originality check failed, skipping", "cycle", cycle, "err", err)
		break
	}

	passes := checkRes.AIScore < threshold
	checkRes.Original = passes
	last = checkRes

	log.DebugContext(ctx, "check result", "cycle", cycle, "ai_score", checkRes.AIScore, "threshold", threshold, "passes", passes)

	if saveErr := s.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
		log.WarnContext(ctx, "save check result", "err", saveErr)
	}

	if passes {
		return content, last
	}
	if cycle == maxCycles {
		log.WarnContext(ctx, "max humanize cycles reached, publishing as-is", "cycles", maxCycles, "final_ai_score", checkRes.AIScore)
		break
	}

	log.DebugContext(ctx, "content flagged — humanize rewrite", "cycle", cycle, "sentences_flagged", len(checkRes.SentencesFlagged))
	rewritten, _, err := settings.client.Complete(ctx, prompt.Humanize(content, settings.keyword, checkRes.Issues, checkRes.SentencesFlagged), settings.maxTokens)
	if err != nil {
		log.WarnContext(ctx, "humanize step failed, using current content", "cycle", cycle, "err", err)
		break
	}
	content = rewritten
}`
- **Влияние:** If humanization fails on any cycle, the loop breaks and returns the last checkResult without rechecking the current content. This means the final published content's AI score is from a prior iteration, not the actual final content. The mismatch between content and recorded AI score creates audit confusion.
- **Фикс:** After each humanization, immediately re-check the score before the next iteration or at least log that the final content differs from the recorded check result.
- **Проверка:** Verified by reading lines 337-387. The code logic is: (1) check current content, save result to 'last', (2) if passes return, (3) if not, humanize the content, (4) if humanization fails break and return content + last. The bug is real: when humanization fails, the returned 'content' variable contains the rewritten content from line 378, but 'last' contains the CheckResult from checking the PRE-humanized content at line 353. This creates a mismatch between the actual returned content and its recorded AI score. For example: cycle 2 checks un-humanized content (AI score 0.85), attempts to humanize, humanization fails, returns the humanized attempt paired with the 0.85 score from the un-humanized check. The final published content's score metadata does not reflect the actual final content state."

#### 127. 🟡 HTTP debug endpoint ignores encode errors in response
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** error-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/loglevel.go:11-14, 30-31`
- **Код:** `func handleGetLogLevel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"level": logger.CurrentLevel()})
}
...
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"level": logger.CurrentLevel()})`
- **Влияние:** If encoding fails, the error is discarded without logging. This makes debugging endpoint failures silent.
- **Фикс:** Log encoding errors: `if err := json.NewEncoder(w).Encode(...); err != nil { log.Error().Err(err).Msg("encode response failed") }`
- **Проверка:** Confirmed: Both handleGetLogLevel (line 13) and handleSetLogLevel (line 31) discard json.NewEncoder errors with _ = ... and provide no logging. However, this is exaggerated as medium severity. These are low-risk debug/admin endpoints with trivial responses (a single string value), making encoding failures virtually impossible. The poor error handling practice is real but the actual business impact is negligible — appropriate severity is low, not medium.

#### 128. 🟡 Missing context deadline propagation in article generation pipeline steps
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** context-handling
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/articles/service.go:246-335`
- **Код:** `func (s *Service) pipeline(ctx context.Context, log *slog.Logger, articleID int64, settings spec) (bool, error) {
	log.DebugContext(ctx, "step 1/5: serp competitors", "limit", s.defaults.SERPLimit)
	serpData, err := s.serp.GetSERP(ctx, settings.keyword, settings.language, s.defaults.SERPLimit)
	...
	log.DebugContext(ctx, "step 2/5: brief", "competitors", len(competitors.Items), "target_keywords", len(settings.cluster.Keywords))
	brief, _, err := settings.client.Complete(ctx, prompt.Brief(...))`
- **Влияние:** The pipeline makes 5 sequential LLM calls and external API calls. If the job timeout is 5 minutes (as seen in main.go line 93), each call could theoretically take the full timeout, causing the overall job to exceed its budget. No intermediate deadlines are set, so the early steps could starve later steps.
- **Фикс:** Add deadlines to major pipeline stages: `ctx, cancel := context.WithTimeout(ctx, timeout); defer cancel()` before each high-latency operation to ensure time budget is distributed.
- **Проверка:** The context deadline IS propagated to the pipeline function via jobrunner.Go() at line 37 of jobrunner.go, which wraps the entire job with context.WithTimeout(base, r.timeout). All 5 pipeline stages (GetSERP, Complete calls, checkAndHumanize, RenderHTML, CreateDraft, etc.) receive this parent context with the deadline already set. The reviewer's concern about "missing context deadline propagation" is inaccurate—the deadline is present. However, the reviewer's underlying point about fairness is valid: there are NO intermediate deadlines or time allocation, so early steps could consume the entire budget. This is a design concern about resource distribution, not about missing deadline propagation. Severity should be low because: (1) the deadline IS inherited by all operations, (2) context.WithTimeout already prevents indefinite hangs, and (3) while unfair distribution is theoretically possible, most well-behaved LLM/external API clients have their own internal timeouts that prevent monopolizing the budget.

#### 129. 🟡 Donor credential access pattern is racy on concurrent Site credential reads and updates
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** race
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/application/linkbuilding/backlink_service.go:223-243`
- **Код:** `func (s *BacklinkService) placeOne(ctx context.Context, log *slog.Logger, c domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) domain.PlacementResult {
	res := domain.PlacementResult{Row: c.Row, DonorURL: c.BaseURL}

	donor, ok, err := s.donors.Get(ctx, c.BaseURL)
	if err != nil {
		...
	}
	if !ok {
		appPwd, err := s.issuer.IssueAppPassword(ctx, c.BaseURL, c.Login, c.Password)
		if err != nil {
			...
		}
		donor = domain.DonorCredential{DonorURL: c.BaseURL, Login: c.Login, AppPassword: appPwd}
		if err := s.donors.Save(ctx, donor); err != nil {
			log.WarnContext(ctx, "save donor credential failed", "url", c.BaseURL, "err", err)
		}
	}`
- **Влияние:** Multiple concurrent placeOne calls for the same DonorURL will execute Get() → not found → IssueAppPassword() → Save() concurrently. Without exclusive access control, two goroutines can issue two app passwords for the same site and attempt concurrent saves, causing wasted API calls and potential account lockouts. The UPSERT in Save() will accept either one non-deterministically.
- **Фикс:** Use a sync.Mutex keyed by DonorURL to serialize credential acquisition per site, or rely on database UNIQUE constraint + CONFLICT handling with explicit error handling for duplicate key violations.
- **Проверка:** The race condition is real: concurrent PlaceBacklinks HTTP requests can trigger concurrent placeOne calls for the same DonorURL, leading to concurrent IssueAppPassword() calls and wasted API calls. However, the severity is LOW not MEDIUM, because: (1) The Save() method uses PostgreSQL's ON CONFLICT (donor_url) DO UPDATE, which is atomic and idempotent—the UPSERT guarantees data consistency; (2) only the app password issuance is duplicated (wasteful but not destructive); (3) no account lockout risk since each password is immediately persisted; (4) the in-memory access pattern (Get then conditionally Save) is not protected by locks, but the database constraint prevents corruption. A proper fix would use sync.Mutex keyed by DonorURL on the service level to prevent duplicate issuance calls, but the current code is defensible as "inefficient but correct" due to the database layer enforcement.


### CROSS-CUTTING: security & testing  (13)

#### 130. 🔴 Protected endpoints tested without auth middleware enforced
- **Severity:** critical → high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/wordpress_sites_test.go:134-143`
- **Код:** `func newWordpressRouter(repo domainwp.Repository) http.Handler {
	healthHandler := handlers.NewHealthHandler(...)
	wpSvc := appwordpress.NewService(repo)
	server := handlers.NewServer(...)
	return apihttp.NewRouter(config.ServerConfig{...}, server, nil)`
- **Влияние:** All unit tests in wordpress_sites_test.go, articles_test.go, and similar files pass `nil` as the auth middleware parameter, meaning they test handler logic WITHOUT enforcing Bearer token validation. These endpoints (CreateWordpressSite, ListWordpressSites, GetWordpressSite, UpdateWordpressSite, DeleteWordpressSite, GenerateArticle, ListArticles, etc.) are declared in the OpenAPI spec to require `bearerAuth` security, but tests never validate that requests without tokens are rejected with 401. An attacker can call protected endpoints without any token.
- **Фикс:** Modify all handler unit tests to instantiate routers with BearerAuth middleware. Create a test setup that uses a valid JWT token (e.g., from jwtauth.New("test-secret", time.Hour).Issue(...)) and attach it to requests. Add explicit test cases verifying 401 response for unauthenticated requests on protected endpoints. See loglevel_test.go (lines 34-44) for the correct pattern.
- **Проверка:** The finding is real. In wordpress_sites_test.go lines 134-143, newWordpressRouter() passes nil as authMW to apihttp.NewRouter(). The router.go code (lines 52-58) shows that when authMW is nil, the middlewares slice is empty. The BearerAuth middleware in auth.go (lines 17-21) checks for a context value that is only set by requireBearer middleware when authMW is not nil. Therefore, test requests bypass authentication entirely. The OpenAPI spec confirms /wordpress-sites endpoints declare bearerAuth security. The issue is real, but severity should be "high" not "critical" because: (1) Production code in main.go line 124 correctly passes BearerAuth middleware, so production is protected; (2) The testing gap is significant but not a live vulnerability. The fix recommended in loglevel_test.go (pass actual BearerAuth middleware) is correct.

#### 131. 🔴 Protected endpoints in integration tests run without auth middleware
- **Severity:** critical → high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/wordpress_sites_integration_test.go:26-43`
- **Код:** `func itWPServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	...
	router := apihttp.NewRouter(config.ServerConfig{
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)`
- **Влияние:** Integration tests at wordpress_sites_integration_test.go:89-160 create an HTTP test server with the router initialized with `nil` as the third auth middleware argument. This means requests to protected endpoints (POST /wordpress-sites, GET /wordpress-sites, PATCH /wordpress-sites/{id}, DELETE /wordpress-sites/{id}) are processed without any Bearer token validation. Integration tests should validate that the full stack (including auth middleware) works correctly.
- **Фикс:** Modify itWPServer to pass httpMiddleware.BearerAuth(verifier) instead of nil. Seed a user and JWT token, include Authorization header in requests. Also test that requests without tokens return 401. Apply the same fix to articles_integration_test.go and other protected endpoint integration tests.
- **Проверка:** Verified the code at wordpress_sites_integration_test.go:26-43. The itWPServer function calls apihttp.NewRouter with nil as the authMW parameter (line 43). In router.go:51-59, when authMW is nil, the mws middleware slice remains empty, so no authentication middleware is applied to any endpoint handlers. The OpenAPI spec requires bearerAuth security for all wordpress-sites endpoints (GET/POST/PATCH/DELETE). The integration test successfully executes all CRUD operations on protected endpoints without providing any Authorization headers, proving auth is bypassed. Production code correctly passes httpMiddleware.BearerAuth(verifier) to NewRouter. The severity is 'high' (not 'critical') because this is a test deficiency that doesn't directly expose users to unauthorized access, but it creates a coverage gap that could hide future auth bugs.

#### 132. 🔴 Missing explicit authorization tests for protected endpoints
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/articles_test.go:1-50`
- **Код:** `No test file found to verify that protected article endpoints enforce authentication. GenerateArticle, ListArticles, GetArticle, PublishArticle are all marked ˗security: [bearerAuth]˗ in openapi.yaml but have no unit tests validating 401 responses without tokens.`
- **Влияние:** Articles endpoints have zero evidence of auth enforcement testing. A request without an Authorization header will pass through to the handler, and the handler will process it (returning 503 if service unavailable, but not 401). This violates the declared API contract.
- **Фикс:** Create articles_test.go with test cases that verify: (1) unauthenticated requests return 401, (2) requests with invalid tokens return 401, (3) requests with valid tokens are processed. Use the same router setup pattern as loglevel_test.go.
- **Проверка:** The finding is accurate. Articles_test.go EXISTS (file found at /Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/articles_test.go, lines 1-123), but it contains zero tests validating authorization enforcement. The file constructs a router with authMW=nil (line 64), meaning no Bearer authentication middleware is active. All six test cases (TestArticles_Generate through TestArticles_PublishMissing, lines 67-122) test business logic but never validate 401 responses without tokens. The OpenAPI spec correctly declares security: [bearerAuth] for all four endpoints (confirmed in openapi.yaml). The BearerAuth middleware in auth.go lines 14-45 enforces the requirement when enabled, and loglevel_test.go lines 29-60 demonstrates the correct test pattern by including httpMiddleware.BearerAuth(jwtSvc) and validating 401 responses. This pattern is absent from articles_test.go, creating a real gap in auth enforcement testing.

#### 133. 🔴 JWT secret strength not validated; weak default accepted in non-local environments
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/config/config.go:199-240`
- **Код:** `type JWTConfig struct {
	Secret string        ˗env:"JWT_SECRET" envDefault:"dev-insecure-change-me" validate:"required"˗
	...
}

if cfg.JWT.Secret == devEncryptionKey {
	return cfg, fmt.Errorf("JWT_SECRET must be overridden outside the local environment")
}`
- **Влияние:** The code only checks string equality against the constant devEncryptionKey ("dev-insecure-change-me", 24 chars). An operator could set JWT_SECRET="x" (1 byte) in production, and the validation would pass. HMAC-SHA256 with a short secret is cryptographically weak. The validation should enforce a minimum length (e.g., 32 bytes / 256 bits). The secret is used directly as bytes in pkg/jwt/jwt.go line 20 without length validation.
- **Фикс:** Add validation in config.Load() to check that cfg.JWT.Secret is at least 32 characters long (256 bits) before accepting it. Add unit test config_test.go to verify short secrets are rejected even in local environment, and that production acceptance requires >32 char secrets.
- **Проверка:** The code at lines 199-240 of config.go correctly shows: (1) JWTConfig accepts a string secret with default "dev-insecure-change-me", (2) validation only checks string equality against this constant in non-local environments (line 235-237), NOT minimum length. The secret is converted to bytes via []byte(secret) in jwtauth.go:16 and used directly with HMAC-SHA256. The finding is accurate: an operator can set JWT_SECRET to any value (including very short strings like "x") outside local env and pass validation, resulting in cryptographically weak HMAC keys. No minimum length (e.g., 32 chars/256 bits) is enforced. Existing tests (config_test.go:171-182) do not cover short-secret rejection. Severity remains high because weak HMAC secrets directly compromise JWT authentication security."

#### 134. 🔴 No rate limiting or brute-force protection on /auth/login
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/auth.go:33-62`
- **Код:** `func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil { ... }
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { ... }
	token, expiresAt, err := h.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, appauth.ErrInvalidCredentials) {
			log.Warn().Str("email", body.Email).Msg("login failed")
			problem.Write(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		...
	}
}`
- **Влияние:** The /auth/login endpoint logs failed attempts but imposes no rate limit. An attacker can brute-force credentials by submitting unlimited login attempts. No exponential backoff, IP-based throttling, or account lockout is implemented. bcrypt.CompareHashAndPassword is inherently slow (~100ms per attempt), but without rate limiting, an attacker can still attempt thousands of passwords per day.
- **Фикс:** Implement rate limiting: (1) per-IP limit (e.g., 5 failed logins per 15 minutes), (2) per-user limit (e.g., 10 failed logins per hour, trigger account lockout), or (3) simple sliding-window rate limiter on the /auth/login handler using a package like github.com/juju/ratelimit. Add test validating that repeated failed logins return 429 (Too Many Requests) after threshold.
- **Проверка:** Code at lines 33-62 in auth.go implements the Login handler with no rate limiting. The endpoint accepts unlimited login attempts, logs failures with email address visible, but imposes no throttling, IP-based blocking, or account lockout. All middleware (Sentry, CORS, logging) are monitoring/utility only. The BearerAuth middleware does not apply to /auth/login. The finding is confirmed: this is a real brute-force vulnerability. Severity remains HIGH because combined with credential exposure in logs (email addresses), an attacker can enumerate accounts and attempt password guessing. The bcrypt delay (~100ms) provides minimal defense. Rate limiting should be implemented before production deployment.

#### 135. 🔴 No HTTPS enforcement or security headers (TLS, HSTS, CSP, X-Frame-Options)
- **Severity:** high  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/router.go:20-61`
- **Код:** `func NewRouter(cfg config.ServerConfig, api oapigen.ServerInterface, authMW oapigen.MiddlewareFunc) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: allowCredentials,
		MaxAge:           300,
	}))
	...`
- **Влияние:** The router does not set any security headers: no Strict-Transport-Security (HSTS), no X-Frame-Options (clickjacking protection), no X-Content-Type-Options (nosniff), no Content-Security-Policy. The server runs on HTTP by default (cmd/server/main.go line 126-131 creates http.Server without TLS). If deployed behind a proxy, internal traffic is unencrypted. Credentials (JWT, API tokens) are sent in Authorization headers, vulnerable to MITM if TLS is not enforced.
- **Фикс:** Add security header middleware to router (before CORS): Set X-Frame-Options: deny, X-Content-Type-Options: nosniff, X-XSS-Protection: 1; mode=block, Strict-Transport-Security: max-age=31536000; includeSubDomains (if TLS enabled). Add TLS support to cmd/server/main.go (read cert/key from env or files). Add security header tests in http/router_test.go.
- **Проверка:** The code at /Users/user/work/multiagent-seo/backend/internal/infrastructure/http/router.go (lines 21-61) creates a chi router with middleware for error recovery, request logging, CORS, and Sentry, but does NOT set any security headers (X-Frame-Options, X-Content-Type-Options, X-XSS-Protection, HSTS, CSP). The server in /Users/user/work/multiagent-seo/backend/cmd/server/main.go (lines 126-138) creates an http.Server with no TLS configuration and calls ListenAndServe(), meaning it runs plain HTTP. The ServerConfig has no TLS fields. No environment variables for TLS exist. The application transmits JWT Bearer tokens and API tokens in Authorization headers over unencrypted HTTP. This is a real and accurate security finding.

#### 136. 🟠 No race condition testing; AsyncRunner goroutines untested under concurrent load
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/jobrunner/jobrunner.go:29-49`
- **Код:** `func (r *AsyncRunner) Go(parent context.Context, fn func(context.Context)) {
	base := context.WithoutCancel(parent)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// ... job execution
	}()
}`
- **Влияние:** The AsyncRunner spawns goroutines without limit and relies on WaitGroup synchronization. No tests use -race flag to detect data races. The jobrunner_test.go (if it exists) does not verify concurrent job submission, cancellation, or timeout semantics. Potential races: (1) wg.Add/Done mismatch under panic, (2) context cancellation race with job start, (3) timeout channel close race.
- **Фикс:** Add jobrunner_test.go with: (1) TestAsyncRunner_RaceCondition running 100+ concurrent Go calls with -race flag, (2) TestAsyncRunner_PanicRecovery submitting jobs that panic and verifying wg.Done is called, (3) TestAsyncRunner_TimeoutCancellation verifying context.WithTimeout actually cancels long-running jobs. Run all tests with `-race` flag in CI.
- **Проверка:** The code at lines 29-49 does spawn goroutines without limit and lacks concurrent testing. However, the finding incorrectly claims a potential "wg.Add/Done mismatch under panic." The code is actually SAFE: r.wg.Add(1) is called before spawning (line 31), and defer r.wg.Done() is the first defer in the goroutine (line 33), guaranteeing it executes even on panic. The panic recovery handler (lines 40-47) is correctly placed AFTER the wg.Done() defer in declaration order, so Done() always executes first. No race condition exists there. The legitimate concerns are: (1) no jobrunner_test.go file exists, so no concurrent load testing or -race flag testing is done; (2) no tests verify panic recovery or timeout semantics. The severity should be medium (lack of concurrent testing) not high (no actual data races in the synchronization logic itself).

#### 137. 🟠 Database credentials in logs and error messages
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/db/pool.go:13-28`
- **Код:** `func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	log := logger.New(ctx, "infrastructure.db")
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	log.Info().Str("host", cfg.Host).Str("dbname", cfg.Dbname).Msg("connected to database")`
- **Влияние:** The error returned from pgxpool.New(ctx, cfg.DSN()) may contain the full DSN including password (from config.DatabaseConfig.DSN() line 41-52 in config.go). If a connection error occurs, it propagates to cmd/server/main.go line 83, logged by log.Warn(). The database password could be exposed in logs, error reports, or Sentry. Line 26 only logs host and dbname (safe), but errors are not sanitized.
- **Фикс:** Sanitize the DSN before passing to pgxpool.New or wrap the error to remove sensitive data. Create a helper function sanitizeDSN(dsn) that returns "postgres://user@host/dbname" without password. Wrap pgxpool errors: if err != nil { return nil, fmt.Errorf("failed to create connection pool: %w", err) }. Add config_test.go test verifying DSN does not leak password in error messages.
- **Проверка:** The code at pool.go lines 13-28 does call pgxpool.New(ctx, cfg.DSN()) with a full DSN containing credentials (built at config.go lines 41-52 as postgres://user:password@host:port/dbname). If pgxpool.New() fails, the wrapped error at line 18 (fmt.Errorf with %w) includes the underlying pgxpool error, which may contain the full DSN including the password. This error is then logged in main.go line 84 via log.Warn().Err(err). The log statement at pool.go line 26 is safe (only logs host and dbname), but errors are not sanitized. The vulnerability is real—credentials could leak in error logs on startup failures—though exposure is limited to startup-time failures and depends on pgxpool's error message formatting.

#### 138. 🟠 Missing input validation on JWT secret; no minimum length enforced
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/pkg/jwt/jwt.go:13-25`
- **Код:** `func Sign(subject string, ttl time.Duration, secret []byte) (token string, expiresAt time.Time, err error) {
	expiresAt = time.Now().Add(ttl)
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	...
}`
- **Влияние:** The Sign function accepts secret []byte without length validation. If called with a 1-byte secret (e.g., "x"), HMAC-SHA256 will still work but is cryptographically weak (entropy ~3 bits vs. 256 bits for a random 32-byte secret). The config validation at config.go:235 only checks string equality, not length.
- **Фикс:** Add validation in pkg/jwt/jwt.go: Add jwtauth.New validation (line 15-17 in jwtauth.go): if len(secret) < 32 { return nil, errors.New("JWT secret must be 32+ bytes") }. Add jwt_test.go test: TestSign_WeakSecret validating that secrets <32 bytes are rejected or cause a warning.
- **Проверка:** The Sign function at /Users/user/work/multiagent-seo/backend/pkg/jwt/jwt.go:13-25 accepts a secret []byte parameter with zero validation. Config-level validation at config.go:235 only checks string equality against the dev default (devEncryptionKey), not minimum length. The code demonstrates that 1-byte secrets like "s" work without error (jwt_test.go:34). HMAC-SHA256 will function with any byte length but crypto entropy is severely compromised with short secrets. The finding is accurate: there is no minimum length enforcement for JWT secrets at either the function or config layer.

#### 139. 🟠 No comprehensive tests validating all protected endpoints enforce authentication
- **Severity:** high → medium  ·  **Verdict:** exaggerated  ·  **Category:** testing
- **Локация:** `/Users/user/work/multiagent-seo/backend/api/openapi.yaml:70-400`
- **Код:** `OpenAPI spec declares 23 endpoints with "security: [bearerAuth]": /users, /api-tokens/*, /wordpress-sites/*, /articles/*, /linkbuilding/*. Unit tests in handlers/*_test.go do not instantiate routers with auth middleware (nil passed). Integration tests similarly pass nil for authMW.`
- **Влияние:** A critical attack surface: 23 protected endpoints with ZERO evidence that they enforce Bearer token requirements. An unauthenticated attacker can call any protected endpoint and it will be processed. The only endpoints with explicit auth enforcement tests are /debug/log-level and /auth/login.
- **Фикс:** Create a comprehensive test suite (e.g., handlers/security_test.go) that: (1) lists all protected endpoints from openapi.yaml, (2) for each, verifies that a request without Authorization header returns 401, (3) verifies that a request with a malformed or invalid token returns 401, (4) verifies that a request with a valid token succeeds. Use a test helper that sets up router with auth middleware and generates valid tokens.
- **Проверка:** Authentication IS enforced for protected endpoints, but at the handler level rather than middleware level in unit tests. Protected endpoints (/users at auth.go:64-81, /api-tokens at apitokens.go:36-90, /articles, /wordpress-sites) all call UserIDFromContext() and return 401 if the user context is missing. The generated OpenAPI code sets BearerAuthScopes context for protected endpoints. However, the finding's concern has merit: (1) Unit tests pass authMW=nil to NewRouter(), bypassing middleware enforcement and relying solely on handler-level checks; (2) This creates an architectural fragility—if a handler forgets the UserIDFromContext() check, the endpoint would be exposed in tests. The vulnerability is NOT present in production (main.go provides real authMW), but the testing pattern is brittle. Severity should be medium (architectural risk in testing), not high.

#### 140. 🟠 No audit logging for sensitive operations (login, token creation, deletion)
- **Severity:** medium  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/handlers/auth.go:49-62`
- **Код:** `if err != nil {
	log := logger.New(r.Context(), "handlers.auth")
	if errors.Is(err, appauth.ErrInvalidCredentials) {
		log.Warn().Str("email", body.Email).Msg("login failed")
		...`
- **Влияние:** Login failures are logged with the email address, but successful logins are not logged at all. API token creation and deletion are also not logged. No audit trail exists for account access or token lifecycle events. An attacker could create API tokens and use them without leaving an audit trail.
- **Фикс:** Add audit logging for: (1) successful login attempts (with user ID, timestamp, IP address), (2) API token creation (who, when, token ID), (3) API token deletion/revocation (who, when, token ID). Log to a dedicated audit stream. Add integration test verifying audit logs are written.
- **Проверка:** The finding is accurate. The Login handler (lines 49-62 in auth.go) only logs failed login attempts with the email address when ErrInvalidCredentials occurs (line 53: log.Warn().Str("email", body.Email).Msg("login failed")). There is NO logging for successful logins — the successful path at line 61 simply returns the token without any audit trail. Additionally, the ApiTokensHandler (apitokens.go) shows that CreateApiToken (lines 36-71) and DeleteApiToken (lines 92-108) only log errors, not successful token creation or deletion operations. For example, line 64-70 returns the created token without logging the successful creation event. There is no dedicated audit logging system found in the codebase. The finding accurately describes a gap in audit trail coverage for login success, token creation, and token deletion events — all sensitive security operations that should be logged for accountability and forensic analysis.

#### 141. 🟡 Composite token verifier error handling and type confusion
- **Severity:** medium → low  ·  **Verdict:** exaggerated  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/cmd/server/main.go:354-368`
- **Код:** `type compositeVerifier struct {
	jwt  domainauth.TokenVerifier
	keys *appapitoken.Service
}

func (c compositeVerifier) Verify(ctx context.Context, token string) (string, error) {
	if appapitoken.HasKeyPrefix(token) {
		uid, err := c.keys.Authenticate(ctx, token)
		if err != nil { return "", err }
		return uid.String(), nil
	}
	return c.jwt.Verify(ctx, token)
}`
- **Влияние:** If a token does NOT start with "mas_", it is assumed to be a JWT and passed to c.jwt.Verify(). Error messages differ between paths (JWT vs API key errors), enabling enumeration. A random string without "mas_" prefix will fail JWT validation, while a string with prefix will fail API key validation, revealing the authentication scheme used.
- **Фикс:** Return a generic "invalid token" error from Verify regardless of token type and path. Ensure both JWT and API key failures return the same error type/message. This prevents enumeration attacks based on error message differences.
- **Проверка:** The code does return different errors: API key path returns "invalid api key" while JWT path returns "invalid token: ...". However, the middleware (auth.go line 30-36) handles both errors identically by returning HTTP 401 with generic "unauthorized" message to clients. The error messages are only logged server-side. While the finding correctly identifies that error paths differ internally, the claimed enumeration attack requires log visibility to an attacker, making the actual security impact low rather than medium. The finding's severity is overstated because HTTP responses don't expose authentication scheme information to attackers.

#### 142. 🟡 CORS MaxAge is very short; preflight caching minimal
- **Severity:** low  ·  **Verdict:** confirmed  ·  **Category:** security
- **Локация:** `/Users/user/work/multiagent-seo/backend/internal/infrastructure/http/router.go:26-32`
- **Код:** `r.Use(cors.Handler(cors.Options{
	AllowedOrigins:   cfg.CORSAllowedOrigins,
	AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
	AllowCredentials: allowCredentials,
	MaxAge:           300,
}))`
- **Влияние:** MaxAge is set to 300 seconds (5 minutes). This means browsers will re-send OPTIONS preflight requests every 5 minutes, increasing overhead. While not a security issue, it is inefficient. Max recommended value is 86400 seconds (24 hours).
- **Фикс:** Increase MaxAge to 86400 (24 hours) for production deployments. Add config parameter CORS_MAX_AGE with default 86400.
- **Проверка:** The code at /Users/user/work/multiagent-seo/backend/internal/infrastructure/http/router.go lines 26-32 does indeed set CORS MaxAge to 300 seconds. This is accurately identified as a short preflight cache duration. The finding correctly states this means browsers will re-send OPTIONS preflight requests every 5 minutes instead of caching them longer. The recommended industry standard is 86400 seconds (24 hours). This is a valid performance optimization opportunity, though correctly classified as low severity since it's an efficiency issue rather than a security vulnerability. The suggested fix to make it configurable is reasonable.

## Что реально хорошо

- Clean, deliberate architecture: domain / application / infrastructure separation is real and consistent, with interfaces at the boundaries and dependency injection wired explicitly in main.go rather than via globals.
- Idiomatic HTTP layer: chi router, RFC 9457 problem+json responses, MaxBytesReader on request bodies, generated OpenAPI server interface, and centralized middleware composition.
- Defensive degradation in production wiring: handlers and the health service nil-check their dependencies and return 503 when a service (e.g. the DB pool) is unavailable, so a missing dependency degrades gracefully rather than panicking.
- Good security primitives where they exist: bcrypt for password hashing, API-key prefix routing in the composite verifier, and webfetch correctly implements redirect capping plus a private-IP dial guard (the right pattern, just not applied to the other clients).
- Thoughtful resilience details: LLM retry layer honoring Retry-After, response-size LimitReaders capping memory, recent idempotency work (skip already-placed donors, preserve audit column), and UTF-8-aware truncation in the topic classifier.
- Readable, consistently styled code with structured logging (slog with context), meaningful log fields, and helper extraction; genuinely easy to follow and review, which is why the systemic issues are fixable rather than buried.
- The error-handling anti-pattern, while pervasive, is mechanical and uniform, meaning a single disciplined pass (propagate instead of warn-and-continue) plus a -race CI gate would eliminate the majority of high-severity findings.

## План действий

- WEEK 1 - Stop silent data loss and outcome inversion (highest ROI): fix the originality-check-failure-reported-as-pass bug (service.go:353/326); make backlink/login flush() failures retry-then-loudly-fail instead of dropping pending results; propagate RenderHTML errors and fail generation on render failure; stop swallowing Publish/SaveImageStats/SaveCheckResult errors, propagate or record a terminal failure reason on the article.
- WEEK 1-2 - Close the concurrency races and add the -race gate: fix the shared goldmark renderer (per-call or pooled), guard or make-immutable Problem.ext, buffer resultsCh in qualifyAll. Create jobrunner_test.go and run the whole suite under go test -race in CI so these never silently regress.
- WEEK 2 - Security hardening for the credential boundary: add /auth/login rate limiting (429 plus lockout), enforce a 32-byte minimum JWT secret, add security-header middleware and TLS support, add CheckRedirect/private-IP guards to the pexels and dataforseo clients, and add audit logging for login success and token create/delete.
- WEEK 2-3 - Make the auth contract testable: build a shared test router that wires real BearerAuth plus a test JWT; replace authMW=nil in handler/integration tests; add explicit 401-without-token and 200-with-token cases across all protected endpoints.
- WEEK 3 - Fill the critical test gaps: unit-test sheets.Lookup, pexels.SearchN, dataforseo parsing, and the article pipeline error paths (SERP/LLM/checker/publish failures) using fakes that return errors; these are the brittle parsing seams most likely to break.
- WEEK 3-4 - Robustness of parsers and i18n: replace brittle regex/byte-truncation parsing ([IMG|...] nested brackets, wpApiSettings nonce, 20KB HTML truncation) with UTF-8 and bracket-safe handling; switch tokenize() to unicode.IsLetter/IsDigit and document or flag the English-only captcha solver.
- WEEK 4 - Context lifecycle and validation: stop using context.WithoutCancel() for jobs/batch writes that must respect shutdown (inherit parent plus add timeout); have Lookup respect a tighter caller deadline; add fail-fast validation for LLM provider names, resolved model strings, maxTokens fallback, and article Defaults at construction time.
- ONGOING - Establish a swallowed-error lint rule: ban underscore-discarded returns on Write/Encode/ReadAll/Marshal and warn-and-continue on side-effecting repo/IO calls in review; pre-allocate slice capacities and de-duplicate small helpers (isEmpty, factory wrappers) opportunistically as low-priority cleanup.

## Отброшенные (ложные) замечания

Для прозрачности — 18 замечаний отсеяны как недостоверные при проверке кодом:

- ❌ **[linkbuilding]** Race condition: resultsCh goroutine uses defer to close after wg.Wait(), but wg.Add() is called after sender goroutine goroutine is launched
  - _проверка:_ The finding is incorrect. The code is properly synchronized. Line 136 calls `wg.Add(1)` immediately before line 137-143 launches the worker goroutine with `go func() { defer wg.Done() }(w)`. The Go memory model guarantees that the `go` statement happens-before any code in the launched goroutine executes, so `wg.Add()` always completes before `wg.Done()` can run. There is no race condition here—this is the textbook *correct* way to use sync.WaitGroup with worker goroutines. The reviewer misread the code structure.
- ❌ **[linkbuilding]** Data race: batch slice reuse in flush() without synchronization
  - _проверка:_ The finding misreads the code flow. WriteResults() (lines 108-149 of websites.go) synchronously iterates over the results slice to build request data (lines 114-129), then makes a single API call using that pre-built data structure (lines 136-139). The function returns immediately after the API call completes; it does not spawn goroutines or read the results parameter asynchronously. In service.go, batch is only reset (line 159) after WriteResults() returns (line 156), so there is no concurrent access to the underlying array. The Google Sheets API call is I/O-bound but happens synchronously within WriteResults, with the slice fully consumed before any reuse.
- ❌ **[linkbuilding]** Non-atomic write creates split-brain state: qualification result writes can partially fail leaving inconsistent sheet state
  - _проверка:_ The code at lines 108-149 uses Google Sheets API BatchUpdate, which the API documentation confirms operates with all-or-nothing atomicity. If the batch fails (quota exceeded, rate limit, validation error), the entire batch is rejected and no values are written. There is no partial failure scenario where "some rows are updated with topic/outbound/suitable while other rows in the same batch are not." The finding's core premise—that non-atomic writes create split-brain state—is factually incorrect. Google's BatchUpdate either succeeds entirely or fails entirely. The code does not have custom retry logic that could introduce duplicate writes on a subsequent call. The finding mischaracterizes the API's actual behavior.
- ❌ **[http]** Undefined constant 'maxBodyBytes' used in multiple handlers
  - _проверка:_ The finding is FALSE. Line 38 in auth.go does reference `maxBodyBytes` (as r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)), BUT this is NOT a compilation error. All these files (auth.go, linkbuilding.go, apitokens.go, articles.go) are in the same `handlers` package. The constant `maxBodyBytes` is defined once at line 21 in wordpress_sites.go as `const maxBodyBytes = 64 << 10` and is package-scoped, meaning all files in the same package can access it without any import or special reference. Go's package-level constants are visible across all files in the same package. I verified this by successfully compiling both the handlers package and the full server binary with `go build` from the backend directory — both completed without errors.
- ❌ **[http]** Headers set after WriteHeader may be silently ignored
  - _проверка:_ The finding misreads the code. The actual code at lines 11-17 correctly sets the Content-Type header on line 12, then calls WriteHeader on line 13. Headers are set BEFORE WriteHeader, which is the correct order. The Content-Type will be properly sent to the client. The reviewer's evidence quotes the code accurately but then incorrectly asserts the order is wrong when it is actually correct.
- ❌ **[http]** isNil() helper with reflect adds unnecessary complexity and performance cost
  - _проверка:_ The finding mischaracterizes the code. The struct fields (h.svc, h.loginSvc, h.backlinkSvc) are interface types (lines 40-42), not typed pointers. The isNil() function correctly uses reflect to detect nil pointer values stored in interface types—a necessary pattern in Go because direct == nil comparison on an interface containing a nil pointer would be false. The suggested fix would introduce a bug. The code is correct.
- ❌ **[persistence]** ON CONFLICT clause in donor credential upsert silently fails on soft-deleted rows
  - _проверка:_ The finding misunderstands how PostgreSQL's partial unique index and ON CONFLICT work together. The code at lines 40-58 correctly implements upsert with soft-delete support: (1) The partial index on (donor_url) WHERE deleted_at IS NULL only constrains live rows. (2) The ON CONFLICT clause correctly checks the same condition. (3) The result is intentional per the migration comment: a soft-deleted URL can be reused by inserting a new row. This is not a bug—it's the designed behavior. If a row was soft-deleted, Save() will insert a new row (not update the soft-deleted one), which is correct and allows the soft-deleted row to remain in the database while the new one is active. No fix is needed."
- ❌ **[llm]** Huggingface Client: Repeated Request Body Marshal Per Retry Attempt
  - _проверка:_ Lines 108-117: body is marshaled once at line 108, then the retry loop (lines 116-140) calls bytes.NewReader(body) on each attempt. This is correct Go code. bytes.NewReader creates a fresh Reader with independent read position on each call—it does not maintain state across invocations. Each retry gets a new Reader starting at position 0, which is the intended behavior. There is no performance issue (marshaling is already outside the loop) and no fragility. The reviewer misunderstood bytes.Reader semantics—this is a standard, idiomatic pattern in Go for retryable requests. The finding is false.
- ❌ **[sheets]** Missing nil pointer check for Google Sheets API response
  - _проверка:_ The code at lines 77-89 does call the Google Sheets API and accesses resp.Values in a for loop without an explicit nil check. However, this is not a bug: (1) The Google Sheets API contract guarantees that if err == nil on line 81, the resp will be a valid *sheets.ValueRange struct, never nil; (2) Even if resp.Values were nil, Go's range operator and len() function handle nil slices gracefully without panicking—the loop would simply iterate zero times and len(resp.Values) returns 0. Lines 124 and 133 already use len(resp.Values) without issue. No nil pointer dereference is possible here.
- ❌ **[sheets]** Unreliable row number tracking in parse functions
  - _проверка:_ The finding misunderstands how the code works. In parseECredentialsJoin (line 332) and staleEStatusRows (line 369), the loop variable `i` comes from `for i, row := range values`, where `values` is the API response from Google Sheets. The Google Sheets API returns rows in sequential order starting from the requested range, so index 0 always corresponds to row 1, index 1 to row 2, etc. Using `i + 1` correctly converts from 0-indexed array position to 1-indexed spreadsheet row numbers. The `continue` statements don't affect this because `i` still increments correctly through the range operation, and the skipped rows are simply not added to the output — but the row number of any row that *is* added is still calculated correctly based on its position in the API response. The same pattern is used in List() (line 95) and is consistent throughout.
- ❌ **[external-io]** DataForSEO CustomUnmarshal for serpSubItem has unreachable code
  - _проверка:_ The UnmarshalJSON implementation at lines 59-65 is NOT dead code and is actively exercised by the test. Line 9 of client_test.go includes "Q3?" as a bare string item, which triggers the b[0] == '"' branch (line 60-61). The code correctly unmarshals string items to Title and leaves Description empty. The else branch (line 64) handles object items. Both paths are reachable and tested. The finding mischaracterizes this as unreachable/dead code when it is in fact working as designed with test coverage.
- ❌ **[pkg-cmd]** jobrunner: Nil logger panics on background job errors
  - _проверка:_ The code at lines 22-26 correctly handles nil loggers by substituting slog.Default() before storing the logger in the AsyncRunner struct. The r.log field is guaranteed to be non-nil after NewAsyncRunner returns, so the r.log.ErrorContext() call at line 42 cannot panic. The nil check actually prevents the panic risk rather than leaving it open. The finding mischaracterizes the code flow—there is no panic risk here.
- ❌ **[pkg-cmd]** apitoken: Prefix slice has no bounds check; panic on short generate
  - _проверка:_ The code at line 46 slices secret[:len(keyPrefix)+6] = secret[:10]. This is safe because generateSecret() at lines 80-85 constructs the string as keyPrefix (4 chars "mas_") concatenated with base64.RawURLEncoding.EncodeToString(32 bytes), which always produces 43 base64 characters. The total secret length is guaranteed to be 47 characters, so slicing at index 10 has no bounds risk. The finding misses that generateSecret() has a fixed, deterministic output size, not a variable one that could panic.
- ❌ **[pkg-cmd]** main.go: Signal handler defers are in wrong order; Flush may not execute
  - _проверка:_ The finding misunderstands Go defer semantics. Lines 67 and 70 set up defers in the order: Flush (line 67), then stop (line 70). In Go, defers execute in LIFO order, so at program exit: stop() executes FIRST, then Flush() executes SECOND. This is the correct order — signal handling stops before flushing Sentry events. The code is correct as-is. Additionally, stop() is explicitly called at line 141 before shutdown, and the deferred stop() at line 70 is redundant but harmless since signal.Stop() is idempotent.
- ❌ **[pkg-cmd]** config: DSN password escaping doesn't handle all special characters safely
  - _проверка:_ The finding is incorrect. The code at lines 41-52 uses url.UserPassword(c.User, c.Password) which correctly percent-encodes special characters including @ as %40. Testing confirms: password "pass@evil.com" produces "pass%40evil.com" in the DSN, and username "admin@domain" produces "admin%40domain". The existing test at config_test.go:68 already validates this with "p@ss/word" -> "p%40ss%2Fword". There is no parsing ambiguity because url.URL.String() produces a correctly-formed URI where the @ separating credentials from host is distinct from escaped @ characters in credentials. No vulnerability exists, and no validation or additional test case is needed.
- ❌ **[concurrency-errors]** Concurrent map mutation in qualifyOne results slice without synchronization
  - _проверка:_ The code at lines 186-203 allocates a pre-sized `results` slice and spawns goroutines that each write to a unique index `results[i]` where i is captured as a value parameter in the goroutine closure. Each goroutine accesses only its own distinct slice index — there is zero concurrent access to overlapping memory locations. The Go race detector would not flag this. The reviewer misread the code: they failed to notice that `i` is passed as a function parameter (not closed over), ensuring each goroutine has its own copy and writes to non-overlapping indices. This is safe concurrent programming, not a race condition.
- ❌ **[concurrency-errors]** Database query rows.Next() does not propagate context cancellation
  - _проверка:_ The code at lines 79-107 correctly handles context cancellation. While the loop at line 96 does not explicitly check ctx.Err() on each iteration, the function properly captures context cancellation via rows.Err() at line 103 after the loop exits. This is the correct and idiomatic pgx pattern in Go. When ctx is cancelled, rows.Next() returns false and stores the error, which rows.Err() retrieves. The finding misunderstands pgx's error-handling design and flags correct code as a bug.
- ❌ **[security-testing]** Logging and Sentry not redacting sensitive request/response data
  - _проверка:_ The finding is inaccurate. The code at line 63 logs only r.URL.Path, which in Go's http package contains the path portion without query parameters (e.g., /search, not /search?password=secret). Query strings are stored separately in r.URL.RawQuery and are not logged by this middleware. The concern about credentials in the path being exposed is not applicable to this code—it does not log query parameters. No redaction is needed because sensitive data in query strings is never logged in the first place.
