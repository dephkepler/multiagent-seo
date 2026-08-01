package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/emailscrape"
)

type ScrapedEmailRepository struct {
	db *pgxpool.Pool
}

func NewScrapedEmailRepository(db *pgxpool.Pool) *ScrapedEmailRepository {
	return &ScrapedEmailRepository{db: db}
}

func (r *ScrapedEmailRepository) Save(ctx context.Context, records []emailscrape.EmailRecord) error {
	if len(records) == 0 {
		return nil
	}

	const q = `INSERT INTO scraped_emails (list_name, site_url, email, source_page)
		VALUES (@list, @site, @email, @src)
		ON CONFLICT ON CONSTRAINT uq_scraped_emails DO NOTHING`

	batch := &pgx.Batch{}
	for _, rec := range records {
		batch.Queue(q, pgx.NamedArgs{
			"list":  rec.ListName,
			"site":  rec.SiteURL,
			"email": rec.Email,
			"src":   rec.SourcePage,
		})
	}

	br := r.db.SendBatch(ctx, batch)
	defer br.Close()
	for range records {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("save scraped emails: %w", err)
		}
	}
	return nil
}
