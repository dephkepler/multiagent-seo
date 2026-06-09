package db

import (
	"context"
	"testing"
	"time"

	"multiagent-seo/pkg/config"
)

func TestNewPoolPingFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := config.DatabaseConfig{
		Host:     "127.0.0.1",
		Port:     "1",
		User:     "postgres",
		Password: "postgres",
		Dbname:   "contentflow",
		SSLMode:  "disable",
	}

	pool, err := NewPool(ctx, cfg)
	if err == nil {
		pool.Close()
		t.Fatal("expected error pinging unreachable database, got nil")
	}
}
