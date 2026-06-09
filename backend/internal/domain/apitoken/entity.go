package apitoken

import (
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Prefix    string
	CreatedAt time.Time
	RevokedAt *time.Time
}
