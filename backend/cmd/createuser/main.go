package main

import (
	"context"
	"errors"
	"flag"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func main() {
	emailFlag := flag.String("email", "", "user email")
	passwordFlag := flag.String("password", "", "user password")
	roleFlag := flag.String("role", "admin", "admin or advocate")
	advocateFlag := flag.String("advocate", "", "for -role advocate: the roster full name this login speaks for, exactly as stored")
	flag.Parse()

	email, password := *emailFlag, *passwordFlag
	if args := flag.Args(); email == "" && password == "" && len(args) == 2 {
		email, password = args[0], args[1]
	}
	if email == "" || password == "" {
		log.Fatal().Msg("email and password are required (-email -password or two positional args)")
	}
	role := user.Role(*roleFlag)
	if !user.IsRole(role) {
		log.Fatal().Str("role", *roleFlag).Msg("role must be admin or advocate")
	}
	// An advocate login with no roster row would see nothing and, worse, would
	// be one forgotten guard away from seeing everything — so it is refused
	// here rather than created and fixed later.
	if role == user.RoleAdvocate && *advocateFlag == "" {
		log.Fatal().Msg("-advocate is required for -role advocate")
	}
	if role == user.RoleAdmin && *advocateFlag != "" {
		log.Fatal().Msg("-advocate makes no sense for an admin login")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load")
	}
	if err := logger.Init(cfg.Logger.Level); err != nil {
		log.Fatal().Err(err).Msg("logger init")
	}

	ctx := context.Background()
	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		log.Fatal().Err(err).Msg("database connect")
	}
	defer database.Close()
	pool := database.Pool()

	advocateID := resolveAdvocate(ctx, pool, *advocateFlag)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal().Err(err).Msg("hash password")
	}

	const q = `INSERT INTO users (email, password_hash, role, advocate_id)
		VALUES (@email, @hash, @role, @advocate_id) RETURNING id`
	var id string
	err = pool.QueryRow(ctx, q, pgx.NamedArgs{
		"email":       email,
		"hash":        string(hash),
		"role":        string(role),
		"advocate_id": advocateID,
	}).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			log.Fatal().Str("email", email).Msg("user with this email already exists")
		}
		log.Fatal().Err(err).Msg("create user")
	}

	log.Info().Str("id", id).Str("email", email).Str("role", string(role)).Msg("user created")
}

// resolveAdvocate turns a full name into the roster id. The match is exact and
// must be unique: picking the wrong "Борзов" would hand one advocate another's
// clients and money, which is not a mistake worth a fuzzy match.
func resolveAdvocate(ctx context.Context, pool *pgxpool.Pool, fullName string) *string {
	if fullName == "" {
		return nil
	}

	const q = `SELECT id::text FROM advocates WHERE full_name = @name`
	rows, err := pool.Query(ctx, q, pgx.NamedArgs{"name": fullName})
	if err != nil {
		log.Fatal().Err(err).Msg("look up advocate")
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		log.Fatal().Err(err).Msg("look up advocate")
	}
	switch len(ids) {
	case 1:
		return &ids[0]
	case 0:
		log.Fatal().Str("advocate", fullName).Msg("no advocate with this exact full name")
	default:
		log.Fatal().Str("advocate", fullName).Int("matches", len(ids)).
			Msg("more than one advocate with this full name — fix the roster first")
	}
	return nil
}
