package domain

import (
	"time"

	"github.com/uptrace/bun"
)

type Pipeline struct {
	bun.BaseModel `bun:"table:pipelines,alias:p"`

	ID        string    `bun:",pk,type:text" json:"id"`
	UserID    string    `bun:",notnull" json:"user_id"`
	Name      string    `bun:",notnull" json:"name"`
	Enabled   bool      `bun:",notnull,default:true" json:"enabled"`
	Schedule  string    `bun:",default:''" json:"schedule"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	Steps []*PipelineStep `bun:"rel:has-many,join:id=pipeline_id" json:"steps,omitempty"`
}

type PipelineStep struct {
	bun.BaseModel `bun:"table:pipeline_steps,alias:ps"`

	ID           string `bun:",pk,type:text" json:"id"`
	PipelineID   string `bun:",notnull" json:"pipeline_id"`
	Order        int    `bun:"step_order,notnull" json:"order"`
	SourceType   string `bun:",notnull" json:"source_type"`
	SourceConfig string `bun:",default:'{}'" json:"source_config"`
	DestType     string `bun:",notnull" json:"dest_type"`
	DestConfig   string `bun:",default:'{}'" json:"dest_config"`
	ConflictMode string `bun:",notnull,default:'source_wins'" json:"conflict_mode"`
	Direction    string `bun:",notnull,default:'pull'" json:"direction"`
}
