package service

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

// DefaultMaxRestoreBytes caps the decompressed size of a restored backup. A gzip stream can
// expand by orders of magnitude, and restore reads the whole thing into memory.
const DefaultMaxRestoreBytes int64 = 128 << 20

// ErrBackupTooLarge reports a backup whose decompressed size exceeds the configured cap.
var ErrBackupTooLarge = errors.New("backup exceeds the maximum restore size")

type BackupService struct {
	contactRepo      repository.ContactRepository
	abRepo           repository.AddressBookRepository
	settingsRepo     repository.UserBackupSettingsRepository
	syncStateRepo    repository.SyncStateRepository // optional — may be nil
	runRepo          repository.BackupRunRepository // optional — may be nil
	logger           *zap.Logger
	backupDir        string
	defaultSchedule  string
	defaultRetention int
	maxRestoreBytes  int64
}

// WithSyncStateRepo lets a restore reconcile the sync state it invalidates.
func (s *BackupService) WithSyncStateRepo(repo repository.SyncStateRepository) *BackupService {
	s.syncStateRepo = repo
	return s
}

// WithRunRepo records every backup attempt. Optional: without it the service behaves exactly
// as it did before the history existed.
func (s *BackupService) WithRunRepo(repo repository.BackupRunRepository) *BackupService {
	s.runRepo = repo
	return s
}

// WithMaxRestoreBytes overrides the decompressed-size cap applied on restore.
func (s *BackupService) WithMaxRestoreBytes(max int64) *BackupService {
	if max > 0 {
		s.maxRestoreBytes = max
	}
	return s
}

func NewBackupService(
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	settingsRepo repository.UserBackupSettingsRepository,
	logger *zap.Logger,
	backupDir string,
	defaultSchedule string,
	defaultRetention int,
) *BackupService {
	if defaultRetention <= 0 {
		defaultRetention = 7
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BackupService{
		contactRepo:      contactRepo,
		abRepo:           abRepo,
		settingsRepo:     settingsRepo,
		logger:           logger,
		backupDir:        backupDir,
		defaultSchedule:  defaultSchedule,
		defaultRetention: defaultRetention,
		maxRestoreBytes:  DefaultMaxRestoreBytes,
	}
}

// BackupInfo describes a single backup file.
type BackupInfo struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// RestoreResult summarises a restore operation.
type RestoreResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// Create creates a new backup for the user, recorded as a manual run.
func (s *BackupService) Create(ctx context.Context, userID string) (*BackupInfo, error) {
	return s.CreateWithTrigger(ctx, userID, domain.BackupTriggerManual)
}

// CreateWithTrigger creates a backup and records the attempt, whatever its outcome.
//
// The record is written here rather than in the scheduled job because the manual
// POST /backup/create runs synchronously in the HTTP handler and would otherwise never reach
// the history — which would make "last successful backup" systematically wrong for anyone who
// backs up by hand.
func (s *BackupService) CreateWithTrigger(ctx context.Context, userID, trigger string) (*BackupInfo, error) {
	run := s.startRun(ctx, userID, trigger)

	info, contactCount, err := s.createBackup(ctx, userID)
	s.finishRun(run, info, contactCount, err)

	return info, err
}

// startRun opens a history row. A failure to record is logged, not fatal: the backup itself
// is what the user asked for.
func (s *BackupService) startRun(ctx context.Context, userID, trigger string) *domain.BackupRun {
	if s.runRepo == nil {
		return nil
	}
	run := &domain.BackupRun{
		ID:        uuid.New().String(),
		UserID:    userID,
		Trigger:   trigger,
		Status:    domain.BackupRunRunning,
		StartedAt: time.Now(),
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		s.logger.Warn("failed to open a backup run record",
			zap.String("user_id", userID), zap.Error(err))
		return nil
	}
	return run
}

// finishRun closes the history row on a context of its own.
//
// Deliberately not the caller's context: during a graceful shutdown that one is already
// cancelled by the time the backup returns, and the update would fail — leaving the row
// "running" forever, which is the exact failure this table exists to make visible.
func (s *BackupService) finishRun(run *domain.BackupRun, info *BackupInfo, contactCount int, cause error) {
	if run == nil || s.runRepo == nil {
		return
	}

	finished := time.Now()
	run.FinishedAt = &finished
	if cause != nil {
		run.Status = domain.BackupRunFailed
		run.ErrorMessage = cause.Error()
	} else {
		run.Status = domain.BackupRunOK
		run.ContactCount = contactCount
		if info != nil {
			run.Filename = info.Filename
			run.SizeBytes = info.Size
			run.Compressed = strings.HasSuffix(info.Filename, ".gz")
		}
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 5*time.Second)
	defer cancel()

	if err := s.runRepo.Update(ctx, run); err != nil {
		s.logger.Warn("failed to close a backup run record",
			zap.String("user_id", run.UserID), zap.Error(err))
	}
}

func (s *BackupService) createBackup(ctx context.Context, userID string) (*BackupInfo, int, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	settings, err := s.GetSettings(ctx, userID)
	if err != nil {
		// Fall back to defaults on error — backup should still proceed.
		settings = &domain.UserBackupSettings{
			Retention: s.defaultRetention,
			Compress:  false,
		}
	}

	contacts, err := s.contactRepo.ListAll(ctx, ab.ID)
	if err != nil {
		return nil, 0, err
	}

	if err := os.MkdirAll(filepath.Join(s.backupDir, userID), 0750); err != nil {
		return nil, 0, fmt.Errorf("create backup dir: %w", err)
	}

	// Use millisecond precision to prevent filename collisions.
	timestamp := time.Now().Format("20060102-150405-000")
	var filename string
	if settings.Compress {
		filename = fmt.Sprintf("backup-%s.vcf.gz", timestamp)
	} else {
		filename = fmt.Sprintf("backup-%s.vcf", timestamp)
	}
	fullPath := filepath.Join(s.backupDir, userID, filename)

	if err := s.writeBackupFile(fullPath, contacts, settings.Compress); err != nil {
		return nil, 0, err
	}

	stat, err := os.Stat(fullPath)
	if err != nil {
		return nil, 0, err
	}

	info := &BackupInfo{
		ID:        filename,
		Filename:  filename,
		Size:      stat.Size(),
		CreatedAt: stat.ModTime(),
	}

	// Enforce retention policy after creating the new backup. A failure here does not
	// invalidate the backup just written, so it is logged rather than returned — but it must
	// not be silent, or the directory grows without bound with nobody the wiser.
	if settings.Retention > 0 {
		if err := s.applyRetention(ctx, userID, settings.Retention); err != nil {
			s.logger.Warn("backup retention failed",
				zap.String("user_id", userID),
				zap.Int("retention", settings.Retention),
				zap.Error(err))
		}
	}

	return info, len(contacts), nil
}

// writeBackupFile writes all contact vCard data to path, optionally gzip-compressed.
//
// The file is built under a temporary name and renamed into place only once it is complete,
// because List and GetPath work off the directory listing: a half-written backup used to be
// visible immediately, counted towards retention, and restorable — silently truncated.
func (s *BackupService) writeBackupFile(path string, contacts []*domain.Contact, compress bool) (err error) {
	tmpPath := path + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}

	// Any failure past this point must leave nothing behind: a stray .tmp would otherwise
	// accumulate in the user's directory forever.
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	var w io.Writer = f
	var gzw *gzip.Writer
	if compress {
		gzw = gzip.NewWriter(f)
		w = gzw
	}

	for _, c := range contacts {
		// See ExportVCardByIDs: a card stored without its trailing CRLF would be glued to the
		// next one, and the restore that reads this file back would keep only the first.
		if _, werr := io.WriteString(w, vcardpkg.Terminated(c.VCardData)); werr != nil {
			if gzw != nil {
				_ = gzw.Close()
			}
			err = fmt.Errorf("write contact: %w", werr)
			return err
		}
	}

	if gzw != nil {
		if cerr := gzw.Close(); cerr != nil {
			err = fmt.Errorf("flush gzip: %w", cerr)
			return err
		}
	}

	if cerr := f.Close(); cerr != nil {
		err = fmt.Errorf("close backup file: %w", cerr)
		return err
	}

	if rerr := os.Rename(tmpPath, path); rerr != nil {
		err = fmt.Errorf("publish backup file: %w", rerr)
		return err
	}
	return nil
}

// List returns all backup files for the user, sorted newest first.
func (s *BackupService) List(ctx context.Context, userID string) ([]BackupInfo, error) {
	dir := filepath.Join(s.backupDir, userID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isBackupFilename(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			ID:        name,
			Filename:  name,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	return backups, nil
}

// GetPath returns the absolute path of a backup file after validating it belongs to the user.
func (s *BackupService) GetPath(ctx context.Context, userID, backupID string) (string, error) {
	// List already filters on this suffix; GetPath did not, so anything else that happened to
	// sit in the user's directory — a stray .tmp, a note someone dropped there — was
	// downloadable and deletable through the backup API.
	if !isBackupFilename(backupID) {
		return "", fmt.Errorf("invalid backup path")
	}

	fullPath := filepath.Join(s.backupDir, userID, backupID)

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(filepath.Join(s.backupDir, userID))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", fmt.Errorf("invalid backup path")
	}

	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("backup not found")
	}
	return fullPath, nil
}

// Delete removes a backup file.
func (s *BackupService) Delete(ctx context.Context, userID, backupID string) error {
	fullPath, err := s.GetPath(ctx, userID, backupID)
	if err != nil {
		return err
	}
	return os.Remove(fullPath)
}

// ErrEmptyBackup guards a replace-restore from wiping an address book with nothing.
var ErrEmptyBackup = errors.New("backup contains no readable contacts")

// preparedContact is a contact parsed out of a backup and ready to insert.
type preparedContact struct {
	contact *domain.Contact
	parsed  *vcardpkg.ParsedContact
}

// Restore imports contacts from a backup file.
// mode "merge" adds contacts that do not already exist (by UID).
// mode "replace" deletes all current contacts and imports the entire backup.
//
// The whole backup is parsed before anything is deleted. Doing it the other way round
// turns an unreadable or empty file into the permanent loss of the address book, since
// DeleteAll has already run by the time the first card fails to parse.
func (s *BackupService) Restore(ctx context.Context, userID, backupID, mode string) (*RestoreResult, error) {
	fullPath, err := s.GetPath(ctx, userID, backupID)
	if err != nil {
		return nil, err
	}

	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	data, err := s.readBackupFile(fullPath)
	if err != nil {
		return nil, err
	}

	result := &RestoreResult{}
	prepared := make([]preparedContact, 0)

	for _, card := range vcardpkg.SplitVCards(data) {
		card = vcardpkg.Terminated(card)
		if card == "" {
			continue
		}

		parsed, err := vcardpkg.ParseVCard(card)
		if err != nil {
			result.Errors++
			continue
		}

		uid := parsed.UID
		if uid == "" {
			uid = uuid.New().String()
			card = vcardpkg.InjectUID(card, uid)
			parsed.UID = uid
		}

		now := time.Now()
		contact := &domain.Contact{
			ID:            uuid.New().String(),
			AddressBookID: ab.ID,
			UID:           uid,
			ETag:          generateETag(card),
			VCardData:     card,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		vcardpkg.ApplyToContact(contact, parsed)

		// Cancellation is honoured during parsing only. Past this point the restore starts
		// deleting and inserting, and a cancellation between DeleteAll and the inserts would
		// leave an empty address book — "more cancellable" would become a way to lose every
		// contact. See the loop below, which deliberately has no such check.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		prepared = append(prepared, preparedContact{contact: contact, parsed: parsed})
	}

	if mode == "replace" {
		if len(prepared) == 0 {
			return nil, fmt.Errorf("%w: refusing to replace %d existing contacts", ErrEmptyBackup, result.Errors)
		}
		if err := s.contactRepo.DeleteAll(ctx, ab.ID); err != nil {
			return nil, fmt.Errorf("delete existing contacts: %w", err)
		}
	}

	// No ctx.Err() in this loop, on purpose: DeleteAll has already run in replace mode, so
	// bailing out here is how a restore turns into data loss.
	for _, p := range prepared {
		if mode == "merge" {
			existing, err := s.contactRepo.GetByUID(ctx, ab.ID, p.contact.UID)
			if err != nil {
				result.Errors++
				continue
			}
			if existing != nil {
				result.Skipped++
				continue
			}
		}

		if err := s.contactRepo.Save(ctx, p.contact, vcardpkg.ChildRecordsFor(p.contact.ID, p.parsed)); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if err := s.reconcileSyncState(ctx, userID, ab.ID); err != nil {
		return nil, fmt.Errorf("reconcile sync state: %w", err)
	}

	return result, nil
}

// reconcileSyncState drops the sync state of contacts a restore did not bring back.
//
// Those rows still map a remote contact to a local one that no longer exists. The next
// export or two-way run reads them as "deleted locally" and deletes the contact on the
// remote — a restore would quietly destroy data on Google or a CardDAV server. Dropping
// the row instead means the next import simply pulls the contact back.
//
// Rows whose contact survived are left alone: their local ETag no longer matches, so the
// restored content is pushed outward as an ordinary edit.
func (s *BackupService) reconcileSyncState(ctx context.Context, userID, addressBookID string) error {
	if s.syncStateRepo == nil {
		return nil
	}

	states, err := s.syncStateRepo.ListAllByUser(ctx, userID)
	if err != nil {
		return err
	}

	for _, state := range states {
		if state.LocalID == "" {
			continue
		}
		contact, err := s.contactRepo.GetByUID(ctx, addressBookID, state.LocalID)
		if err != nil {
			return err
		}
		if contact != nil {
			continue
		}
		if err := s.syncStateRepo.Delete(ctx, state.ID); err != nil {
			return err
		}
	}

	return nil
}

// isBackupFilename reports whether a name is one this service would have written.
func isBackupFilename(name string) bool {
	return strings.HasSuffix(name, ".vcf") || strings.HasSuffix(name, ".vcf.gz")
}

// readBackupFile reads a backup file and decompresses it if it is gzip-encoded.
func (s *BackupService) readBackupFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open backup: %w", err)
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return "", fmt.Errorf("open gzip reader: %w", err)
		}
		defer gzr.Close()
		r = gzr
	}

	max := s.maxRestoreBytes
	if max <= 0 {
		max = DefaultMaxRestoreBytes
	}

	// Read one byte past the limit so an oversized backup is an error rather than a silent
	// truncation: a replace-restore deletes the address book first, so a quietly truncated
	// read would destroy contacts that the backup could no longer supply.
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return "", fmt.Errorf("read backup data: %w", err)
	}
	if int64(len(data)) > max {
		return "", fmt.Errorf("%w: over %d bytes decompressed", ErrBackupTooLarge, max)
	}
	return string(data), nil
}

// applyRetention deletes the oldest backups, keeping only maxCount files.
func (s *BackupService) applyRetention(ctx context.Context, userID string, maxCount int) error {
	backups, err := s.List(ctx, userID)
	if err != nil || len(backups) <= maxCount {
		return err
	}
	// List is sorted newest-first; delete from the tail. Collect the failures instead of
	// dropping them: a directory that cannot be pruned is exactly the condition an operator
	// needs to hear about, and it is invisible from the outside.
	var errs []error
	for _, b := range backups[maxCount:] {
		path := filepath.Join(s.backupDir, userID, b.ID)
		if rerr := os.Remove(path); rerr != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", b.ID, rerr))
		}
	}
	return errors.Join(errs...)
}

// GetSettings returns the backup settings for a user, falling back to defaults
// if no user-specific settings have been saved yet.
func (s *BackupService) GetSettings(ctx context.Context, userID string) (*domain.UserBackupSettings, error) {
	settings, err := s.settingsRepo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return &domain.UserBackupSettings{
			UserID:    userID,
			Schedule:  s.defaultSchedule,
			Retention: s.defaultRetention,
			Enabled:   s.defaultSchedule != "",
			Compress:  false,
		}, nil
	}
	return settings, nil
}

// SaveSettings persists the user's backup settings.
func (s *BackupService) SaveSettings(ctx context.Context, userID string, settings *domain.UserBackupSettings) error {
	settings.UserID = userID
	settings.UpdatedAt = time.Now()
	return s.settingsRepo.Upsert(ctx, settings)
}

// GetUserSchedule returns the effective cron schedule for the user (empty string = disabled).
// Used by the scheduler at startup to register per-user backup jobs.
func (s *BackupService) GetUserSchedule(ctx context.Context, userID string) (string, error) {
	settings, err := s.GetSettings(ctx, userID)
	if err != nil {
		return "", err
	}
	if !settings.Enabled {
		return "", nil
	}
	return settings.Schedule, nil
}
