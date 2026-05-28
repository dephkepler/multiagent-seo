// Package config loads application configuration from environment variables.
//
// All variables share the CF_ prefix (e.g. CF_DB_HOST). Defaults declared in
// envDefault tags target a local docker-compose dev setup; real deployments
// override via the runtime environment.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v10"

	"multiagent-seo/pkg/validate"
)

const envPrefix = "CF_"

type ServerConfig struct {
	Port                 string        `env:"APP_PORT" envDefault:"8080" validate:"required"`
	Host                 string        `env:"APP_HOST" envDefault:"localhost" validate:"required"`
	BasePath             string        `env:"APP_BASE_PATH" envDefault:"/"`
	CORSAllowedOrigins   []string      `env:"APP_CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3000" envSeparator:","`
	ReadTimeout          time.Duration `env:"APP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout         time.Duration `env:"APP_WRITE_TIMEOUT" envDefault:"5m"`
	ShutdownWaitTimeout  time.Duration `env:"APP_SHUTDOWN_WAIT_TIMEOUT" envDefault:"30s"`
	BackgroundJobTimeout time.Duration `env:"APP_BACKGROUND_JOB_TIMEOUT" envDefault:"15m"`
}

type DatabaseConfig struct {
	Host          string `env:"DB_HOST" envDefault:"localhost" validate:"required"`
	Port          string `env:"DB_PORT" envDefault:"5432" validate:"required"`
	User          string `env:"DB_USER" envDefault:"postgres" validate:"required"`
	Password      string `env:"DB_PASSWORD" envDefault:"postgres" validate:"required"`
	Dbname        string `env:"DB_NAME" envDefault:"contentflow" validate:"required"`
	SSLMode       string `env:"DB_SSLMODE" envDefault:"disable" validate:"required"`
	MigrationsDir string `env:"MIGRATIONS_DIR" envDefault:"migrations"`
}

// DSN builds a libpq/pgx-compatible connection string.
func (c DatabaseConfig) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   c.Host + ":" + c.Port,
		Path:   c.Dbname,
	}
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

type SentryConfig struct {
	Enabled     bool   `env:"SENTRY_ENABLED" envDefault:"false"`
	Dsn         string `env:"SENTRY_DSN" envDefault:""`
	Environment string `env:"SENTRY_ENVIRONMENT" envDefault:"local" validate:"required"`
	Release     string `env:"SENTRY_RELEASE" envDefault:""`
}

type LLMConfig struct {
	Provider     string `env:"LLM_PROVIDER" envDefault:"groq"`
	APIKey       string `env:"LLM_API_KEY"`
	Model        string `env:"LLM_MODEL" envDefault:"llama-3.3-70b-versatile"`
	GroqAPIKey   string `env:"LLM_GROQ_API_KEY"`
	ClaudeAPIKey string `env:"LLM_CLAUDE_API_KEY"`
}

// normalizeProvider collapses the "anthropic" alias onto "claude" so both
// names resolve to the same key, including the generic-APIKey fallback.
func normalizeProvider(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "anthropic" {
		return "claude"
	}
	return s
}

func (c LLMConfig) KeyFor(provider string) string {
	p := normalizeProvider(provider)
	switch p {
	case "groq":
		if c.GroqAPIKey != "" {
			return c.GroqAPIKey
		}
	case "claude":
		if c.ClaudeAPIKey != "" {
			return c.ClaudeAPIKey
		}
	}
	if p == normalizeProvider(c.Provider) {
		return c.APIKey
	}
	return ""
}

type SheetsConfig struct {
	CredentialsFile string `env:"SHEETS_CREDENTIALS_FILE" envDefault:"./credentials.json"`
	SpreadsheetID   string `env:"SHEETS_SPREADSHEET_ID"`
	Sheet           string `env:"SHEETS_SHEET" envDefault:"Keywords"`
	TopicColumn     string `env:"SHEETS_TOPIC_COLUMN" envDefault:"A"`
	KeywordColumn   string `env:"SHEETS_KEYWORD_COLUMN" envDefault:"B"`
	TitleColumn     string `env:"SHEETS_TITLE_COLUMN" envDefault:"C"` // empty disables suggested-title lookup
	HeaderRow       bool   `env:"SHEETS_HEADER_ROW" envDefault:"true"`
}

type WorkerConfig struct {
	PollInterval time.Duration `env:"WORKER_POLL_INTERVAL" envDefault:"60s"`
}

type ArticleConfig struct {
	Language   string `env:"ARTICLE_LANGUAGE" envDefault:"ru"`
	MinWords   int    `env:"ARTICLE_MIN_WORDS" envDefault:"1500"`
	MaxWords   int    `env:"ARTICLE_MAX_WORDS" envDefault:"3000"`
	SiteTopic  string `env:"ARTICLE_SITE_TOPIC"`
	ExtraRules string `env:"ARTICLE_EXTRA_RULES"`
}

type DataForSEOConfig struct {
	Login     string `env:"DATAFORSEO_LOGIN"`
	Password  string `env:"DATAFORSEO_PASSWORD"`
	SERPLimit int    `env:"DATAFORSEO_SERP_LIMIT" envDefault:"5"`
}

type CheckerConfig struct {
	// Provider selects the checker implementation: "mock", "originality", or "huggingface".
	Provider    string  `env:"CHECKER_PROVIDER" envDefault:"mock"`
	APIKey      string  `env:"CHECKER_API_KEY"`
	AIThreshold float64 `env:"CHECKER_AI_THRESHOLD" envDefault:"0.8"`
	Model       string  `env:"CHECKER_MODEL"`
	// MaxCycles caps humanize rewrites; on overflow the article is published as-is.
	MaxCycles int `env:"CHECKER_MAX_CYCLES" envDefault:"3"`
}

type PexelsConfig struct {
	Enabled bool   `env:"PEXELS_ENABLED" envDefault:"true"`
	APIKey  string `env:"PEXELS_API_KEY"`
}

// devEncryptionKey is the shared dev default for WP_ENCRYPTION_KEY and JWT_SECRET;
// Load rejects it outside the local environment so prod can't boot on it silently.
const devEncryptionKey = "dev-insecure-change-me"

type WordPressConfig struct {
	// EncryptionKey is the pgcrypto symmetric key used to encrypt stored WordPress
	// app passwords. The default is dev-only; production MUST override it.
	EncryptionKey string `env:"WP_ENCRYPTION_KEY" envDefault:"dev-insecure-change-me" validate:"required"`
}

type JWTConfig struct {
	// Secret signs and verifies auth tokens (HS256). Dev-only default; prod MUST override.
	Secret string        `env:"JWT_SECRET" envDefault:"dev-insecure-change-me" validate:"required"`
	TTL    time.Duration `env:"JWT_TTL" envDefault:"24h"`
}

type Config struct {
	Server     ServerConfig     `validate:"required"`
	Database   DatabaseConfig   `validate:"required"`
	Sentry     SentryConfig     `validate:"required"`
	LLM        LLMConfig        `validate:"required"`
	Sheets     SheetsConfig     `validate:"required"`
	Worker     WorkerConfig     `validate:"required"`
	Article    ArticleConfig    `validate:"required"`
	DataForSEO DataForSEOConfig `validate:"required"`
	Checker    CheckerConfig    `validate:"required"`
	Pexels     PexelsConfig     `validate:"required"`
	WordPress  WordPressConfig  `validate:"required"`
	JWT        JWTConfig        `validate:"required"`
}

func Load() (Config, error) {
	cfg := Config{}

	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: envPrefix}); err != nil {
		return cfg, fmt.Errorf("parse environment variables: %w", err)
	}

	if err := validate.Validate(cfg); err != nil {
		return cfg, fmt.Errorf("config validation: %w", err)
	}

	if cfg.Sentry.Environment != "local" {
		if cfg.WordPress.EncryptionKey == devEncryptionKey {
			return cfg, fmt.Errorf("WP_ENCRYPTION_KEY must be overridden outside the local environment")
		}
		if cfg.JWT.Secret == devEncryptionKey {
			return cfg, fmt.Errorf("JWT_SECRET must be overridden outside the local environment")
		}
	}

	return cfg, nil
}
