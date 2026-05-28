package config

import (
	"testing"
)

func setMinEnv(t *testing.T) {
	t.Helper()
	// Required fields without envDefault must be set so validation passes;
	// everything else relies on defaults declared in struct tags.
}

func TestLoad_DefaultsAreValid(t *testing.T) {
	setMinEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed with defaults: %v", err)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("Server.Port = %q, want %q", cfg.Server.Port, "8080")
	}
	if cfg.Database.Dbname != "contentflow" {
		t.Errorf("Database.Dbname = %q, want %q", cfg.Database.Dbname, "contentflow")
	}
	if cfg.LLM.Provider != "groq" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "groq")
	}
	if cfg.Article.Language != "ru" {
		t.Errorf("Article.Language = %q, want %q", cfg.Article.Language, "ru")
	}
	if !cfg.Pexels.Enabled {
		t.Errorf("Pexels.Enabled should default to true")
	}
	if cfg.Sentry.Enabled {
		t.Errorf("Sentry.Enabled should default to false")
	}
}

func TestLoad_CFPrefixApplied(t *testing.T) {
	t.Setenv("CF_APP_PORT", "9999")
	t.Setenv("CF_ARTICLE_MIN_WORDS", "500")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Port != "9999" {
		t.Errorf("CF_APP_PORT not applied: got %q", cfg.Server.Port)
	}
	if cfg.Article.MinWords != 500 {
		t.Errorf("CF_ARTICLE_MIN_WORDS not applied: got %d", cfg.Article.MinWords)
	}
}

func TestLoad_ListSplit(t *testing.T) {
	t.Setenv("CF_APP_CORS_ALLOWED_ORIGINS", "http://a,http://b,http://c")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Server.CORSAllowedOrigins) != 3 {
		t.Errorf("CORS origins = %v, want 3 entries", cfg.Server.CORSAllowedOrigins)
	}
}

func TestDatabaseDSN(t *testing.T) {
	c := DatabaseConfig{
		Host: "db.example", Port: "5432",
		User: "u", Password: "p@ss/word",
		Dbname: "mydb", SSLMode: "require",
	}
	// @ and / must be percent-encoded in userinfo or postgres URL parsing breaks
	// on the host/path boundaries.
	got := c.DSN()
	want := "postgres://u:p%40ss%2Fword@db.example:5432/mydb?sslmode=require"
	if got != want {
		t.Errorf("DSN()\n got: %s\nwant: %s", got, want)
	}
}

func TestLLMKeyFor(t *testing.T) {
	c := LLMConfig{
		Provider:     "groq",
		APIKey:       "default-key",
		GroqAPIKey:   "groq-key",
		ClaudeAPIKey: "claude-key",
	}
	if got := c.KeyFor("groq"); got != "groq-key" {
		t.Errorf("KeyFor(groq) = %q, want groq-key", got)
	}
	if got := c.KeyFor("claude"); got != "claude-key" {
		t.Errorf("KeyFor(claude) = %q, want claude-key", got)
	}
	if got := c.KeyFor("anthropic"); got != "claude-key" {
		t.Errorf("KeyFor(anthropic) = %q, want claude-key", got)
	}
	if got := c.KeyFor("unknown"); got != "" {
		t.Errorf("KeyFor(unknown) = %q, want empty", got)
	}

	// Fallback to APIKey only when no per-provider key set AND provider matches default.
	c2 := LLMConfig{Provider: "groq", APIKey: "default-key"}
	if got := c2.KeyFor("groq"); got != "default-key" {
		t.Errorf("KeyFor fallback = %q, want default-key", got)
	}
}
