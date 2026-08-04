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
	Alias       string
	URL         string
	Username    string
	AppPassword string
}

type UpdateSite struct {
	Alias       *string
	URL         *string
	Username    *string
	AppPassword *string
	Enabled     *bool
}

type Credentials struct {
	URL         string
	Username    string
	AppPassword string
}

type Repository interface {
	Create(ctx context.Context, in CreateSite) (Site, error)
	List(ctx context.Context) ([]Site, error)
	Get(ctx context.Context, id uuid.UUID) (Site, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateSite) (Site, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Credentials(ctx context.Context, id uuid.UUID) (Credentials, error)
}
