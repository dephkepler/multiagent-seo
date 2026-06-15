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
		INSERT INTO articles (keyword, site_id, site, status, request_params)
		VALUES (@keyword, @site_id, @site, @status, @request_params)
		RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"keyword":        in.Keyword,
		"site_id":        in.SiteID,
		"site":           in.Site,
		"status":         articles.StatusGenerating,
		"request_params": in.RequestParams,
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
		       competitor_data, check_result, request_params,
		       created_at, updated_at, published_at, human_rating
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

func (r *ArticleRepository) List(ctx context.Context, limit, offset int) ([]articles.Article, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM articles`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count articles: %w", err)
	}

	const q = `
		SELECT id, keyword, site_id, site, status,
		       COALESCE(wp_post_id, 0),
		       COALESCE(wp_edit_url, ''),
		       COALESCE(wp_post_url, ''),
		       images_requested, images_resolved, images_skipped,
		       created_at, updated_at, human_rating
		FROM articles ORDER BY created_at DESC LIMIT @limit OFFSET @offset`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"limit": limit, "offset": offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()

	list := make([]articles.Article, 0)
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan article: %w", err)
		}
		list = append(list, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	return list, total, nil
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

func (r *ArticleRepository) FailOrphanedGenerating(ctx context.Context) (int64, error) {
	const q = `UPDATE articles SET status = @failed, updated_at = NOW() WHERE status = @generating`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"failed":     articles.StatusFailed,
		"generating": articles.StatusGenerating,
	})
	if err != nil {
		return 0, fmt.Errorf("fail orphaned generating: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *ArticleRepository) MarkPublished(ctx context.Context, id int64, postURL string) error {
	const q = `
		UPDATE articles
		SET status = @status, wp_post_url = @wp_post_url, published_at = NOW(), updated_at = NOW()
		WHERE id = @id`

	return r.exec(ctx, "mark published", q, pgx.NamedArgs{
		"status":      articles.StatusPublished,
		"wp_post_url": postURL,
		"id":          id,
	})
}

func (r *ArticleRepository) SetHumanRating(ctx context.Context, id int64, rating *bool) error {
	const q = `UPDATE articles SET human_rating = @rating WHERE id = @id`
	return r.exec(ctx, "set human rating", q, pgx.NamedArgs{"rating": rating, "id": id})
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

func (r *ArticleRepository) SaveCompetitorData(ctx context.Context, id int64, competitor any) error {
	b, err := json.Marshal(competitor)
	if err != nil {
		return fmt.Errorf("marshal competitor data: %w", err)
	}
	const q = `UPDATE articles SET competitor_data = @data, updated_at = NOW() WHERE id = @id`
	return r.exec(ctx, "save competitor data", q, pgx.NamedArgs{"data": b, "id": id})
}

func (r *ArticleRepository) SaveCheckResult(ctx context.Context, id int64, check any) error {
	b, err := json.Marshal(check)
	if err != nil {
		return fmt.Errorf("marshal check result: %w", err)
	}
	const q = `UPDATE articles SET check_result = @result, updated_at = NOW() WHERE id = @id`
	return r.exec(ctx, "save check result", q, pgx.NamedArgs{"result": b, "id": id})
}

func (r *ArticleRepository) SaveRevision(ctx context.Context, rev articles.Revision) (int, error) {
	const q = `
		INSERT INTO article_revisions
			(article_id, version, source, content_md, content_html, seo_title, seo_description, word_count)
		SELECT @article_id, COALESCE(MAX(version), 0) + 1, @source, @content_md, @content_html, @seo_title, @seo_description, @word_count
		FROM article_revisions WHERE article_id = @article_id
		RETURNING version`

	var version int
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"article_id":      rev.ArticleID,
		"source":          string(rev.Source),
		"content_md":      rev.ContentMD,
		"content_html":    rev.ContentHTML,
		"seo_title":       rev.SEOTitle,
		"seo_description": rev.SEODescription,
		"word_count":      rev.WordCount,
	}).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("save revision: %w", mapPGError(err))
	}
	return version, nil
}

func (r *ArticleRepository) SaveEvent(ctx context.Context, ev articles.GenerationEvent) error {
	const q = `
		INSERT INTO generation_events
			(article_id, stage, provider, model, input_tokens, output_tokens, latency_ms, ok)
		VALUES (@article_id, @stage, @provider, @model, @input_tokens, @output_tokens, @latency_ms, @ok)`
	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"article_id":    ev.ArticleID,
		"stage":         ev.Stage,
		"provider":      ev.Provider,
		"model":         ev.Model,
		"input_tokens":  ev.InputTokens,
		"output_tokens": ev.OutputTokens,
		"latency_ms":    ev.LatencyMS,
		"ok":            ev.OK,
	})
	if err != nil {
		return fmt.Errorf("save generation event: %w", err)
	}
	return nil
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
		&a.HumanRating,
	)
	a.SiteID = siteID.UUID
	if err != nil {
		return a, fmt.Errorf("scan article row: %w", err)
	}
	return a, nil
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
		&a.RequestParams,
		&a.CreatedAt,
		&a.UpdatedAt,
		&a.PublishedAt,
		&a.HumanRating,
	)
	a.SiteID = siteID.UUID
	if err != nil {
		return a, fmt.Errorf("scan article row: %w", err)
	}
	return a, nil
}
