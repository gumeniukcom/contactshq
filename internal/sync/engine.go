package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// SyncMode controls the direction of synchronisation. The internal address book is
// always the local side, so a mode names which way contacts move relative to it.
type SyncMode string

const (
	SyncModeImport SyncMode = "import"  // remote → local (default)
	SyncModeExport SyncMode = "export"  // local → remote
	SyncModeTwoWay SyncMode = "two_way" // import, then export
)

// ErrUnknownSyncMode keeps a typo in a stored pipeline from silently running no phase
// at all and reporting success.
var ErrUnknownSyncMode = errors.New("unknown sync direction")

// ParseSyncMode accepts the current vocabulary and the pull/push/bidirectional names it
// replaced. The old names survive because migration 019 left every step with the
// internal book as its destination, where pull always meant remote → local.
func ParseSyncMode(s string) (SyncMode, error) {
	switch s {
	case "", string(SyncModeImport), "pull":
		return SyncModeImport, nil
	case string(SyncModeExport), "push":
		return SyncModeExport, nil
	case string(SyncModeTwoWay), "bidirectional":
		return SyncModeTwoWay, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownSyncMode, s)
	}
}

// SyncResult summarises one Sync invocation.
type SyncResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Deleted   int `json:"deleted"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
	Conflicts int `json:"conflicts"`
}

// A provider that returns a short or empty listing — an expired token, a truncated
// page, a provider-side outage — is indistinguishable from one where the user deleted
// everything. Propagating that as deletions wipes the other side. Abort instead once
// the share of tracked items about to disappear crosses the threshold; a genuine bulk
// delete can be re-run after the user re-syncs in smaller steps or clears sync state.
const (
	deletionAbortRatio   = 0.5
	deletionAbortMinimum = 5
)

// ErrMassDeletion aborts a run before propagating an implausible number of deletions.
var ErrMassDeletion = errors.New("aborting sync: implausible number of deletions")

func checkDeletionSafety(toDelete, tracked int) error {
	if tracked < deletionAbortMinimum || toDelete == 0 {
		return nil
	}
	if float64(toDelete)/float64(tracked) > deletionAbortRatio {
		return fmt.Errorf("%w: %d of %d tracked contacts missing from the provider listing",
			ErrMassDeletion, toDelete, tracked)
	}
	return nil
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

// Sync moves contacts between an external provider and the internal address book.
// Callers that pass no mode get an import.
func (e *Engine) Sync(ctx context.Context, userID, pipelineID string, remote, local SyncProvider, conflictMode ConflictMode, modes ...SyncMode) (*SyncResult, error) {
	mode := SyncModeImport
	if len(modes) > 0 {
		mode = modes[0]
	}

	providerKey := remote.Name() + "->" + local.Name()

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

	result, err := e.doSync(ctx, userID, providerKey, remote, local, conflictMode, mode)

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

func (e *Engine) doSync(ctx context.Context, userID, providerKey string, remote, local SyncProvider, conflictMode ConflictMode, mode SyncMode) (*SyncResult, error) {
	result := &SyncResult{}

	if mode == SyncModeImport || mode == SyncModeTwoWay {
		if err := e.pullPhase(ctx, userID, providerKey, remote, local, conflictMode, result); err != nil {
			return result, err
		}
	}

	if mode == SyncModeExport || mode == SyncModeTwoWay {
		if err := e.pushPhase(ctx, userID, providerKey, local, remote, conflictMode, result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// pullPhase brings the remote provider's contacts into the local address book.
//
// sync_states rows map a remote id to a local id. The two are only equal by accident —
// for contacts first created locally and pushed out, the provider assigns its own remote
// id — so every lookup must go through the side it belongs to.
func (e *Engine) pullPhase(ctx context.Context, userID, providerKey string, remote, local SyncProvider, conflictMode ConflictMode, result *SyncResult) error {
	remoteItems, err := remote.List(ctx)
	if err != nil {
		return fmt.Errorf("list remote items: %w", err)
	}

	localItems, err := local.List(ctx)
	if err != nil {
		return fmt.Errorf("list local items: %w", err)
	}

	prevStates, err := e.syncRepo.ListByUser(ctx, userID, providerKey)
	if err != nil {
		return fmt.Errorf("list sync states: %w", err)
	}

	remoteMap := make(map[string]SyncItem, len(remoteItems))
	for _, item := range remoteItems {
		remoteMap[item.RemoteID] = item
	}

	localMap := make(map[string]SyncItem, len(localItems))
	for _, item := range localItems {
		localMap[item.RemoteID] = item
	}

	prevByRemoteID := make(map[string]*domain.SyncState, len(prevStates))
	for _, s := range prevStates {
		prevByRemoteID[s.RemoteID] = s
	}

	// Refuse to act on a listing that would erase most of what we track.
	missing := 0
	for remoteID := range prevByRemoteID {
		if _, exists := remoteMap[remoteID]; !exists {
			missing++
		}
	}
	if err := checkDeletionSafety(missing, len(prevByRemoteID)); err != nil {
		return err
	}

	pending := e.pendingConflictsByRemoteID(ctx, userID, providerKey)
	now := time.Now()

	// Process remote items
	for remoteID, remoteItem := range remoteMap {
		prev := prevByRemoteID[remoteID]

		if prev == nil {
			// New on the remote → create locally
			put, err := local.Put(ctx, remoteItem)
			if err != nil {
				e.logger.Error("sync: failed to create local contact", zap.String("remote_id", remoteID), zap.Error(err))
				result.Errors++
				continue
			}

			state := &domain.SyncState{
				ID:           uuid.New().String(),
				UserID:       userID,
				ProviderType: providerKey,
				RemoteID:     remoteID,
				LocalID:      put.RemoteID,
				RemoteETag:   remoteItem.ETag,
				LocalETag:    put.ETag,
				ContentHash:  contentHash(remoteItem.VCardData),
				BaseVCard:    remoteItem.VCardData,
				LastSyncedAt: now,
			}
			if err := e.syncRepo.Create(ctx, state); err != nil {
				result.Errors++
				continue
			}
			result.Created++
			continue
		}

		// Check if the remote changed
		remoteModified := prev.RemoteETag != remoteItem.ETag || prev.ContentHash != contentHash(remoteItem.VCardData)
		if !remoteModified {
			continue
		}

		// The remote changed — check the local side, addressing it by its own id.
		localItem, localExists := localMap[prev.LocalID]
		localModified := localExists && prev.LocalETag != localItem.ETag

		vcardToApply := remoteItem.VCardData

		if localModified {
			result.Conflicts++

			// Only auto and source_wins are allowed to resolve on their own. Manual must
			// never silently merge, which is the whole point of asking the user.
			var merged *MergeResult
			if conflictMode == ConflictAuto {
				if mergeResult, mergeErr := MergeVCards(prev.BaseVCard, localItem.VCardData, remoteItem.VCardData); mergeErr == nil && mergeResult.AutoMerged {
					merged = mergeResult
				}
			}

			if merged != nil {
				vcardToApply = merged.MergedVCard
			} else {
				if conflictMode == ConflictAuto || conflictMode == ConflictManual {
					e.recordConflict(ctx, userID, providerKey, now, prev, localItem, remoteItem, pending)
				}

				if conflictMode != ConflictSourceWins {
					result.Skipped++
					continue
				}
			}
		}

		put, err := local.Put(ctx, SyncItem{
			RemoteID:  prev.LocalID,
			ETag:      prev.LocalETag,
			VCardData: vcardToApply,
		})
		if err != nil {
			result.Errors++
			continue
		}

		prev.RemoteETag = remoteItem.ETag
		prev.LocalID = put.RemoteID
		prev.LocalETag = put.ETag
		prev.ContentHash = contentHash(vcardToApply)
		prev.BaseVCard = vcardToApply
		prev.LastSyncedAt = now
		if err := e.syncRepo.Update(ctx, prev); err != nil {
			result.Errors++
			continue
		}
		result.Updated++
	}

	// Handle deletions (tracked contacts the remote no longer lists)
	for remoteID, prev := range prevByRemoteID {
		if _, exists := remoteMap[remoteID]; !exists {
			if err := local.Delete(ctx, prev.LocalID); err != nil {
				e.logger.Error("sync: failed to delete local contact", zap.String("local_id", prev.LocalID), zap.Error(err))
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

// pendingConflictsByRemoteID indexes the conflicts already awaiting review, so a run
// updates them in place instead of appending a fresh row on every schedule tick.
func (e *Engine) pendingConflictsByRemoteID(ctx context.Context, userID, providerKey string) map[string]*domain.SyncConflict {
	if e.conflictRepo == nil {
		return nil
	}
	existing, err := e.conflictRepo.ListPendingByProvider(ctx, userID, providerKey)
	if err != nil {
		e.logger.Warn("failed to list pending conflicts", zap.Error(err))
		return map[string]*domain.SyncConflict{}
	}
	byRemoteID := make(map[string]*domain.SyncConflict, len(existing))
	for _, c := range existing {
		byRemoteID[c.RemoteID] = c
	}
	return byRemoteID
}

func (e *Engine) recordConflict(
	ctx context.Context,
	userID, providerKey string,
	now time.Time,
	prev *domain.SyncState,
	localItem, remoteItem SyncItem,
	pending map[string]*domain.SyncConflict,
) {
	if e.conflictRepo == nil {
		return
	}

	var diffs []FieldConflict
	if mergeResult, mergeErr := MergeVCards(prev.BaseVCard, localItem.VCardData, remoteItem.VCardData); mergeErr == nil {
		diffs = mergeResult.Conflicts
	}
	diffsJSON, _ := json.Marshal(diffs)

	if existing, ok := pending[prev.RemoteID]; ok {
		existing.BaseVCard = prev.BaseVCard
		existing.LocalVCard = localItem.VCardData
		existing.RemoteVCard = remoteItem.VCardData
		existing.RemoteETag = remoteItem.ETag
		existing.FieldDiffs = string(diffsJSON)
		if err := e.conflictRepo.Update(ctx, existing); err != nil {
			e.logger.Warn("failed to update conflict record", zap.Error(err))
		}
		return
	}

	conflict := &domain.SyncConflict{
		ID:             uuid.New().String(),
		UserID:         userID,
		ProviderType:   providerKey,
		RemoteID:       prev.RemoteID,
		LocalContactID: prev.LocalID,
		BaseVCard:      prev.BaseVCard,
		LocalVCard:     localItem.VCardData,
		RemoteVCard:    remoteItem.VCardData,
		RemoteETag:     remoteItem.ETag,
		FieldDiffs:     string(diffsJSON),
		Status:         "pending",
		CreatedAt:      now,
	}
	if err := e.conflictRepo.Create(ctx, conflict); err != nil {
		e.logger.Warn("failed to create conflict record", zap.Error(err))
		return
	}
	pending[prev.RemoteID] = conflict
}

// pushPhase syncs locally-changed items from local (dest) back to remote (source).
// "local" is the internal provider, "remote" is the external provider.
func (e *Engine) pushPhase(ctx context.Context, userID, providerKey string, local, remote SyncProvider, conflictMode ConflictMode, result *SyncResult) error {
	localItems, err := local.List(ctx)
	if err != nil {
		return fmt.Errorf("push: list local items: %w", err)
	}

	// The remote listing is what tells us whether someone else changed a contact since
	// our last sync. Without it a push silently overwrites their edit.
	remoteItems, err := remote.List(ctx)
	if err != nil {
		return fmt.Errorf("push: list remote items: %w", err)
	}

	prevStates, err := e.syncRepo.ListByUser(ctx, userID, providerKey)
	if err != nil {
		return fmt.Errorf("push: list sync states: %w", err)
	}

	localMap := make(map[string]SyncItem, len(localItems))
	for _, item := range localItems {
		localMap[item.RemoteID] = item
	}

	remoteMap := make(map[string]SyncItem, len(remoteItems))
	for _, item := range remoteItems {
		remoteMap[item.RemoteID] = item
	}

	prevByLocalID := make(map[string]*domain.SyncState, len(prevStates))
	for _, s := range prevStates {
		prevByLocalID[s.LocalID] = s
	}

	missing := 0
	for localID := range prevByLocalID {
		if _, exists := localMap[localID]; !exists {
			missing++
		}
	}
	if err := checkDeletionSafety(missing, len(prevByLocalID)); err != nil {
		return err
	}

	pending := e.pendingConflictsByRemoteID(ctx, userID, providerKey)
	now := time.Now()

	// Push locally-changed contacts to remote
	for localID, localItem := range localMap {
		prev, exists := prevByLocalID[localID]
		if !exists {
			// Not yet tracked — push as new. The provider may assign its own id.
			put, err := remote.Put(ctx, localItem)
			if err != nil {
				e.logger.Error("push: failed to put new item to remote", zap.String("local_id", localID), zap.Error(err))
				result.Errors++
				continue
			}
			state := &domain.SyncState{
				ID:           uuid.New().String(),
				UserID:       userID,
				ProviderType: providerKey,
				RemoteID:     put.RemoteID,
				LocalID:      localID,
				RemoteETag:   put.ETag,
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

		// Did the remote move on too? Overwriting it would destroy that edit.
		if remoteItem, ok := remoteMap[prev.RemoteID]; ok && remoteItem.ETag != prev.RemoteETag {
			result.Conflicts++
			if conflictMode != ConflictDestWins {
				e.recordConflict(ctx, userID, providerKey, now, prev, localItem, remoteItem, pending)
				result.Skipped++
				continue
			}
		}

		put, err := remote.Put(ctx, SyncItem{
			RemoteID:  prev.RemoteID,
			ETag:      prev.RemoteETag,
			VCardData: localItem.VCardData,
		})
		if err != nil {
			e.logger.Error("push: failed to put changed item to remote", zap.String("remote_id", prev.RemoteID), zap.Error(err))
			result.Errors++
			continue
		}

		prev.RemoteID = put.RemoteID
		prev.RemoteETag = put.ETag
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
	for localID, prev := range prevByLocalID {
		if _, exists := localMap[localID]; !exists {
			if err := remote.Delete(ctx, prev.RemoteID); err != nil {
				e.logger.Error("push: failed to delete from remote", zap.String("remote_id", prev.RemoteID), zap.Error(err))
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
