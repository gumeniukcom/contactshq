package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// MergeLogEntry records one merge, independently of the contacts it involved.
//
// It holds no foreign key to contacts on purpose: potential_duplicates cascades on contact
// deletion, and history that vanished when the winner was later deleted would be worthless.
type MergeLogEntry struct {
	bun.BaseModel `bun:"table:merge_log,alias:ml"`

	// Explicit column names throughout: Bun's default mapping turns WinnerID into
	// winner_i_d and LoserUID into loser_u_i_d.
	ID                string    `bun:"id,pk,type:text"                            json:"id"`
	UserID            string    `bun:"user_id,notnull"                            json:"user_id"`
	WinnerID          string    `bun:"winner_id"                                  json:"winner_id"`
	WinnerDisplayName string    `bun:"winner_display_name"                        json:"winner_display_name"`
	LoserUID          string    `bun:"loser_uid"                                  json:"loser_uid"`
	LoserDisplayName  string    `bun:"loser_display_name"                         json:"loser_display_name"`
	LoserVCard        string    `bun:"loser_vcard"                                json:"loser_vcard,omitempty"`
	Resolution        string    `bun:"resolution"                                 json:"resolution"`
	MergedAt          time.Time `bun:"merged_at,nullzero,notnull,default:current_timestamp" json:"merged_at"`
}
