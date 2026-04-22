package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/viper"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	url := os.Getenv("CF_DATABASE_URL")
	if url == "" {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./internal/config")
		viper.AddConfigPath(".")
		if err := viper.ReadInConfig(); err != nil {
			log.Error("read config", "err", err)
			os.Exit(1)
		}
		url = viper.GetString("database.url")
	}

	m, err := migrate.New("file://migrations", url)
	if err != nil {
		log.Error("migrate new", "err", err)
		os.Exit(1)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("no migrations to apply")
			return
		}
		log.Error("migrate up", "err", err)
		os.Exit(1)
	}

	log.Info("migrations applied")
}
