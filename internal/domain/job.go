package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs,alias:j"`

	ID        string    `bun:",pk,type:text" json:"id"`
	Type      string    `bun:",notnull" json:"type"`
	Payload   string    `bun:",type:text,default:'{}'" json:"payload"`
	Status    string    `bun:",notnull,default:'pending'" json:"status"` // pending, running, completed, failed
	Error     string    `bun:",default:''" json:"error,omitempty"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
