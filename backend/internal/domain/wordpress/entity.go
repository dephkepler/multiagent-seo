package wordpress

import (
	"time"

	"github.com/google/uuid"
)

type Site struct {
	ID        uuid.UUID
	Alias     string
	URL       string
	Username  string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
