package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	LLM       LLMConfig       `mapstructure:"llm"       yaml:"llm"`
	Database  DatabaseConfig  `mapstructure:"database"  yaml:"database"`
	Sheets    SheetsConfig    `mapstructure:"sheets"    yaml:"sheets"`
	WordPress WordPressConfig `mapstructure:"wordpress" yaml:"wordpress"`
	Telegram  TelegramConfig  `mapstructure:"telegram"  yaml:"telegram"`
	Worker    WorkerConfig    `mapstructure:"worker"    yaml:"worker"`
	Article   ArticleConfig   `mapstructure:"article"   yaml:"article"`
}

type LLMConfig struct {
	APIKey string `mapstructure:"apiKey" yaml:"apiKey"`
	Model  string `mapstructure:"model"  yaml:"model"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url" yaml:"url"`
}

type SheetsConfig struct {
	CredentialsFile string `mapstructure:"credentialsFile" yaml:"credentialsFile"`
	SpreadsheetID   string `mapstructure:"spreadsheetId"   yaml:"spreadsheetId"`
	Sheet           string `mapstructure:"sheet"           yaml:"sheet"`
	KeywordColumn   string `mapstructure:"keywordColumn"   yaml:"keywordColumn"`
}

type WordPressConfig struct {
	URL         string `mapstructure:"url"         yaml:"url"`
	User        string `mapstructure:"user"        yaml:"user"`
	AppPassword string `mapstructure:"appPassword" yaml:"appPassword"`
}

type TelegramConfig struct {
	BotToken     string   `mapstructure:"botToken"     yaml:"botToken"`
	AllowedUsers []string `mapstructure:"allowedUsers" yaml:"allowedUsers"`
}

type WorkerConfig struct {
	PollInterval time.Duration `mapstructure:"pollInterval" yaml:"pollInterval"`
}

type ArticleConfig struct {
	Language   string `mapstructure:"language"   yaml:"language"`
	MinWords   int    `mapstructure:"minWords"   yaml:"minWords"`
	MaxWords   int    `mapstructure:"maxWords"   yaml:"maxWords"`
	SiteTopic  string `mapstructure:"siteTopic"  yaml:"siteTopic"`
	ExtraRules string `mapstructure:"extraRules" yaml:"extraRules"`
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		LLM: LLMConfig{
			Model: "claude-haiku-4-5-20251001",
		},
		Sheets: SheetsConfig{
			Sheet:         "Sheet1",
			KeywordColumn: "A",
		},
		Worker: WorkerConfig{
			PollInterval: 60 * time.Second,
		},
		Article: ArticleConfig{
			Language: "ru",
			MinWords: 1500,
			MaxWords: 3000,
		},
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetEnvPrefix("CF")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
