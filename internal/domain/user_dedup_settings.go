package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type UserDedupSettings struct {
	bun.BaseModel `bun:"table:user_dedup_settings,alias:uds"`

	UserID    string    `bun:",pk,type:text"                               json:"user_id"`
	Schedule  string    `bun:",notnull,default:'0 2 * * *'"                json:"schedule"`
	Enabled   bool      `bun:",notnull,default:false"                      json:"enabled"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
