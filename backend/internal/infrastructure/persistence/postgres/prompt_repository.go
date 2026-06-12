package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/articles"
)

type PromptRepository struct {
	db *pgxpool.Pool
}

func NewPromptRepository(pool *pgxpool.Pool) *PromptRepository {
	return &PromptRepository{db: pool}
}

func (r *PromptRepository) ActiveVariants(ctx context.Context, stage string) ([]articles.PromptVariant, error) {
	const q = `
		SELECT id, stage, body, status, origin, parent_id, created_at
		FROM prompt_variants
		WHERE stage = @stage AND status <> @retired
		ORDER BY id`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"stage": stage, "retired": articles.VariantRetired})
	if err != nil {
		return nil, fmt.Errorf("active variants: %w", err)
	}
	defer rows.Close()

	out := make([]articles.PromptVariant, 0)
	for rows.Next() {
		var v articles.PromptVariant
		if err := rows.Scan(&v.ID, &v.Stage, &v.Body, &v.Status, &v.Origin, &v.ParentID, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan prompt variant: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("active variants: %w", err)
	}
	return out, nil
}

func (r *PromptRepository) InsertVariant(ctx context.Context, v articles.PromptVariant) (int64, error) {
	const q = `
		INSERT INTO prompt_variants (stage, body, status, origin, parent_id)
		VALUES (@stage, @body, @status, @origin, @parent_id)
		RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"stage":     v.Stage,
		"body":      v.Body,
		"status":    v.Status,
		"origin":    v.Origin,
		"parent_id": v.ParentID,
	}).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert prompt variant: %w", err)
	}
	return id, nil
}

func (r *PromptRepository) SaveOutcome(ctx context.Context, o articles.PromptOutcome) error {
	const q = `
		INSERT INTO prompt_outcomes
			(article_id, stage, variant_id, reward, ai_score, quality_ok, humanize_cycles, tokens)
		VALUES (@article_id, @stage, @variant_id, @reward, @ai_score, @quality_ok, @humanize_cycles, @tokens)`
	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"article_id":      o.ArticleID,
		"stage":           o.Stage,
		"variant_id":      o.VariantID,
		"reward":          o.Reward,
		"ai_score":        o.AIScore,
		"quality_ok":      o.QualityOK,
		"humanize_cycles": o.HumanizeCycles,
		"tokens":          o.Tokens,
	})
	if err != nil {
		return fmt.Errorf("save prompt outcome: %w", err)
	}
	return nil
}

func (r *PromptRepository) SelectionStats(ctx context.Context, stage string, since time.Time) ([]articles.PromptVariantStat, error) {
	const q = `
		SELECT v.id, v.stage, v.body, v.status, v.origin, v.parent_id, v.created_at,
		       count(o.id) FILTER (WHERE o.reward IS NOT NULL)                    AS samples,
		       coalesce(sum(o.reward) FILTER (WHERE o.reward IS NOT NULL), 0)     AS sum_reward,
		       coalesce(sum(1 - o.reward) FILTER (WHERE o.reward IS NOT NULL), 0) AS sum_complement
		FROM prompt_variants v
		LEFT JOIN prompt_outcomes o
		       ON o.variant_id = v.id AND o.created_at >= @since
		WHERE v.stage = @stage AND v.status <> @retired
		GROUP BY v.id
		ORDER BY v.id`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{
		"stage":   stage,
		"since":   since,
		"retired": articles.VariantRetired,
	})
	if err != nil {
		return nil, fmt.Errorf("selection stats: %w", err)
	}
	defer rows.Close()

	out := make([]articles.PromptVariantStat, 0)
	for rows.Next() {
		var s articles.PromptVariantStat
		if err := rows.Scan(
			&s.Variant.ID,
			&s.Variant.Stage,
			&s.Variant.Body,
			&s.Variant.Status,
			&s.Variant.Origin,
			&s.Variant.ParentID,
			&s.Variant.CreatedAt,
			&s.Samples,
			&s.SumReward,
			&s.SumComplement,
		); err != nil {
			return nil, fmt.Errorf("scan selection stat: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("selection stats: %w", err)
	}
	return out, nil
}

func (r *PromptRepository) SetVariantStatus(ctx context.Context, id int64, status articles.VariantStatus) error {
	const q = `UPDATE prompt_variants SET status = @status WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"status": status, "id": id})
	if err != nil {
		return fmt.Errorf("set prompt status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set prompt status: variant %d not found", id)
	}
	return nil
}
