package service_test

import (
	"context"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// --- mock ContactRepository ---

type mockContactRepo struct {
	contacts map[string]*domain.Contact
	byUID    map[string]*domain.Contact // key: addressBookID+":"+uid
}

func newMockContactRepo() *mockContactRepo {
	return &mockContactRepo{
		contacts: make(map[string]*domain.Contact),
		byUID:    make(map[string]*domain.Contact),
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

func (m *mockContactRepo) DeleteAll(_ context.Context, abID string) error {
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

func (m *mockContactRepo) ListAll(_ context.Context, abID string) ([]*domain.Contact, error) {
	var out []*domain.Contact
	for _, c := range m.contacts {
		if c.AddressBookID == abID {
			out = append(out, c)
		}
	}
	return out, nil
}

// Child record methods — no-op in tests.
func (m *mockContactRepo) ReplaceEmails(_ context.Context, _ string, _ []*domain.ContactEmail) error {
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

func (m *mockAbRepo) Update(_ context.Context, ab *domain.AddressBook) error { return nil }

func (m *mockAbRepo) Delete(_ context.Context, _ string) error { return nil }
