package repository

import (
	"context"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit, offset int) ([]*domain.User, int, error)
	ListAllIDs(ctx context.Context) ([]string, error)
}

type AddressBookRepository interface {
	Create(ctx context.Context, ab *domain.AddressBook) error
	GetByID(ctx context.Context, id string) (*domain.AddressBook, error)
	GetByUserID(ctx context.Context, userID string) (*domain.AddressBook, error)
	// GetOrCreateByUserID returns the address book for userID, creating it if it doesn't exist.
	GetOrCreateByUserID(ctx context.Context, userID string) (*domain.AddressBook, error)
	// ChangeSeq is the collection's CTag: it advances on every write to its contacts.
	ChangeSeq(ctx context.Context, addressBookID string) (int64, error)
	Update(ctx context.Context, ab *domain.AddressBook) error
	Delete(ctx context.Context, id string) error
}

type ContactRepository interface {
	Create(ctx context.Context, contact *domain.Contact) error
	GetByID(ctx context.Context, id string) (*domain.Contact, error)
	GetByUID(ctx context.Context, addressBookID, uid string) (*domain.Contact, error)
	Update(ctx context.Context, contact *domain.Contact) error
	Delete(ctx context.Context, id string) error
	DeleteAll(ctx context.Context, addressBookID string) error
	List(ctx context.Context, addressBookID string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error)
	Search(ctx context.Context, addressBookID, query string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error)
	ListAll(ctx context.Context, addressBookID string) ([]*domain.Contact, error)
	// ListForDedup returns only the columns duplicate detection reads.
	ListForDedup(ctx context.Context, addressBookID string) ([]*domain.Contact, error)
	// ListDedupValues returns the secondary emails and phone numbers of an address book as
	// two narrow (contact_id, value) projections. Detection buckets on these as well as on
	// the flat columns ListForDedup returns; a contact's second phone number identifies them
	// exactly as well as their first.
	ListDedupValues(ctx context.Context, addressBookID string) (emails, phones []domain.ContactValueRef, err error)

	// Child-record management (delete-then-insert in a transaction)
	ReplaceEmails(ctx context.Context, contactID string, rows []*domain.ContactEmail) error
	ReplacePhones(ctx context.Context, contactID string, rows []*domain.ContactPhone) error
	ReplaceAddresses(ctx context.Context, contactID string, rows []*domain.ContactAddress) error
	ReplaceURLs(ctx context.Context, contactID string, rows []*domain.ContactURL) error
	ReplaceIMs(ctx context.Context, contactID string, rows []*domain.ContactIM) error
	ReplaceCategories(ctx context.Context, contactID string, rows []*domain.ContactCategory) error
	ReplaceDates(ctx context.Context, contactID string, rows []*domain.ContactDate) error

	// Versions that also load child records
	DeleteMany(ctx context.Context, addressBookID string, ids []string) (int, error)

	// ChangesSince powers RFC 6578 collection synchronisation.
	ChangesSince(ctx context.Context, addressBookID string, sinceSeq int64) (*CollectionChanges, error)
	ListByIDs(ctx context.Context, addressBookID string, ids []string) ([]*domain.Contact, error)

	// Save writes a contact and all of its child rows atomically.
	Save(ctx context.Context, contact *domain.Contact, children domain.ChildRecords) error
	// MergeInto saves the surviving contact and deletes the merged-away one in one
	// transaction, recording the deletion in the change journal.
	MergeInto(ctx context.Context, winner *domain.Contact, children domain.ChildRecords, loserID string) error
	GetByIDWithRelations(ctx context.Context, id string) (*domain.Contact, error)
	GetByUIDWithRelations(ctx context.Context, addressBookID, uid string) (*domain.Contact, error)
	ListWithRelations(ctx context.Context, addressBookID string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error)
	SearchWithRelations(ctx context.Context, addressBookID, query string, limit, offset int, filters ListFilters) ([]*domain.Contact, int, error)
	Facets(ctx context.Context, addressBookID string) (*ContactFacets, error)
}

type UserBackupSettingsRepository interface {
	Get(ctx context.Context, userID string) (*domain.UserBackupSettings, error)
	Upsert(ctx context.Context, s *domain.UserBackupSettings) error
	ListAll(ctx context.Context) ([]*domain.UserBackupSettings, error)
}

type ProviderConnectionRepository interface {
	Create(ctx context.Context, c *domain.ProviderConnection) error
	GetByID(ctx context.Context, id string) (*domain.ProviderConnection, error)
	ListByUser(ctx context.Context, userID string) ([]*domain.ProviderConnection, error)
	GetByUserAndType(ctx context.Context, userID, providerType string) (*domain.ProviderConnection, error)
	Update(ctx context.Context, c *domain.ProviderConnection) error
	Delete(ctx context.Context, id string) error
	UpdateToken(ctx context.Context, id, accessToken, refreshToken string, expiry *time.Time) error
	SetConnected(ctx context.Context, id string, connected bool) error
}

type SyncStateRepository interface {
	Create(ctx context.Context, state *domain.SyncState) error
	GetByRemoteID(ctx context.Context, userID, providerType, remoteID string) (*domain.SyncState, error)
	GetByLocalID(ctx context.Context, userID, providerType, localID string) (*domain.SyncState, error)
	ListByUser(ctx context.Context, userID, providerType string) ([]*domain.SyncState, error)
	ListAllByUser(ctx context.Context, userID string) ([]*domain.SyncState, error)
	Update(ctx context.Context, state *domain.SyncState) error
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID, providerType string) error
}

type PipelineRepository interface {
	Create(ctx context.Context, pipeline *domain.Pipeline) error
	GetByID(ctx context.Context, id string) (*domain.Pipeline, error)
	ListByUser(ctx context.Context, userID string) ([]*domain.Pipeline, error)
	ListAllEnabled(ctx context.Context) ([]*domain.Pipeline, error)
	Update(ctx context.Context, pipeline *domain.Pipeline) error
	Delete(ctx context.Context, id string) error
	CreateStep(ctx context.Context, step *domain.PipelineStep) error
	GetSteps(ctx context.Context, pipelineID string) ([]*domain.PipelineStep, error)
	DeleteSteps(ctx context.Context, pipelineID string) error
}

type SyncRunRepository interface {
	Create(ctx context.Context, run *domain.SyncRun) error
	Update(ctx context.Context, run *domain.SyncRun) error
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.SyncRun, error)
	ListActiveByUser(ctx context.Context, userID string) ([]*domain.SyncRun, error)
	ListByPipeline(ctx context.Context, userID, pipelineID string, limit int) ([]*domain.SyncRun, error)
	// MarkStaleInterrupted closes runs orphaned by a process that died.
	MarkStaleInterrupted(ctx context.Context, startedBefore time.Time) (int, error)
	// DeleteOlderThan prunes history; sync_runs grows with every pipeline execution.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

// BackupRunRepository records what happened to each backup attempt. The files on disk cannot
// answer that: retention deletes them.
type BackupRunRepository interface {
	Create(ctx context.Context, run *domain.BackupRun) error
	Update(ctx context.Context, run *domain.BackupRun) error
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.BackupRun, error)
	LastSuccess(ctx context.Context, userID string) (*domain.BackupRun, error)
	LastRun(ctx context.Context, userID string) (*domain.BackupRun, error)
	// MarkStaleInterrupted closes runs left open by a process that died, bounded to those
	// started before the given moment so a second instance's live runs are untouched.
	MarkStaleInterrupted(ctx context.Context, startedBefore time.Time) (int, error)
}

// MergeLogRepository stores what a merge did, outliving the contacts it touched.
type MergeLogRepository interface {
	Create(ctx context.Context, entry *domain.MergeLogEntry) error
	ListByUser(ctx context.Context, userID string, limit int) ([]*domain.MergeLogEntry, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

type PotentialDuplicateRepository interface {
	Create(ctx context.Context, d *domain.PotentialDuplicate) error
	// CreateIfAbsent inserts unless the pair is already recorded; the bool reports insertion.
	CreateIfAbsent(ctx context.Context, d *domain.PotentialDuplicate) (bool, error)
	GetByID(ctx context.Context, id string) (*domain.PotentialDuplicate, error)
	// GetByIDWithContacts filters on ownership inside the query and loads both contacts
	// with all of their child collections.
	GetByIDWithContacts(ctx context.Context, userID, id string) (*domain.PotentialDuplicate, error)
	ListByUser(ctx context.Context, userID, status string, limit, offset int) ([]*domain.PotentialDuplicate, int, error)
	GetByContacts(ctx context.Context, userID, aID, bID string) (*domain.PotentialDuplicate, error)
	Update(ctx context.Context, d *domain.PotentialDuplicate) error
	DeleteByContact(ctx context.Context, contactID string) error
	CountPending(ctx context.Context, userID string) (int, error)
}

type UserDedupSettingsRepository interface {
	Get(ctx context.Context, userID string) (*domain.UserDedupSettings, error)
	Upsert(ctx context.Context, s *domain.UserDedupSettings) error
	ListAll(ctx context.Context) ([]*domain.UserDedupSettings, error)
}

type AppPasswordRepository interface {
	Create(ctx context.Context, ap *domain.AppPassword) error
	ListByUser(ctx context.Context, userID string) ([]domain.AppPassword, error)
	GetByID(ctx context.Context, id string) (*domain.AppPassword, error)
	Delete(ctx context.Context, id string) error
	ListAllByUser(ctx context.Context, userID string) ([]domain.AppPassword, error)
	UpdateLastUsed(ctx context.Context, id string) error
}

type SyncConflictRepository interface {
	Create(ctx context.Context, c *domain.SyncConflict) error
	GetByID(ctx context.Context, id string) (*domain.SyncConflict, error)
	ListByUser(ctx context.Context, userID, status string, limit, offset int) ([]*domain.SyncConflict, int, error)
	ListPendingByProvider(ctx context.Context, userID, providerType string) ([]*domain.SyncConflict, error)
	Update(ctx context.Context, c *domain.SyncConflict) error
	DeleteByProvider(ctx context.Context, userID, providerType string) error
	CountPending(ctx context.Context, userID string) (int, error)
}
