package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	LLM        LLMConfig        `mapstructure:"llm"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Sheets     SheetsConfig     `mapstructure:"sheets"`
	WordPress  WordPressConfig  `mapstructure:"wordpress"`
	Telegram   TelegramConfig   `mapstructure:"telegram"`
	Worker     WorkerConfig     `mapstructure:"worker"`
	Article    ArticleConfig    `mapstructure:"article"`
	Server     ServerConfig     `mapstructure:"server"`
	DataForSEO DataForSEOConfig `mapstructure:"dataforseo"`
	Checker    CheckerConfig    `mapstructure:"checker"`
}

type LLMConfig struct {
	Provider     string `mapstructure:"provider"`
	APIKey       string `mapstructure:"apiKey"` // primary key for the default provider
	Model        string `mapstructure:"model"`
	GroqAPIKey   string `mapstructure:"groqApiKey"`   // optional — used when per-request provider=groq
	ClaudeAPIKey string `mapstructure:"claudeApiKey"` // optional — used when per-request provider=claude
}

// KeyFor returns the API key for the given provider, falling back to APIKey
// when the requested provider matches the default and no explicit per-provider
// key is configured.
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
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type SheetsConfig struct {
	CredentialsFile string `mapstructure:"credentialsFile"`
	SpreadsheetID   string `mapstructure:"spreadsheetId"`
	Sheet           string `mapstructure:"sheet"`         // worksheet name, e.g. "Keywords"
	TopicColumn     string `mapstructure:"topicColumn"`   // column letter holding the topic, e.g. "A"
	KeywordColumn   string `mapstructure:"keywordColumn"` // column letter holding the keyword, e.g. "B"
	TitleColumn     string `mapstructure:"titleColumn"`   // optional column with a suggested article title, e.g. "C"; empty = disabled
	HeaderRow       bool   `mapstructure:"headerRow"`     // true if row 1 is a header and should be skipped
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
	Login    string `mapstructure:"login"`
	Password string `mapstructure:"password"`
	// SERPLimit is how many competitor results to fetch (default 5).
	SERPLimit int `mapstructure:"serpLimit"`
}

type CheckerConfig struct {
	// Provider selects the checker implementation: "mock", "originality", or "huggingface".
	Provider    string  `mapstructure:"provider"`
	APIKey      string  `mapstructure:"apiKey"`
	AIThreshold float64 `mapstructure:"aiThreshold"`
	// Model is the detector model identifier — only used by providers that support model selection (e.g. huggingface).
	Model string `mapstructure:"model"`
	// MaxCycles is the maximum number of humanize rewrites before giving up and publishing anyway.
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
	v.SetDefault("dataforseo.serpLimit", 5)
	v.SetDefault("checker.provider", "mock")
	v.SetDefault("checker.aiThreshold", 0.8)
	v.SetDefault("checker.maxCycles", 3)

	v.SetEnvPrefix("CF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindings := map[string]string{
		"llm.apiKey":             "CF_LLM_API_KEY",
		"llm.groqApiKey":         "CF_LLM_GROQ_API_KEY",
		"llm.claudeApiKey":       "CF_LLM_CLAUDE_API_KEY",
		"database.url":           "CF_DATABASE_URL",
		"wordpress.url":          "CF_WORDPRESS_URL",
		"wordpress.user":         "CF_WORDPRESS_USER",
		"wordpress.appPassword":  "CF_WORDPRESS_APP_PASSWORD",
		"telegram.botToken":      "CF_TELEGRAM_BOT_TOKEN",
		"sheets.credentialsFile":  "CF_SHEETS_CREDENTIALS_FILE",
		"sheets.spreadsheetId":    "CF_SHEETS_SPREADSHEET_ID",
		"sheets.sheet":            "CF_SHEETS_SHEET",
		"sheets.topicColumn":      "CF_SHEETS_TOPIC_COLUMN",
		"sheets.keywordColumn":    "CF_SHEETS_KEYWORD_COLUMN",
		"sheets.titleColumn":      "CF_SHEETS_TITLE_COLUMN",
		"sheets.headerRow":        "CF_SHEETS_HEADER_ROW",
		"dataforseo.login":        "CF_DATAFORSEO_LOGIN",
		"dataforseo.password":     "CF_DATAFORSEO_PASSWORD",
		"dataforseo.serpLimit":    "CF_DATAFORSEO_SERP_LIMIT",
		"checker.provider":        "CF_CHECKER_PROVIDER",
		"checker.apiKey":          "CF_CHECKER_API_KEY",
		"checker.aiThreshold":     "CF_CHECKER_AI_THRESHOLD",
		"checker.model":           "CF_CHECKER_MODEL",
		"checker.maxCycles":       "CF_CHECKER_MAX_CYCLES",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}

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

// Validate checks that all required config fields are set.
// Returns an error listing every missing field, not just the first.
func (c *Config) Validate() error {
	var missing []string

	if c.LLM.APIKey == "" {
		missing = append(missing, "llm.apiKey")
	}
	if c.Database.URL == "" {
		missing = append(missing, "database.url")
	}
	if c.WordPress.URL == "" {
		missing = append(missing, "wordpress.url")
	}
	if c.WordPress.User == "" {
		missing = append(missing, "wordpress.user")
	}
	if c.WordPress.AppPassword == "" {
		missing = append(missing, "wordpress.appPassword")
	}

	if len(missing) > 0 {
		return fmt.Errorf("invalid config: missing %s", strings.Join(missing, ", "))
	}
	return nil
}
