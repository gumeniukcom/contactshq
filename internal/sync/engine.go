package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"go.uber.org/zap"
)

// syncRunStatus values
const (
	syncRunStatusRunning   = "running"
	syncRunStatusCompleted = "completed"
	syncRunStatusFailed    = "failed"
)

// ConflictMode determines how to handle sync conflicts.
type ConflictMode string

const (
	ConflictSourceWins ConflictMode = "source_wins" // remote overwrites local
	ConflictDestWins   ConflictMode = "dest_wins"   // local is kept
	ConflictSkip       ConflictMode = "skip"        // skip conflicting item
	ConflictAuto       ConflictMode = "auto"        // three-way merge; unresolvable → queue + skip
	ConflictManual     ConflictMode = "manual"      // always queue for user review + skip
)

// SyncMode controls the direction of synchronisation.
type SyncMode string

const (
	SyncModePull          SyncMode = "pull"          // remote → local (default)
	SyncModePush          SyncMode = "push"          // local → remote
	SyncModeBidirectional SyncMode = "bidirectional" // pull then push
)

// SyncResult summarises one Sync invocation.
type SyncResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Deleted   int `json:"deleted"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
	Conflicts int `json:"conflicts"`
}

// Engine performs contact synchronisation between two SyncProviders.
type Engine struct {
	syncRepo     repository.SyncStateRepository
	syncRunRepo  repository.SyncRunRepository      // optional — may be nil
	conflictRepo repository.SyncConflictRepository // optional — may be nil
	logger       *zap.Logger
}

func NewEngine(syncRepo repository.SyncStateRepository, logger *zap.Logger) *Engine {
	return &Engine{syncRepo: syncRepo, logger: logger}
}

func NewEngineWithRunRepo(syncRepo repository.SyncStateRepository, runRepo repository.SyncRunRepository, logger *zap.Logger) *Engine {
	return &Engine{syncRepo: syncRepo, syncRunRepo: runRepo, logger: logger}
}

func NewEngineWithAllRepos(
	syncRepo repository.SyncStateRepository,
	runRepo repository.SyncRunRepository,
	conflictRepo repository.SyncConflictRepository,
	logger *zap.Logger,
) *Engine {
	return &Engine{syncRepo: syncRepo, syncRunRepo: runRepo, conflictRepo: conflictRepo, logger: logger}
}

// Sync synchronises source → dest (and optionally dest → source when mode is bidirectional or push).
// Backward-compatible: callers that don't pass mode get SyncModePull behaviour.
func (e *Engine) Sync(ctx context.Context, userID, pipelineID string, source, dest SyncProvider, conflictMode ConflictMode, modes ...SyncMode) (*SyncResult, error) {
	mode := SyncModePull
	if len(modes) > 0 {
		mode = modes[0]
	}

	providerKey := source.Name() + "->" + dest.Name()

	var run *domain.SyncRun
	if e.syncRunRepo != nil {
		run = &domain.SyncRun{
			ID:           uuid.New().String(),
			UserID:       userID,
			PipelineID:   pipelineID,
			ProviderType: providerKey,
			Status:       syncRunStatusRunning,
			StartedAt:    time.Now(),
		}
		if err := e.syncRunRepo.Create(ctx, run); err != nil {
			e.logger.Warn("failed to create sync run record", zap.Error(err))
			run = nil
		}
	}

	result, err := e.doSync(ctx, userID, providerKey, source, dest, conflictMode, mode)

	if run != nil && e.syncRunRepo != nil {
		finished := time.Now()
		run.FinishedAt = &finished
		if err != nil {
			run.Status = syncRunStatusFailed
			run.ErrorMessage = err.Error()
		} else {
			run.Status = syncRunStatusCompleted
			run.CreatedCount = result.Created
			run.UpdatedCount = result.Updated
			run.DeletedCount = result.Deleted
			run.ErrorCount = result.Errors
		}
		if updateErr := e.syncRunRepo.Update(ctx, run); updateErr != nil {
			e.logger.Warn("failed to update sync run record", zap.Error(updateErr))
		}
	}

	return result, err
}

func (e *Engine) doSync(ctx context.Context, userID, providerKey string, source, dest SyncProvider, conflictMode ConflictMode, mode SyncMode) (*SyncResult, error) {
	result := &SyncResult{}

	if mode == SyncModePull || mode == SyncModeBidirectional {
		if err := e.pullPhase(ctx, userID, providerKey, source, dest, conflictMode, result); err != nil {
			return result, err
		}
	}

	if mode == SyncModePush || mode == SyncModeBidirectional {
		if err := e.pushPhase(ctx, userID, providerKey, dest, source, result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// pullPhase syncs items from source into dest (remote → local).
func (e *Engine) pullPhase(ctx context.Context, userID, providerKey string, source, dest SyncProvider, conflictMode ConflictMode, result *SyncResult) error {
	sourceItems, err := source.List(ctx)
	if err != nil {
		return fmt.Errorf("list source items: %w", err)
	}

	destItems, err := dest.List(ctx)
	if err != nil {
		return fmt.Errorf("list dest items: %w", err)
	}

	prevStates, err := e.syncRepo.ListByUser(ctx, userID, providerKey)
	if err != nil {
		return fmt.Errorf("list sync states: %w", err)
	}

	sourceMap := make(map[string]SyncItem, len(sourceItems))
	for _, item := range sourceItems {
		sourceMap[item.RemoteID] = item
	}

	destMap := make(map[string]SyncItem, len(destItems))
	for _, item := range destItems {
		destMap[item.RemoteID] = item
	}

	prevStateMap := make(map[string]*domain.SyncState, len(prevStates))
	for _, s := range prevStates {
		prevStateMap[s.RemoteID] = s
	}

	now := time.Now()

	// Process source items
	for remoteID, srcItem := range sourceMap {
		prev := prevStateMap[remoteID]

		if prev == nil {
			// NEW on source → Put to dest
			newETag, err := dest.Put(ctx, srcItem)
			if err != nil {
				e.logger.Error("sync: failed to put new item to dest", zap.String("remote_id", remoteID), zap.Error(err))
				result.Errors++
				continue
			}

			state := &domain.SyncState{
				ID:           uuid.New().String(),
				UserID:       userID,
				ProviderType: providerKey,
				RemoteID:     remoteID,
				LocalID:      remoteID,
				RemoteETag:   srcItem.ETag,
				LocalETag:    newETag,
				ContentHash:  contentHash(srcItem.VCardData),
				BaseVCard:    srcItem.VCardData,
				LastSyncedAt: now,
			}
			if err := e.syncRepo.Create(ctx, state); err != nil {
				result.Errors++
				continue
			}
			result.Created++
			continue
		}

		// Check if source modified
		sourceModified := prev.RemoteETag != srcItem.ETag || prev.ContentHash != contentHash(srcItem.VCardData)
		if !sourceModified {
			continue
		}

		// Source modified — check dest
		destItem, destExists := destMap[remoteID]
		destModified := destExists && prev.LocalETag != destItem.ETag

		if destModified {
			result.Conflicts++

			// Attempt three-way merge
			mergeResult, mergeErr := MergeVCards(prev.BaseVCard, destItem.VCardData, srcItem.VCardData)
			if mergeErr == nil && mergeResult.AutoMerged {
				// Auto-merge succeeded — apply merged vCard to dest
				newETag, putErr := dest.Put(ctx, SyncItem{
					RemoteID:  remoteID,
					ETag:      srcItem.ETag,
					VCardData: mergeResult.MergedVCard,
				})
				if putErr != nil {
					result.Errors++
					continue
				}
				prev.RemoteETag = srcItem.ETag
				prev.LocalETag = newETag
				prev.ContentHash = contentHash(mergeResult.MergedVCard)
				prev.BaseVCard = mergeResult.MergedVCard
				prev.LastSyncedAt = now
				if err := e.syncRepo.Update(ctx, prev); err != nil {
					result.Errors++
					continue
				}
				result.Updated++
				continue
			}

			// Auto-merge failed or merge error — queue conflict record
			if e.conflictRepo != nil {
				var diffs []FieldConflict
				if mergeErr == nil {
					diffs = mergeResult.Conflicts
				}
				diffsJSON, _ := json.Marshal(diffs)
				conflict := &domain.SyncConflict{
					ID:             uuid.New().String(),
					UserID:         userID,
					ProviderType:   providerKey,
					RemoteID:       remoteID,
					LocalContactID: prev.LocalID,
					BaseVCard:      prev.BaseVCard,
					LocalVCard:     destItem.VCardData,
					RemoteVCard:    srcItem.VCardData,
					FieldDiffs:     string(diffsJSON),
					Status:         "pending",
					CreatedAt:      now,
				}
				if createErr := e.conflictRepo.Create(ctx, conflict); createErr != nil {
					e.logger.Warn("failed to create conflict record", zap.Error(createErr))
				}
			}

			// Apply conflict mode
			switch conflictMode {
			case ConflictSourceWins:
				// Fall through to put below
			case ConflictAuto, ConflictManual, ConflictDestWins, ConflictSkip:
				result.Skipped++
				continue
			}
		}

		// Apply source → dest
		newETag, err := dest.Put(ctx, srcItem)
		if err != nil {
			result.Errors++
			continue
		}

		prev.RemoteETag = srcItem.ETag
		prev.LocalETag = newETag
		prev.ContentHash = contentHash(srcItem.VCardData)
		prev.BaseVCard = srcItem.VCardData
		prev.LastSyncedAt = now
		if err := e.syncRepo.Update(ctx, prev); err != nil {
			result.Errors++
			continue
		}
		result.Updated++
	}

	// Handle deletions (items in prev state but not in source)
	for remoteID, prev := range prevStateMap {
		if _, exists := sourceMap[remoteID]; !exists {
			if err := dest.Delete(ctx, remoteID); err != nil {
				e.logger.Error("sync: failed to delete from dest", zap.String("remote_id", remoteID), zap.Error(err))
				result.Errors++
				continue
			}
			if err := e.syncRepo.Delete(ctx, prev.ID); err != nil {
				result.Errors++
				continue
			}
			result.Deleted++
		}
	}

	return nil
}

// pushPhase syncs locally-changed items from local (dest) back to remote (source).
// "local" is the internal provider, "remote" is the external provider.
func (e *Engine) pushPhase(ctx context.Context, userID, providerKey string, local, remote SyncProvider, result *SyncResult) error {
	localItems, err := local.List(ctx)
	if err != nil {
		return fmt.Errorf("push: list local items: %w", err)
	}

	prevStates, err := e.syncRepo.ListByUser(ctx, userID, providerKey)
	if err != nil {
		return fmt.Errorf("push: list sync states: %w", err)
	}

	localMap := make(map[string]SyncItem, len(localItems))
	for _, item := range localItems {
		localMap[item.RemoteID] = item
	}

	prevStateMap := make(map[string]*domain.SyncState, len(prevStates))
	for _, s := range prevStates {
		prevStateMap[s.RemoteID] = s
	}

	now := time.Now()

	// Push locally-changed contacts to remote
	for remoteID, localItem := range localMap {
		prev, exists := prevStateMap[remoteID]
		if !exists {
			// Not yet tracked — push as new
			newETag, err := remote.Put(ctx, localItem)
			if err != nil {
				e.logger.Error("push: failed to put new item to remote", zap.String("remote_id", remoteID), zap.Error(err))
				result.Errors++
				continue
			}
			state := &domain.SyncState{
				ID:           uuid.New().String(),
				UserID:       userID,
				ProviderType: providerKey,
				RemoteID:     remoteID,
				LocalID:      remoteID,
				RemoteETag:   newETag,
				LocalETag:    localItem.ETag,
				ContentHash:  contentHash(localItem.VCardData),
				BaseVCard:    localItem.VCardData,
				LastSyncedAt: now,
			}
			if err := e.syncRepo.Create(ctx, state); err != nil {
				result.Errors++
				continue
			}
			result.Created++
			continue
		}

		// Check if local changed since last sync
		if prev.LocalETag == localItem.ETag {
			continue // no local change
		}

		newETag, err := remote.Put(ctx, localItem)
		if err != nil {
			e.logger.Error("push: failed to put changed item to remote", zap.String("remote_id", remoteID), zap.Error(err))
			result.Errors++
			continue
		}

		prev.RemoteETag = newETag
		prev.LocalETag = localItem.ETag
		prev.ContentHash = contentHash(localItem.VCardData)
		prev.BaseVCard = localItem.VCardData
		prev.LastSyncedAt = now
		if err := e.syncRepo.Update(ctx, prev); err != nil {
			result.Errors++
			continue
		}
		result.Updated++
	}

	// Push deletions: contacts removed locally should be removed from remote
	for remoteID, prev := range prevStateMap {
		if _, exists := localMap[remoteID]; !exists {
			if err := remote.Delete(ctx, remoteID); err != nil {
				e.logger.Error("push: failed to delete from remote", zap.String("remote_id", remoteID), zap.Error(err))
				result.Errors++
				continue
			}
			if err := e.syncRepo.Delete(ctx, prev.ID); err != nil {
				result.Errors++
				continue
			}
			result.Deleted++
		}
	}

	return nil
}

func contentHash(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
