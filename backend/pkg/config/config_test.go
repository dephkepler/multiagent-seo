package config

import (
	"testing"
)

func setMinEnv(t *testing.T) {
	t.Helper()
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

	c2 := LLMConfig{Provider: "groq", APIKey: "default-key"}
	if got := c2.KeyFor("groq"); got != "default-key" {
		t.Errorf("KeyFor fallback = %q, want default-key", got)
	}

	c3 := LLMConfig{Provider: "claude", APIKey: "default-key"}
	if got := c3.KeyFor("anthropic"); got != "default-key" {
		t.Errorf("KeyFor(anthropic) alias fallback = %q, want default-key", got)
	}
}

func TestLLMDefaultsFor(t *testing.T) {
	base := LLMConfig{Provider: "groq", Model: "llama-3.3-70b-versatile"}

	t.Run("unset task falls back to global", func(t *testing.T) {
		p, m := base.DefaultsFor(TaskBacklink)
		if p != "groq" || m != "llama-3.3-70b-versatile" {
			t.Errorf("DefaultsFor(backlink) = (%q,%q), want global", p, m)
		}
	})

	t.Run("per-task overrides take precedence", func(t *testing.T) {
		c := base
		c.BacklinkProvider = "claude"
		c.BacklinkModel = "claude-haiku-4-5"
		p, m := c.DefaultsFor(TaskBacklink)
		if p != "claude" || m != "claude-haiku-4-5" {
			t.Errorf("DefaultsFor(backlink) = (%q,%q), want override", p, m)
		}
		if p, m := c.DefaultsFor(TaskQualify); p != "groq" || m != "llama-3.3-70b-versatile" {
			t.Errorf("DefaultsFor(qualify) = (%q,%q), want global (override is for backlink only)", p, m)
		}
	})

	t.Run("only model overridden, provider stays global", func(t *testing.T) {
		c := base
		c.QualifyModel = "llama-3.1-8b-instant"
		p, m := c.DefaultsFor(TaskQualify)
		if p != "groq" || m != "llama-3.1-8b-instant" {
			t.Errorf("DefaultsFor(qualify) = (%q,%q), want (groq, llama-3.1-8b-instant)", p, m)
		}
	})

	t.Run("only provider overridden picks provider's own default model", func(t *testing.T) {
		c := base
		c.GroqDefaultModel = "llama-3.3-70b-versatile"
		c.ClaudeDefaultModel = "claude-haiku-4-5"
		c.BacklinkProvider = "claude"
		p, m := c.DefaultsFor(TaskBacklink)
		if p != "claude" || m != "claude-haiku-4-5" {
			t.Errorf("DefaultsFor(backlink) = (%q,%q), want (claude, claude-haiku-4-5)", p, m)
		}
	})
}

func TestLLMModelFor(t *testing.T) {
	c := LLMConfig{
		Provider:           "groq",
		Model:              "llama-3.3-70b-versatile",
		GroqDefaultModel:   "llama-3.3-70b-versatile",
		ClaudeDefaultModel: "claude-haiku-4-5",
	}
	if got := c.ModelFor("groq"); got != "llama-3.3-70b-versatile" {
		t.Errorf("ModelFor(groq) = %q", got)
	}
	if got := c.ModelFor("claude"); got != "claude-haiku-4-5" {
		t.Errorf("ModelFor(claude) = %q", got)
	}
	if got := c.ModelFor("anthropic"); got != "claude-haiku-4-5" {
		t.Errorf("ModelFor(anthropic) = %q", got)
	}
}

func TestLoad_RejectsDevSecretsOutsideLocal(t *testing.T) {
	t.Setenv("CF_SENTRY_ENVIRONMENT", "production")
	if _, err := Load(); err == nil {
		t.Fatal("expected error: dev secrets in non-local env")
	}

	t.Setenv("CF_WP_ENCRYPTION_KEY", "a-real-prod-secret")
	t.Setenv("CF_JWT_SECRET", "a-real-jwt-secret")
	if _, err := Load(); err != nil {
		t.Fatalf("real secrets in non-local env should pass: %v", err)
	}
}
