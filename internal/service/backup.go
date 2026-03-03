package service

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

type BackupService struct {
	contactRepo      repository.ContactRepository
	abRepo           repository.AddressBookRepository
	settingsRepo     repository.UserBackupSettingsRepository
	backupDir        string
	defaultSchedule  string
	defaultRetention int
}

func NewBackupService(
	contactRepo repository.ContactRepository,
	abRepo repository.AddressBookRepository,
	settingsRepo repository.UserBackupSettingsRepository,
	backupDir string,
	defaultSchedule string,
	defaultRetention int,
) *BackupService {
	if defaultRetention <= 0 {
		defaultRetention = 7
	}
	return &BackupService{
		contactRepo:      contactRepo,
		abRepo:           abRepo,
		settingsRepo:     settingsRepo,
		backupDir:        backupDir,
		defaultSchedule:  defaultSchedule,
		defaultRetention: defaultRetention,
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

// Create creates a new backup for the user. Compression and retention are
// applied according to the user's backup settings.
func (s *BackupService) Create(ctx context.Context, userID string) (*BackupInfo, error) {
	ab, err := s.abRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	if err := os.MkdirAll(filepath.Join(s.backupDir, userID), 0750); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
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
		return nil, err
	}

	stat, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	info := &BackupInfo{
		ID:        filename,
		Filename:  filename,
		Size:      stat.Size(),
		CreatedAt: stat.ModTime(),
	}

	// Enforce retention policy after creating the new backup.
	if settings.Retention > 0 {
		_ = s.applyRetention(ctx, userID, settings.Retention)
	}

	return info, nil
}

// writeBackupFile writes all contact vCard data to path, optionally gzip-compressed.
func (s *BackupService) writeBackupFile(path string, contacts []*domain.Contact, compress bool) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}

	var w io.Writer = f
	var gzw *gzip.Writer
	if compress {
		gzw = gzip.NewWriter(f)
		w = gzw
	}

	for _, c := range contacts {
		if _, err := io.WriteString(w, c.VCardData); err != nil {
			if gzw != nil {
				_ = gzw.Close()
			}
			_ = f.Close()
			return fmt.Errorf("write contact: %w", err)
		}
	}

	if gzw != nil {
		if err := gzw.Close(); err != nil {
			_ = f.Close()
			return fmt.Errorf("flush gzip: %w", err)
		}
	}
	return f.Close()
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
		if !strings.HasSuffix(name, ".vcf") && !strings.HasSuffix(name, ".vcf.gz") {
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

// Restore imports contacts from a backup file.
// mode "merge" adds contacts that do not already exist (by UID).
// mode "replace" deletes all current contacts and imports the entire backup.
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

	if mode == "replace" {
		if err := s.contactRepo.DeleteAll(ctx, ab.ID); err != nil {
			return nil, fmt.Errorf("delete existing contacts: %w", err)
		}
	}

	cards := vcardpkg.SplitVCards(data)
	result := &RestoreResult{}

	for _, card := range cards {
		card = strings.TrimSpace(card)
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

		if mode == "merge" {
			existing, err := s.contactRepo.GetByUID(ctx, ab.ID, uid)
			if err != nil {
				result.Errors++
				continue
			}
			if existing != nil {
				result.Skipped++
				continue
			}
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

		if err := s.contactRepo.Create(ctx, contact); err != nil {
			result.Errors++
			continue
		}
		if err := writeChildRecords(ctx, s.contactRepo, contact.ID, parsed); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	return result, nil
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

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read backup data: %w", err)
	}
	return string(data), nil
}

// applyRetention deletes the oldest backups, keeping only maxCount files.
func (s *BackupService) applyRetention(ctx context.Context, userID string, maxCount int) error {
	backups, err := s.List(ctx, userID)
	if err != nil || len(backups) <= maxCount {
		return err
	}
	// List is sorted newest-first; delete from the tail.
	for _, b := range backups[maxCount:] {
		path := filepath.Join(s.backupDir, userID, b.ID)
		_ = os.Remove(path)
	}
	return nil
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
