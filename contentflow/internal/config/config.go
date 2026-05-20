package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// DefaultSite is the fallback site alias and the one required at startup.
const DefaultSite = "default"

type Config struct {
	LLM        LLMConfig                  `mapstructure:"llm"`
	Database   DatabaseConfig             `mapstructure:"database"`
	Sheets     SheetsConfig               `mapstructure:"sheets"`
	WordPress  map[string]WordPressConfig `mapstructure:"wordpress"`
	Telegram   TelegramConfig             `mapstructure:"telegram"`
	Worker     WorkerConfig               `mapstructure:"worker"`
	Article    ArticleConfig              `mapstructure:"article"`
	Server     ServerConfig               `mapstructure:"server"`
	DataForSEO DataForSEOConfig           `mapstructure:"dataforseo"`
	Checker    CheckerConfig              `mapstructure:"checker"`
	Pexels     PexelsConfig               `mapstructure:"pexels"`
}

// SiteAliases returns the configured WordPress site aliases sorted alphabetically
// so logs and error messages stay deterministic.
func (c *Config) SiteAliases() []string {
	out := make([]string, 0, len(c.WordPress))
	for k := range c.WordPress {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type LLMConfig struct {
	Provider     string `mapstructure:"provider"`
	APIKey       string `mapstructure:"apiKey"`
	Model        string `mapstructure:"model"`
	GroqAPIKey   string `mapstructure:"groqApiKey"`
	ClaudeAPIKey string `mapstructure:"claudeApiKey"`
}

// KeyFor falls back to APIKey only when the requested provider matches the
// default one and no per-provider key is set.
func (c LLMConfig) KeyFor(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "groq":
		if c.GroqAPIKey != "" {
			return c.GroqAPIKey
		}
	case "claude", "anthropic":
		if c.ClaudeAPIKey != "" {
			return c.ClaudeAPIKey
		}
	}
	if p == strings.ToLower(c.Provider) {
		return c.APIKey
	}
	return ""
}

type ServerConfig struct {
	Addr         string        `mapstructure:"addr"`
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"writeTimeout"`
	// ShutdownWaitTimeout bounds shutdown drain; past it the DB pool closes
	// even with goroutines still running, which may surface pool-closed errors.
	ShutdownWaitTimeout  time.Duration `mapstructure:"shutdownWaitTimeout"`
	BackgroundJobTimeout time.Duration `mapstructure:"backgroundJobTimeout"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type SheetsConfig struct {
	CredentialsFile string `mapstructure:"credentialsFile"`
	SpreadsheetID   string `mapstructure:"spreadsheetId"`
	Sheet           string `mapstructure:"sheet"`
	TopicColumn     string `mapstructure:"topicColumn"`
	KeywordColumn   string `mapstructure:"keywordColumn"`
	TitleColumn     string `mapstructure:"titleColumn"` // empty disables suggested-title lookup
	HeaderRow       bool   `mapstructure:"headerRow"`
}

type WordPressConfig struct {
	URL         string `mapstructure:"url"`
	User        string `mapstructure:"user"`
	AppPassword string `mapstructure:"appPassword"`
}

type TelegramConfig struct {
	BotToken     string   `mapstructure:"botToken"`
	AllowedUsers []string `mapstructure:"allowedUsers"`
}

type WorkerConfig struct {
	PollInterval time.Duration `mapstructure:"pollInterval"`
}

type ArticleConfig struct {
	Language   string `mapstructure:"language"`
	MinWords   int    `mapstructure:"minWords"`
	MaxWords   int    `mapstructure:"maxWords"`
	SiteTopic  string `mapstructure:"siteTopic"`
	ExtraRules string `mapstructure:"extraRules"`
}

type DataForSEOConfig struct {
	Login     string `mapstructure:"login"`
	Password  string `mapstructure:"password"`
	SERPLimit int    `mapstructure:"serpLimit"`
}

type PexelsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"apiKey"`
}

type CheckerConfig struct {
	// Provider selects the checker implementation: "mock", "originality", or "huggingface".
	Provider    string  `mapstructure:"provider"`
	APIKey      string  `mapstructure:"apiKey"`
	AIThreshold float64 `mapstructure:"aiThreshold"`
	Model       string  `mapstructure:"model"`
	// MaxCycles caps humanize rewrites; on overflow the article is published as-is.
	MaxCycles int `mapstructure:"maxCycles"`
}

func NewConfig() (*Config, error) {
	v := viper.New()

	v.SetDefault("llm.provider", "groq")
	v.SetDefault("llm.model", "claude-haiku-4-5-20251001")
	v.SetDefault("sheets.sheet", "Keywords")
	v.SetDefault("sheets.topicColumn", "A")
	v.SetDefault("sheets.keywordColumn", "B")
	v.SetDefault("sheets.titleColumn", "C")
	v.SetDefault("sheets.headerRow", true)
	v.SetDefault("worker.pollInterval", 60*time.Second)
	v.SetDefault("article.language", "ru")
	v.SetDefault("article.minWords", 1500)
	v.SetDefault("article.maxWords", 3000)
	v.SetDefault("server.addr", ":8080")
	v.SetDefault("server.readTimeout", 10*time.Second)
	v.SetDefault("server.writeTimeout", 5*time.Minute)
	v.SetDefault("server.shutdownWaitTimeout", 30*time.Second)
	v.SetDefault("server.backgroundJobTimeout", 15*time.Minute)
	v.SetDefault("dataforseo.serpLimit", 5)
	v.SetDefault("checker.provider", "mock")
	v.SetDefault("checker.aiThreshold", 0.8)
	v.SetDefault("checker.maxCycles", 3)
	v.SetDefault("pexels.enabled", true)

	v.SetEnvPrefix("CF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Env vars only target the "default" WordPress entry; multi-site config lives in YAML.
	bindings := map[string]string{
		"llm.apiKey":                  "CF_LLM_API_KEY",
		"llm.groqApiKey":              "CF_LLM_GROQ_API_KEY",
		"llm.claudeApiKey":            "CF_LLM_CLAUDE_API_KEY",
		"database.url":                "CF_DATABASE_URL",
		"wordpress.default.url":         "CF_WORDPRESS_URL",
		"wordpress.default.user":        "CF_WORDPRESS_USER",
		"wordpress.default.appPassword": "CF_WORDPRESS_APP_PASSWORD",
		"telegram.botToken":           "CF_TELEGRAM_BOT_TOKEN",
		"sheets.credentialsFile":      "CF_SHEETS_CREDENTIALS_FILE",
		"sheets.spreadsheetId":        "CF_SHEETS_SPREADSHEET_ID",
		"sheets.sheet":                "CF_SHEETS_SHEET",
		"sheets.topicColumn":          "CF_SHEETS_TOPIC_COLUMN",
		"sheets.keywordColumn":        "CF_SHEETS_KEYWORD_COLUMN",
		"sheets.titleColumn":          "CF_SHEETS_TITLE_COLUMN",
		"sheets.headerRow":            "CF_SHEETS_HEADER_ROW",
		"dataforseo.login":            "CF_DATAFORSEO_LOGIN",
		"dataforseo.password":         "CF_DATAFORSEO_PASSWORD",
		"dataforseo.serpLimit":        "CF_DATAFORSEO_SERP_LIMIT",
		"checker.provider":            "CF_CHECKER_PROVIDER",
		"checker.apiKey":              "CF_CHECKER_API_KEY",
		"checker.aiThreshold":         "CF_CHECKER_AI_THRESHOLD",
		"checker.model":               "CF_CHECKER_MODEL",
		"checker.maxCycles":           "CF_CHECKER_MAX_CYCLES",
		"pexels.enabled":              "CF_PEXELS_ENABLED",
		"pexels.apiKey":               "CF_PEXELS_API_KEY",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}

	// Accept the unprefixed PEXELS_API_KEY too; CF_PEXELS_API_KEY still wins
	// when both are set (viper resolves in registration order).
	_ = v.BindEnv("pexels.apiKey", "PEXELS_API_KEY")

	v.SetConfigType("yaml")
	v.AddConfigPath("./internal/config")
	v.AddConfigPath(".")

	v.SetConfigName("config")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	v.SetConfigName("settings")
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate reports every missing required field at once, not just the first.
func (c *Config) Validate() error {
	var missing []string

	if c.LLM.APIKey == "" {
		missing = append(missing, "llm.apiKey")
	}
	if c.Database.URL == "" {
		missing = append(missing, "database.url")
	}
	if len(c.WordPress) == 0 {
		missing = append(missing, "wordpress (at least one site required)")
	} else {
		if _, ok := c.WordPress[DefaultSite]; !ok {
			missing = append(missing, fmt.Sprintf("wordpress.%s (required entry)", DefaultSite))
		}
		for _, alias := range c.SiteAliases() {
			site := c.WordPress[alias]
			if site.URL == "" {
				missing = append(missing, fmt.Sprintf("wordpress.%s.url", alias))
			}
			if site.User == "" {
				missing = append(missing, fmt.Sprintf("wordpress.%s.user", alias))
			}
			if site.AppPassword == "" {
				missing = append(missing, fmt.Sprintf("wordpress.%s.appPassword", alias))
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("invalid config: missing %s", strings.Join(missing, ", "))
	}
	return nil
}
