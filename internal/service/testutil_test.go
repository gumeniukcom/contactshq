package service_test

import (
	"context"
	"sort"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// --- mock ContactRepository ---

type mockContactRepo struct {
	contacts map[string]*domain.Contact
	byUID    map[string]*domain.Contact // key: addressBookID+":"+uid
	updates  int                        // Update() call count
	saves    int                        // Save() call count
	// deleteAllCalls proves a replace-restore did not reach the destructive step when the
	// backup was rejected.
	deleteAllCalls int
	// emailsWritten records the last child-row payload per contact, so tests can tell a
	// persisted write apart from a mutation of the pointer they already hold.
	emailsWritten map[string][]*domain.ContactEmail
	// beforeListForDedup lets a test hold a scan open long enough to start a second one.
	beforeListForDedup func()
}

// Save mirrors the repository: contact row and child rows land together.
func (m *mockContactRepo) Save(_ context.Context, c *domain.Contact, children domain.ChildRecords) error {
	m.saves++
	m.contacts[c.ID] = c
	m.byUID[c.AddressBookID+":"+c.UID] = c
	m.emailsWritten[c.ID] = children.Emails
	return nil
}

// MergeInto mirrors the repository: the winner is saved and the loser removed together.
func (m *mockContactRepo) MergeInto(ctx context.Context, winner *domain.Contact, children domain.ChildRecords, loserID string) error {
	if err := m.Save(ctx, winner, children); err != nil {
		return err
	}
	return m.Delete(ctx, loserID)
}

func newMockContactRepo() *mockContactRepo {
	return &mockContactRepo{
		contacts:      make(map[string]*domain.Contact),
		byUID:         make(map[string]*domain.Contact),
		emailsWritten: make(map[string][]*domain.ContactEmail),
	}
}

func (m *mockContactRepo) Create(_ context.Context, c *domain.Contact) error {
	m.contacts[c.ID] = c
	m.byUID[c.AddressBookID+":"+c.UID] = c
	return nil
}

func (m *mockContactRepo) GetByID(_ context.Context, id string) (*domain.Contact, error) {
	return m.contacts[id], nil
}

func (m *mockContactRepo) GetByUID(_ context.Context, abID, uid string) (*domain.Contact, error) {
	return m.byUID[abID+":"+uid], nil
}

func (m *mockContactRepo) Update(_ context.Context, c *domain.Contact) error {
	m.updates++
	m.contacts[c.ID] = c
	m.byUID[c.AddressBookID+":"+c.UID] = c
	return nil
}

func (m *mockContactRepo) Delete(_ context.Context, id string) error {
	if c, ok := m.contacts[id]; ok {
		delete(m.byUID, c.AddressBookID+":"+c.UID)
		delete(m.contacts, id)
	}
	return nil
}

func (m *mockContactRepo) DeleteMany(_ context.Context, abID string, ids []string) (int, error) {
	deleted := 0
	for _, id := range ids {
		c, ok := m.contacts[id]
		if !ok || c.AddressBookID != abID {
			continue
		}
		delete(m.byUID, c.AddressBookID+":"+c.UID)
		delete(m.contacts, id)
		deleted++
	}
	return deleted, nil
}

func (m *mockContactRepo) ListByIDs(_ context.Context, abID string, ids []string) ([]*domain.Contact, error) {
	var out []*domain.Contact
	for _, id := range ids {
		if c, ok := m.contacts[id]; ok && c.AddressBookID == abID {
			out = append(out, c)
		}
	}
	return sortByName(out), nil
}

// sortByName mirrors the repository's ORDER BY. Map iteration would otherwise make every
// listing non-deterministic, and tests would pass or fail at random.
func sortByName(contacts []*domain.Contact) []*domain.Contact {
	sort.SliceStable(contacts, func(i, j int) bool {
		if contacts[i].LastName != contacts[j].LastName {
			return contacts[i].LastName < contacts[j].LastName
		}
		return contacts[i].FirstName < contacts[j].FirstName
	})
	return contacts
}

func (m *mockContactRepo) DeleteAll(_ context.Context, abID string) error {
	m.deleteAllCalls++
	for id, c := range m.contacts {
		if c.AddressBookID == abID {
			delete(m.byUID, c.AddressBookID+":"+c.UID)
			delete(m.contacts, id)
		}
	}
	return nil
}

func (m *mockContactRepo) List(_ context.Context, _ string, _, _ int, _ repository.ListFilters) ([]*domain.Contact, int, error) {
	return nil, 0, nil
}

func (m *mockContactRepo) Search(_ context.Context, _, _ string, _, _ int, _ repository.ListFilters) ([]*domain.Contact, int, error) {
	return nil, 0, nil
}

func (m *mockContactRepo) ListForDedup(ctx context.Context, abID string) ([]*domain.Contact, error) {
	if m.beforeListForDedup != nil {
		m.beforeListForDedup()
	}
	return m.ListAll(ctx, abID)
}

func (m *mockContactRepo) ListAll(_ context.Context, abID string) ([]*domain.Contact, error) {
	var out []*domain.Contact
	for _, c := range m.contacts {
		if c.AddressBookID == abID {
			out = append(out, c)
		}
	}
	return sortByName(out), nil
}

// Child record methods — no-op in tests.
func (m *mockContactRepo) ReplaceEmails(_ context.Context, contactID string, emails []*domain.ContactEmail) error {
	m.emailsWritten[contactID] = emails
	return nil
}
func (m *mockContactRepo) ReplacePhones(_ context.Context, _ string, _ []*domain.ContactPhone) error {
	return nil
}
func (m *mockContactRepo) ReplaceAddresses(_ context.Context, _ string, _ []*domain.ContactAddress) error {
	return nil
}
func (m *mockContactRepo) ReplaceURLs(_ context.Context, _ string, _ []*domain.ContactURL) error {
	return nil
}
func (m *mockContactRepo) ReplaceIMs(_ context.Context, _ string, _ []*domain.ContactIM) error {
	return nil
}
func (m *mockContactRepo) ReplaceCategories(_ context.Context, _ string, _ []*domain.ContactCategory) error {
	return nil
}
func (m *mockContactRepo) ReplaceDates(_ context.Context, _ string, _ []*domain.ContactDate) error {
	return nil
}
func (m *mockContactRepo) GetByIDWithRelations(_ context.Context, id string) (*domain.Contact, error) {
	return m.contacts[id], nil
}
func (m *mockContactRepo) GetByUIDWithRelations(_ context.Context, abID, uid string) (*domain.Contact, error) {
	return m.byUID[abID+":"+uid], nil
}
func (m *mockContactRepo) ListWithRelations(_ context.Context, _ string, _, _ int, _ repository.ListFilters) ([]*domain.Contact, int, error) {
	return nil, 0, nil
}
func (m *mockContactRepo) SearchWithRelations(_ context.Context, _, _ string, _, _ int, _ repository.ListFilters) ([]*domain.Contact, int, error) {
	return nil, 0, nil
}
func (m *mockContactRepo) ChangesSince(_ context.Context, _ string, _ int64) (*repository.CollectionChanges, error) {
	return &repository.CollectionChanges{}, nil
}
func (m *mockContactRepo) OldestTombstoneSeq(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockContactRepo) Facets(_ context.Context, _ string) (*repository.ContactFacets, error) {
	return &repository.ContactFacets{Categories: []string{}, Orgs: []string{}}, nil
}

// --- mock AddressBookRepository ---

type mockAbRepo struct {
	ab *domain.AddressBook
}

func (m *mockAbRepo) Create(_ context.Context, ab *domain.AddressBook) error { return nil }

func (m *mockAbRepo) GetByID(_ context.Context, _ string) (*domain.AddressBook, error) {
	return m.ab, nil
}

func (m *mockAbRepo) GetByUserID(_ context.Context, _ string) (*domain.AddressBook, error) {
	return m.ab, nil
}

func (m *mockAbRepo) GetOrCreateByUserID(_ context.Context, _ string) (*domain.AddressBook, error) {
	return m.ab, nil
}

func (m *mockAbRepo) ChangeSeq(_ context.Context, _ string) (int64, error) { return 0, nil }

func (m *mockAbRepo) Update(_ context.Context, ab *domain.AddressBook) error { return nil }

func (m *mockAbRepo) Delete(_ context.Context, _ string) error { return nil }
