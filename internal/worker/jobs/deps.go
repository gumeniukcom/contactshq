package jobs

import (
	"context"

	"github.com/gumeniukcom/contactshq/internal/service"
)

// The handlers used to take the concrete *service.BackupService and *service.DuplicateDetector,
// which meant testing "does a failure come back wrapped with the user id?" required a real
// database and a real filesystem — so nobody tested it, and the only test in this package
// checked payload parsing.
//
// The interfaces are declared here, in the consumer, and stay as narrow as what the handlers
// actually call. The composition root passes the same concrete services as before.

// BackupCreator creates a backup and records why it was started.
type BackupCreator interface {
	CreateWithTrigger(ctx context.Context, userID, trigger string) (*service.BackupInfo, error)
}

// DuplicateScanner scans a user's address book for duplicate pairs.
type DuplicateScanner interface {
	Detect(ctx context.Context, userID string) (*service.DetectionResult, error)
}
