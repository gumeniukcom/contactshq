package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type UserBackupSettings struct {
	bun.BaseModel `bun:"table:user_backup_settings,alias:ubs"`

	UserID    string    `bun:",pk,type:text"                               json:"user_id"`
	Schedule  string    `bun:",notnull,default:'0 2 * * *'"                json:"schedule"`
	Retention int       `bun:",notnull,default:7"                          json:"retention"`
	Enabled   bool      `bun:",notnull,default:true"                       json:"enabled"`
	Compress  bool      `bun:",notnull,default:false"                      json:"compress"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
