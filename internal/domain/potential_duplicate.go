package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type PotentialDuplicate struct {
	bun.BaseModel `bun:"table:potential_duplicates,alias:pd"`

	ID           string    `bun:",pk,type:text"                              json:"id"`
	UserID       string    `bun:",notnull"                                   json:"user_id"`
	ContactAID   string    `bun:",notnull"                                   json:"contact_a_id"`
	ContactBID   string    `bun:",notnull"                                   json:"contact_b_id"`
	Score        float64   `bun:",notnull"                                   json:"score"`
	MatchReasons string    `bun:"match_reasons,notnull,default:'[]'"          json:"match_reasons"` // JSON []string
	Status       string    `bun:",notnull,default:'pending'"                 json:"status"`
	CreatedAt    time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`

	ContactA *Contact `bun:"rel:belongs-to,join:contact_a_id=id" json:"contact_a,omitempty"`
	ContactB *Contact `bun:"rel:belongs-to,join:contact_b_id=id" json:"contact_b,omitempty"`
}
