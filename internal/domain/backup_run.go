package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// Backup run statuses.
const (
	BackupRunRunning = "running"
	BackupRunOK      = "completed"
	BackupRunFailed  = "failed"
	// BackupRunInterrupted marks a run whose process died before it could finish. Without
	// it, a killed container leaves rows that look like backups still in progress forever.
	BackupRunInterrupted = "interrupted"
)

// What started a backup.
const (
	BackupTriggerManual    = "manual"
	BackupTriggerScheduled = "scheduled"
	// BackupTriggerCatchup is a run started because the server noticed on boot that the last
	// successful backup was older than the schedule allows.
	BackupTriggerCatchup = "catchup"
)

// BackupRun records one attempt to create a backup.
//
// It exists because the files on disk cannot answer "when did this last succeed": retention
// deletes them, and at retention=1 the only surviving file is always the newest one.
type BackupRun struct {
	bun.BaseModel `bun:"table:backup_runs,alias:br"`

	ID           string     `bun:"id,pk,type:text"                            json:"id"`
	UserID       string     `bun:"user_id,notnull"                            json:"user_id"`
	Trigger      string     `bun:"trigger,notnull,default:'manual'"           json:"trigger"`
	Status       string     `bun:"status,notnull,default:'running'"           json:"status"`
	Filename     string     `bun:"filename,notnull,default:''"                json:"filename,omitempty"`
	SizeBytes    int64      `bun:"size_bytes,notnull,default:0"               json:"size_bytes"`
	ContactCount int        `bun:"contact_count,notnull,default:0"            json:"contact_count"`
	Compressed   bool       `bun:"compressed,notnull,default:false"           json:"compressed"`
	ErrorMessage string     `bun:"error_message,notnull,default:''"           json:"error_message,omitempty"`
	StartedAt    time.Time  `bun:"started_at,nullzero,notnull,default:current_timestamp" json:"started_at"`
	FinishedAt   *time.Time `bun:"finished_at,nullzero"                       json:"finished_at,omitempty"`
}
