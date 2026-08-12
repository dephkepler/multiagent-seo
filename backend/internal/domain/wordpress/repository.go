package wordpress

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound    = errors.New("wordpress site not found")
	ErrAliasExists = errors.New("wordpress site alias already exists")
)

type CreateSite struct {
	Alias    string
	Provider Provider
	URL      string

	Username    string
	AppPassword string

	Languages map[string]LanguageConfig
}

type UpdateSite struct {
	Alias   *string
	URL     *string
	Enabled *bool

	Username    *string
	AppPassword *string

	// Languages, when non-nil, replaces the whole map — there's no partial
	// per-language patch, the caller always submits the full picture.
	Languages map[string]LanguageConfig
}

type Credentials struct {
	Provider Provider
	URL      string

	Username    string
	AppPassword string

	Languages map[string]LanguageConfig
}

type Repository interface {
	Create(ctx context.Context, in CreateSite) (Site, error)
	List(ctx context.Context) ([]Site, error)
	Get(ctx context.Context, id uuid.UUID) (Site, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateSite) (Site, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Credentials(ctx context.Context, id uuid.UUID) (Credentials, error)
}
