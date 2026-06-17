package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/emailscrape"
)

type ScrapeJobRepository struct {
	db *pgxpool.Pool
}

func NewScrapeJobRepository(db *pgxpool.Pool) *ScrapeJobRepository {
	return &ScrapeJobRepository{db: db}
}

func (r *ScrapeJobRepository) Create(ctx context.Context, sheet string, total int) (string, error) {
	const q = `INSERT INTO scrape_jobs (sheet, state, total) VALUES (@sheet, @state, @total) RETURNING id`
	var id string
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"sheet": sheet,
		"state": emailscrape.JobRunning,
		"total": total,
	}).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create scrape job: %w", err)
	}
	return id, nil
}

func (r *ScrapeJobRepository) UpdateProgress(ctx context.Context, id string, processed int) error {
	const q = `UPDATE scrape_jobs SET processed = @processed, updated_at = now() WHERE id = @id`
	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id, "processed": processed}); err != nil {
		return fmt.Errorf("update scrape job progress: %w", err)
	}
	return nil
}

func (r *ScrapeJobRepository) Finish(ctx context.Context, id, state, errMsg string) error {
	const q = `UPDATE scrape_jobs SET state = @state, error = @error, updated_at = now() WHERE id = @id`
	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id, "state": state, "error": nullable(errMsg)}); err != nil {
		return fmt.Errorf("finish scrape job: %w", err)
	}
	return nil
}

func (r *ScrapeJobRepository) Get(ctx context.Context, id string) (emailscrape.Job, error) {
	const q = `SELECT id, sheet, state, total, processed FROM scrape_jobs WHERE id = @id`
	var j emailscrape.Job
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id}).Scan(&j.ID, &j.Sheet, &j.State, &j.Total, &j.Processed)
	if errors.Is(err, pgx.ErrNoRows) {
		return emailscrape.Job{}, emailscrape.ErrJobNotFound
	}
	if err != nil {
		return emailscrape.Job{}, fmt.Errorf("get scrape job: %w", err)
	}
	return j, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
