package pg

import (
	"time"

	"github.com/google/uuid"
)

type Model struct {
	ID        uuid.UUID `gorm:"primaryKey;uniqueIndex;unique;index;type:uuid;default:gen_random_uuid()"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
}
