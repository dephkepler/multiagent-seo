package config

import (
	"testing"
)

func TestLoad_DefaultsAreValid(t *testing.T) {
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
