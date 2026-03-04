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

	// Child-record management (delete-then-insert in a transaction)
	ReplaceEmails(ctx context.Context, contactID string, rows []*domain.ContactEmail) error
	ReplacePhones(ctx context.Context, contactID string, rows []*domain.ContactPhone) error
	ReplaceAddresses(ctx context.Context, contactID string, rows []*domain.ContactAddress) error
	ReplaceURLs(ctx context.Context, contactID string, rows []*domain.ContactURL) error
	ReplaceIMs(ctx context.Context, contactID string, rows []*domain.ContactIM) error
	ReplaceCategories(ctx context.Context, contactID string, rows []*domain.ContactCategory) error
	ReplaceDates(ctx context.Context, contactID string, rows []*domain.ContactDate) error

	// Versions that also load child records
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
}

type PotentialDuplicateRepository interface {
	Create(ctx context.Context, d *domain.PotentialDuplicate) error
	GetByID(ctx context.Context, id string) (*domain.PotentialDuplicate, error)
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
