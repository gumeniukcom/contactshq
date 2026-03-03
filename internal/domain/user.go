package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID           string    `bun:",pk,type:text" json:"id"`
	Email        string    `bun:",unique,notnull" json:"email"`
	PasswordHash string    `bun:",notnull" json:"-"`
	DisplayName  string    `bun:",default:''" json:"display_name"`
	Role         string    `bun:",notnull,default:'user'" json:"role"`
	CreatedAt    time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt    time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
