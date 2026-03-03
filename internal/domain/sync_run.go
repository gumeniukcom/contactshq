package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type SyncRun struct {
	bun.BaseModel `bun:"table:sync_runs,alias:sr"`

	ID           string     `bun:",pk,type:text"                              json:"id"`
	UserID       string     `bun:",notnull"                                    json:"user_id"`
	PipelineID   string     `bun:",notnull,default:''"                         json:"pipeline_id,omitempty"`
	ProviderType string     `bun:",notnull"                                    json:"provider_type"`
	Status       string     `bun:",notnull,default:'running'"                  json:"status"`
	CreatedCount int        `bun:",notnull,default:0"                          json:"created_count"`
	UpdatedCount int        `bun:",notnull,default:0"                          json:"updated_count"`
	DeletedCount int        `bun:",notnull,default:0"                          json:"deleted_count"`
	ErrorCount   int        `bun:",notnull,default:0"                          json:"error_count"`
	ErrorMessage string     `bun:",notnull,default:''"                         json:"error_message,omitempty"`
	StartedAt    time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"started_at"`
	FinishedAt   *time.Time `bun:",nullzero"                                   json:"finished_at,omitempty"`
}
