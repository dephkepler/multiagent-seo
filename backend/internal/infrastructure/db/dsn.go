package db

import (
	"net"
	"net/url"

	"multiagent-seo/pkg/config"
)

func FormatConnectionURL(cfg config.DatabaseConfig, forMigrations bool) string {
	scheme := "postgres"
	if forMigrations {
		scheme = "pgx5"
	}
	u := url.URL{
		Scheme: scheme,
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, cfg.Port),
		Path:   "/" + cfg.Dbname,
	}
	q := u.Query()
	q.Set("sslmode", cfg.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}
