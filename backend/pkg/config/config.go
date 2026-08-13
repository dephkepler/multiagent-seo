package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"

	"multiagent-seo/pkg/validate"
)

const envPrefix = "CF_"

type ServerConfig struct {
	Port                     string        `env:"APP_PORT" envDefault:"8080" validate:"required"`
	Host                     string        `env:"APP_HOST" envDefault:"localhost" validate:"required"`
	BasePath                 string        `env:"APP_BASE_PATH" envDefault:"/"`
	CORSAllowedOrigins       []string      `env:"APP_CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3000" envSeparator:","`
	ReadTimeout              time.Duration `env:"APP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout             time.Duration `env:"APP_WRITE_TIMEOUT" envDefault:"5m"`
	ShutdownWaitTimeout      time.Duration `env:"APP_SHUTDOWN_WAIT_TIMEOUT" envDefault:"30s"`
	BackgroundJobTimeout     time.Duration `env:"APP_BACKGROUND_JOB_TIMEOUT" envDefault:"60m"`
	BackgroundJobConcurrency int           `env:"APP_BACKGROUND_JOB_CONCURRENCY" envDefault:"3"`
}

type LoggerConfig struct {
	Level string `env:"LOG_LEVEL" envDefault:"info" validate:"required"`
}

type DatabaseConfig struct {
	Host     string `env:"DB_HOST" envDefault:"localhost" validate:"required"`
	Port     string `env:"DB_PORT" envDefault:"5432" validate:"required"`
	User     string `env:"DB_USER" envDefault:"postgres" validate:"required"`
	Password string `env:"DB_PASSWORD" envDefault:"postgres" validate:"required"`
	Dbname   string `env:"DB_NAME" envDefault:"contentflow" validate:"required"`
	SSLMode  string `env:"DB_SSLMODE" envDefault:"disable" validate:"required"`

	MigrationsDir string `env:"MIGRATIONS_DIR" envDefault:"migrations"`
}

type SentryConfig struct {
	Enabled     bool   `env:"SENTRY_ENABLED" envDefault:"false"`
	Dsn         string `env:"SENTRY_DSN" envDefault:""`
	Environment string `env:"SENTRY_ENVIRONMENT" envDefault:"local" validate:"required"`
	Release     string `env:"SENTRY_RELEASE" envDefault:""`
}

type LLMConfig struct {
	Provider           string `env:"LLM_PROVIDER" envDefault:"groq"`
	APIKey             string `env:"LLM_API_KEY"`
	Model              string `env:"LLM_MODEL" envDefault:"llama-3.3-70b-versatile"`
	GroqAPIKey         string `env:"LLM_GROQ_API_KEY"`
	ClaudeAPIKey       string `env:"LLM_CLAUDE_API_KEY"`
	QualifyProvider    string `env:"LLM_QUALIFY_PROVIDER"`
	QualifyModel       string `env:"LLM_QUALIFY_MODEL"`
	BacklinkProvider   string `env:"LLM_BACKLINK_PROVIDER"`
	BacklinkModel      string `env:"LLM_BACKLINK_MODEL"`
	GroqDefaultModel   string `env:"LLM_GROQ_DEFAULT_MODEL"   envDefault:"llama-3.3-70b-versatile"`
	ClaudeDefaultModel string `env:"LLM_CLAUDE_DEFAULT_MODEL" envDefault:"claude-haiku-4-5"`
}

type SheetsConfig struct {
	CredentialsFile string `env:"SHEETS_CREDENTIALS_FILE" envDefault:"./credentials.json"`
	SpreadsheetID   string `env:"SHEETS_SPREADSHEET_ID"`
	Sheet           string `env:"SHEETS_SHEET" envDefault:"Keywords"`
	TopicColumn     string `env:"SHEETS_TOPIC_COLUMN" envDefault:"A"`
	KeywordColumn   string `env:"SHEETS_KEYWORD_COLUMN" envDefault:"B"`
	TitleColumn     string `env:"SHEETS_TITLE_COLUMN" envDefault:"C"`
	HeaderRow       bool   `env:"SHEETS_HEADER_ROW" envDefault:"true"`
}

type WorkerConfig struct {
	PollInterval time.Duration `env:"WORKER_POLL_INTERVAL" envDefault:"60s"`
}

type LeadsSheetsConfig struct {
	SpreadsheetID      string `env:"LEADS_SHEETS_SPREADSHEET_ID"`
	Sheet              string `env:"LEADS_SHEETS_SHEET" envDefault:"customers"`
	ConsultationsSheet string `env:"LEADS_SHEETS_CONSULTATIONS_SHEET" envDefault:"consultations"`
}

// GA4Config is the site-traffic side of the leads dashboard ("Сайт,
// посетители" / "Сео" columns) — optional, same service account as Sheets
// (CF_SHEETS_CREDENTIALS_FILE), just needs Viewer on the GA4 property and
// this numeric property ID. Empty PropertyID disables it, same no-op
// pattern as LeadsSheets.
type GA4Config struct {
	PropertyID string `env:"GA4_PROPERTY_ID"`
}

type TelegramConfig struct {
	BotToken     string  `env:"TELEGRAM_BOT_TOKEN"`
	ChatID       int64   `env:"TELEGRAM_CHAT_ID"`
	PaymentCard  string  `env:"TELEGRAM_PAYMENT_CARD"`
	AllowedUsers []int64 `env:"TELEGRAM_ALLOWED_USERS" envSeparator:","`
}

type ReminderConfig struct {
	CheckInterval time.Duration `env:"REMINDER_CHECK_INTERVAL" envDefault:"5m"`
	Before        time.Duration `env:"REMINDER_BEFORE" envDefault:"30m"`
}

type MailConfig struct {
	IMAPHost string `env:"MAIL_IMAP_HOST"`
	IMAPPort int    `env:"MAIL_IMAP_PORT" envDefault:"993"`
	Username string `env:"MAIL_USERNAME"`
	Password string `env:"MAIL_PASSWORD"`
	Folder   string `env:"MAIL_FOLDER" envDefault:"INBOX"`
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
	Provider    string          `env:"CHECKER_PROVIDER" envDefault:"mock"`
	APIKey      string          `env:"CHECKER_API_KEY"`
	AIThreshold float64         `env:"CHECKER_AI_THRESHOLD" envDefault:"0.8"`
	Model       string          `env:"CHECKER_MODEL"`
	MaxCycles   int             `env:"CHECKER_MAX_CYCLES" envDefault:"3"`
	Copyleaks   CopyleaksConfig `envPrefix:"CHECKER_COPYLEAKS_"`
}

type CopyleaksConfig struct {
	Email   string `env:"EMAIL"`
	Sandbox bool   `env:"SANDBOX"`
}

type PexelsConfig struct {
	Enabled bool   `env:"PEXELS_ENABLED" envDefault:"true"`
	APIKey  string `env:"PEXELS_API_KEY"`
}

const devEncryptionKey = "dev-insecure-change-me"

type WordPressConfig struct {
	EncryptionKey string        `env:"WP_ENCRYPTION_KEY" envDefault:"dev-insecure-change-me" validate:"required"`
	HTTPTimeout   time.Duration `env:"WP_HTTP_TIMEOUT" envDefault:"30s"`
}

type MODXConfig struct {
	DBHost     string `env:"MODX_DB_HOST"`
	DBPort     string `env:"MODX_DB_PORT" envDefault:"3306"`
	DBUser     string `env:"MODX_DB_USER"`
	DBPassword string `env:"MODX_DB_PASSWORD"`
	DBName     string `env:"MODX_DB_NAME"`
	SiteURL    string `env:"MODX_SITE_URL"`
}

type JWTConfig struct {
	Secret string        `env:"JWT_SECRET" envDefault:"dev-insecure-change-me" validate:"required"`
	TTL    time.Duration `env:"JWT_TTL" envDefault:"24h"`
}

type Config struct {
	Server       ServerConfig     `validate:"required"`
	Logger       LoggerConfig     `validate:"required"`
	Database     DatabaseConfig   `validate:"required"`
	Sentry       SentryConfig     `validate:"required"`
	LLM          LLMConfig        `validate:"required"`
	Sheets       SheetsConfig     `validate:"required"`
	Worker       WorkerConfig     `validate:"required"`
	Article      ArticleConfig    `validate:"required"`
	DataForSEO   DataForSEOConfig `validate:"required"`
	Checker      CheckerConfig    `validate:"required"`
	Pexels       PexelsConfig     `validate:"required"`
	WordPress    WordPressConfig  `validate:"required"`
	JWT          JWTConfig        `validate:"required"`
	Prompt       PromptConfig
	LinkBuilding LinkBuildingConfig
	EmailScrape  EmailScrapeConfig
	Mail         MailConfig
	Telegram     TelegramConfig
	LeadsSheets  LeadsSheetsConfig
	GA4          GA4Config
	Reminder     ReminderConfig
	MODX         MODXConfig
}

type PromptConfig struct {
	EvolveEnabled   bool          `env:"PROMPT_EVOLVE_ENABLED" envDefault:"true"`
	HumanWeight     float64       `env:"PROMPT_HUMAN_WEIGHT" envDefault:"0.65" validate:"gt=0,lte=1"`
	PromoteInterval time.Duration `env:"PROMPT_PROMOTE_INTERVAL" envDefault:"6h"`
	EvolveInterval  time.Duration `env:"PROMPT_EVOLVE_INTERVAL" envDefault:"168h"`
}

type LinkBuildingConfig struct {
	PlaceDelayMin  time.Duration `env:"LINKBUILDING_PLACE_DELAY_MIN" envDefault:"2s"`
	PlaceDelayMax  time.Duration `env:"LINKBUILDING_PLACE_DELAY_MAX" envDefault:"5s"`
	LockedCooldown time.Duration `env:"LINKBUILDING_LOCKED_COOLDOWN" envDefault:"24h"`
	FailCooldown   time.Duration `env:"LINKBUILDING_FAIL_COOLDOWN" envDefault:"6h"`
}

type EmailScrapeConfig struct {
	Concurrency     int           `env:"EMAILSCRAPE_CONCURRENCY" envDefault:"50"`
	FlushBatch      int           `env:"EMAILSCRAPE_FLUSH_BATCH" envDefault:"50"`
	WriteTimeout    time.Duration `env:"EMAILSCRAPE_WRITE_TIMEOUT" envDefault:"30s"`
	MaxContactPages int           `env:"EMAILSCRAPE_MAX_CONTACT_PAGES" envDefault:"8"`
	PerHostRPS      float64       `env:"EMAILSCRAPE_PER_HOST_RPS" envDefault:"1"`
	PerHostBurst    int           `env:"EMAILSCRAPE_PER_HOST_BURST" envDefault:"1"`
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
