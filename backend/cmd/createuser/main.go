package main

import (
	"context"
	"errors"
	"flag"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func main() {
	emailFlag := flag.String("email", "", "user email")
	passwordFlag := flag.String("password", "", "user password")
	flag.Parse()

	email, password := *emailFlag, *passwordFlag
	if args := flag.Args(); email == "" && password == "" && len(args) == 2 {
		email, password = args[0], args[1]
	}
	if email == "" || password == "" {
		log.Fatal().Msg("email and password are required (-email -password or two positional args)")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load")
	}
	if err := logger.Init(cfg.Logger.Level); err != nil {
		log.Fatal().Err(err).Msg("logger init")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("database connect")
	}
	defer pool.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal().Err(err).Msg("hash password")
	}

	const q = `INSERT INTO users (email, password_hash) VALUES (@email, @hash) RETURNING id`
	var id string
	err = pool.QueryRow(ctx, q, pgx.NamedArgs{"email": email, "hash": string(hash)}).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			log.Fatal().Str("email", email).Msg("user with this email already exists")
		}
		log.Fatal().Err(err).Msg("create user")
	}

	log.Info().Str("id", id).Str("email", email).Msg("user created")
}
