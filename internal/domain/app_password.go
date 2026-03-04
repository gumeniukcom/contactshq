package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type AppPassword struct {
	bun.BaseModel `bun:"table:app_passwords,alias:ap"`

	ID           string     `bun:",pk,type:text" json:"id"`
	UserID       string     `bun:",notnull" json:"user_id"`
	Label        string     `bun:",notnull,default:''" json:"label"`
	PasswordHash string     `bun:",notnull" json:"-"`
	LastUsedAt   *time.Time `bun:",nullzero" json:"last_used_at"`
	CreatedAt    time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
}
