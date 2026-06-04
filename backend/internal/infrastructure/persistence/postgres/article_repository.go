package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/articles"
)

func mapPGError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23503":
		return fmt.Errorf("invalid site reference: %w", err)
	default:
		return err
	}
}

type ArticleRepository struct {
	db *pgxpool.Pool
}

func NewArticleRepository(pool *pgxpool.Pool) *ArticleRepository {
	return &ArticleRepository{db: pool}
}

func (r *ArticleRepository) Create(ctx context.Context, in articles.CreateArticle) (int64, error) {
	const q = `
		INSERT INTO articles (keyword, site_id, site, status)
		VALUES (@keyword, @site_id, @site, @status)
		RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"keyword": in.Keyword,
		"site_id": in.SiteID,
		"site":    in.Site,
		"status":  articles.StatusGenerating,
	}).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create article: %w", mapPGError(err))
	}
	return id, nil
}

func (r *ArticleRepository) Get(ctx context.Context, id int64) (*articles.Article, error) {
	const q = `
		SELECT id, keyword, site_id, site, status,
		       COALESCE(wp_post_id, 0),
		       COALESCE(wp_edit_url, ''),
		       COALESCE(wp_post_url, ''),
		       images_requested, images_resolved, images_skipped,
		       competitor_data, check_result,
		       created_at, updated_at
		FROM articles WHERE id = @id`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id})
	a, err := scanArticleFull(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, articles.ErrNotFound
		}
		return nil, fmt.Errorf("get article: %w", err)
	}
	return &a, nil
}

func (r *ArticleRepository) List(ctx context.Context) ([]articles.Article, error) {
	const q = `
		SELECT id, keyword, site_id, site, status,
		       COALESCE(wp_post_id, 0),
		       COALESCE(wp_edit_url, ''),
		       COALESCE(wp_post_url, ''),
		       images_requested, images_resolved, images_skipped,
		       created_at, updated_at
		FROM articles ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()

	list := make([]articles.Article, 0)
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}
		list = append(list, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}
	return list, nil
}

func (r *ArticleRepository) UpdateDraft(ctx context.Context, id, wpPostID int64, editURL string) error {
	const q = `
		UPDATE articles
		SET status = @status, wp_post_id = @wp_post_id, wp_edit_url = @wp_edit_url, updated_at = NOW()
		WHERE id = @id`

	return r.exec(ctx, "update draft", q, pgx.NamedArgs{
		"status":      articles.StatusDraft,
		"wp_post_id":  wpPostID,
		"wp_edit_url": editURL,
		"id":          id,
	})
}

func (r *ArticleRepository) MarkFailed(ctx context.Context, id int64) error {
	const q = `UPDATE articles SET status = @status, updated_at = NOW() WHERE id = @id`
	return r.exec(ctx, "mark failed", q, pgx.NamedArgs{
		"status": articles.StatusFailed,
		"id":     id,
	})
}

func (r *ArticleRepository) MarkPublished(ctx context.Context, id int64, postURL string) error {
	const q = `
		UPDATE articles
		SET status = @status, wp_post_url = @wp_post_url, updated_at = NOW()
		WHERE id = @id`

	return r.exec(ctx, "mark published", q, pgx.NamedArgs{
		"status":      articles.StatusPublished,
		"wp_post_url": postURL,
		"id":          id,
	})
}

func (r *ArticleRepository) SaveImageStats(ctx context.Context, id int64, requested, resolved, skipped int) error {
	const q = `
		UPDATE articles
		SET images_requested = @requested, images_resolved = @resolved, images_skipped = @skipped, updated_at = NOW()
		WHERE id = @id`

	return r.exec(ctx, "save image stats", q, pgx.NamedArgs{
		"requested": requested,
		"resolved":  resolved,
		"skipped":   skipped,
		"id":        id,
	})
}

func (r *ArticleRepository) SaveCompetitorData(ctx context.Context, id int64, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal competitor data: %w", err)
	}
	const q = `UPDATE articles SET competitor_data = @data, updated_at = NOW() WHERE id = @id`
	return r.exec(ctx, "save competitor data", q, pgx.NamedArgs{"data": b, "id": id})
}

func (r *ArticleRepository) SaveCheckResult(ctx context.Context, id int64, result any) error {
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal check result: %w", err)
	}
	const q = `UPDATE articles SET check_result = @result, updated_at = NOW() WHERE id = @id`
	return r.exec(ctx, "save check result", q, pgx.NamedArgs{"result": b, "id": id})
}

func (r *ArticleRepository) exec(ctx context.Context, op, q string, args pgx.NamedArgs) error {
	tag, err := r.db.Exec(ctx, q, args)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if tag.RowsAffected() == 0 {
		return articles.ErrNotFound
	}
	return nil
}

func scanArticle(row pgx.Row) (articles.Article, error) {
	var (
		a      articles.Article
		siteID uuid.NullUUID
	)
	err := row.Scan(
		&a.ID,
		&a.Keyword,
		&siteID,
		&a.Site,
		&a.Status,
		&a.WPPostID,
		&a.WPEditURL,
		&a.WPPostURL,
		&a.ImagesRequested,
		&a.ImagesResolved,
		&a.ImagesSkipped,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	a.SiteID = siteID.UUID
	return a, err
}

func scanArticleFull(row pgx.Row) (articles.Article, error) {
	var (
		a      articles.Article
		siteID uuid.NullUUID
	)
	err := row.Scan(
		&a.ID,
		&a.Keyword,
		&siteID,
		&a.Site,
		&a.Status,
		&a.WPPostID,
		&a.WPEditURL,
		&a.WPPostURL,
		&a.ImagesRequested,
		&a.ImagesResolved,
		&a.ImagesSkipped,
		&a.CompetitorData,
		&a.CheckResult,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	a.SiteID = siteID.UUID
	return a, err
}
