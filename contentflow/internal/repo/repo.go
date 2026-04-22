package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusGenerating = "generating"
	StatusDraft      = "draft"
	StatusFailed     = "failed"
)

type Article struct {
	ID        int64
	Keyword   string
	Status    string
	WPPostID  int64
	WPEditURL string
	WPPostURL string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repo struct {
	db *pgxpool.Pool
}

func New(ctx context.Context, url string) (*Repo, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to db: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Repo{db: db}, nil
}

func (r *Repo) Close() {
	r.db.Close()
}

// scannable abstracts pgx.Row and pgx.Rows — both implement Scan(...any) error.
type scannable interface {
	Scan(dest ...any) error
}

// scanArticle reads one article row from a pgx.Row or pgx.Rows.
func scanArticle(s scannable, a *Article) error {
	return s.Scan(
		&a.ID,
		&a.Keyword,
		&a.Status,
		&a.WPPostID,
		&a.WPEditURL,
		&a.WPPostURL,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
}

func (r *Repo) CreateArticle(ctx context.Context, keyword string) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO articles (keyword, status) VALUES ($1, $2) RETURNING id
	`, keyword, StatusGenerating).Scan(&id)
	return id, err
}

func (r *Repo) UpdateDraft(ctx context.Context, id, wpPostID int64, editURL string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE articles SET status=$1, wp_post_id=$2, wp_edit_url=$3, updated_at=NOW() WHERE id=$4
	`, StatusDraft, wpPostID, editURL, id)
	return err
}

func (r *Repo) MarkFailed(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE articles SET status=$1, updated_at=NOW() WHERE id=$2
	`, StatusFailed, id)
	return err
}

func (r *Repo) GetArticle(ctx context.Context, id int64) (*Article, error) {
	var a Article
	row := r.db.QueryRow(ctx, `
		SELECT id, keyword, status,
		       COALESCE(wp_post_id, 0),
		       COALESCE(wp_edit_url, ''),
		       COALESCE(wp_post_url, ''),
		       created_at, updated_at
		FROM articles WHERE id=$1
	`, id)
	if err := scanArticle(row, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repo) ListArticles(ctx context.Context) ([]Article, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, keyword, status,
		       COALESCE(wp_post_id, 0),
		       COALESCE(wp_edit_url, ''),
		       COALESCE(wp_post_url, ''),
		       created_at, updated_at
		FROM articles ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Article{}
	for rows.Next() {
		var a Article
		if err := scanArticle(rows, &a); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}
