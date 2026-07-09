package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// --- mocks ---

type mockConflictRepo struct {
	byID map[string]*domain.SyncConflict
}

func (m *mockConflictRepo) Create(_ context.Context, c *domain.SyncConflict) error {
	m.byID[c.ID] = c
	return nil
}
func (m *mockConflictRepo) GetByID(_ context.Context, id string) (*domain.SyncConflict, error) {
	c, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}
func (m *mockConflictRepo) ListByUser(_ context.Context, _, _ string, _, _ int) ([]*domain.SyncConflict, int, error) {
	return nil, 0, nil
}
func (m *mockConflictRepo) ListPendingByProvider(_ context.Context, _, _ string) ([]*domain.SyncConflict, error) {
	return nil, nil
}
func (m *mockConflictRepo) Update(_ context.Context, c *domain.SyncConflict) error {
	m.byID[c.ID] = c
	return nil
}
func (m *mockConflictRepo) DeleteByProvider(_ context.Context, _, _ string) error { return nil }
func (m *mockConflictRepo) CountPending(_ context.Context, _ string) (int, error) { return 0, nil }

type mockStateRepo struct {
	states map[string]*domain.SyncState // keyed by remoteID
}

func (m *mockStateRepo) Create(_ context.Context, s *domain.SyncState) error {
	m.states[s.RemoteID] = s
	return nil
}
func (m *mockStateRepo) GetByRemoteID(_ context.Context, _, _, remoteID string) (*domain.SyncState, error) {
	return m.states[remoteID], nil
}
func (m *mockStateRepo) GetByLocalID(_ context.Context, _, _, _ string) (*domain.SyncState, error) {
	return nil, nil
}
func (m *mockStateRepo) ListByUser(_ context.Context, _, _ string) ([]*domain.SyncState, error) {
	return nil, nil
}
func (m *mockStateRepo) Update(_ context.Context, s *domain.SyncState) error {
	m.states[s.RemoteID] = s
	return nil
}
func (m *mockStateRepo) Delete(_ context.Context, _ string) error          { return nil }
func (m *mockStateRepo) DeleteByUser(_ context.Context, _, _ string) error { return nil }

// --- fixtures ---

const (
	conflictUID  = "contact-uid"
	providerType = "google->internal"
	remoteID     = "people/c1"
)

func vcardWith(fn, email string) string {
	card := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + conflictUID + "\r\nFN:" + fn + "\r\n"
	if email != "" {
		card += "EMAIL:" + email + "\r\n"
	}
	return card + "END:VCARD\r\n"
}

func setupConflict(t *testing.T) (*service.SyncConflictService, *mockConflictRepo, *mockStateRepo, *mockContactRepo) {
	t.Helper()

	conflictRepo := &mockConflictRepo{byID: map[string]*domain.SyncConflict{}}
	stateRepo := &mockStateRepo{states: map[string]*domain.SyncState{}}
	contactRepo := newMockContactRepo()
	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}

	conflictRepo.byID["c1"] = &domain.SyncConflict{
		ID:             "c1",
		UserID:         "u1",
		ProviderType:   providerType,
		RemoteID:       remoteID,
		LocalContactID: conflictUID,
		BaseVCard:      vcardWith("Alice", ""),
		LocalVCard:     vcardWith("Alice Local", "local@example.com"),
		RemoteVCard:    vcardWith("Alice Remote", "remote@example.com"),
		RemoteETag:     "remote-etag-2",
		Status:         "pending",
	}

	stateRepo.states[remoteID] = &domain.SyncState{
		ID:           "s1",
		UserID:       "u1",
		ProviderType: providerType,
		RemoteID:     remoteID,
		LocalID:      conflictUID,
		RemoteETag:   "remote-etag-1",
		LocalETag:    "local-etag-1",
		BaseVCard:    vcardWith("Alice", ""),
	}

	contact := &domain.Contact{
		ID:            "contact-1",
		AddressBookID: testAddressBookID,
		UID:           conflictUID,
		ETag:          "local-etag-1",
		VCardData:     vcardWith("Alice Local", "local@example.com"),
	}
	contactRepo.contacts[contact.ID] = contact
	contactRepo.byUID[testAddressBookID+":"+conflictUID] = contact

	svc := service.NewSyncConflictService(conflictRepo, stateRepo, contactRepo, abRepo)
	return svc, conflictRepo, stateRepo, contactRepo
}

// --- tests ---

// The resolution must reach the contact. Previously it was written to the conflict row
// and read by nothing, so the user's choice changed no data at all.
func TestResolve_WritesResolvedVCardToContact(t *testing.T) {
	svc, _, _, contactRepo := setupConflict(t)

	_, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{
		"FN":    "remote",
		"EMAIL": "local",
	})
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	// The repository holds the same pointer the service mutates, so asserting on its
	// fields alone would pass even if nothing were ever persisted. Check the writes.
	if contactRepo.updates != 1 {
		t.Fatalf("contactRepo.Update called %d times, want 1 — the resolution was never persisted", contactRepo.updates)
	}
	if _, ok := contactRepo.emailsWritten["contact-1"]; !ok {
		t.Error("child records were not rewritten from the resolved vCard")
	}

	contact := contactRepo.contacts["contact-1"]
	if !strings.Contains(contact.VCardData, "Alice Remote") {
		t.Errorf("chosen remote FN not applied: %q", contact.VCardData)
	}
	if !strings.Contains(contact.VCardData, "local@example.com") {
		t.Errorf("chosen local EMAIL not applied: %q", contact.VCardData)
	}
	if contact.ETag == "local-etag-1" {
		t.Error("contact ETag must change after the resolution is written")
	}
	if contact.FirstName == "" && contact.LastName == "" && contact.Email == "" {
		t.Error("flat contact fields were not refreshed from the resolved vCard")
	}
}

// The sync state must move on, or the next run re-detects the same conflict.
func TestResolve_AdvancesSyncState(t *testing.T) {
	svc, _, stateRepo, _ := setupConflict(t)

	_, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{"FN": "remote"})
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	state := stateRepo.states[remoteID]
	if !strings.Contains(state.BaseVCard, "Alice Remote") {
		t.Errorf("merge base not advanced to the resolved vCard: %q", state.BaseVCard)
	}
	if state.RemoteETag != "remote-etag-2" {
		t.Errorf("RemoteETag = %q, want the etag seen when the conflict arose", state.RemoteETag)
	}
	// A cleared local ETag is what makes the next push carry the resolution to the remote.
	if state.LocalETag != "" {
		t.Errorf("LocalETag = %q, want empty so the resolution is pushed out", state.LocalETag)
	}
}

func TestResolve_MarksConflictResolved(t *testing.T) {
	svc, conflictRepo, _, _ := setupConflict(t)

	got, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{"FN": "local"})
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	stored := conflictRepo.byID["c1"]
	if stored.Status != "resolved" {
		t.Errorf("Status = %q, want resolved", stored.Status)
	}
	if stored.ResolvedAt == nil {
		t.Error("ResolvedAt not set")
	}
	if got.ResolvedVCard == "" {
		t.Error("ResolvedVCard not returned")
	}
}

func TestResolve_RejectsAnotherUsersConflict(t *testing.T) {
	svc, _, _, contactRepo := setupConflict(t)

	_, err := svc.Resolve(context.Background(), "someone-else", "c1", map[string]string{"FN": "local"})

	if !errors.Is(err, service.ErrConflictForbidden) {
		t.Fatalf("Resolve() = %v, want ErrConflictForbidden", err)
	}
	if contactRepo.updates != 0 {
		t.Error("contact must not be written for a forbidden request")
	}
}

func TestResolve_RejectsAlreadyResolved(t *testing.T) {
	svc, conflictRepo, _, _ := setupConflict(t)
	conflictRepo.byID["c1"].Status = "resolved"

	_, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{"FN": "local"})

	if !errors.Is(err, service.ErrConflictNotPending) {
		t.Fatalf("Resolve() = %v, want ErrConflictNotPending", err)
	}
}

func TestResolve_UnknownConflict(t *testing.T) {
	svc, _, _, _ := setupConflict(t)

	_, err := svc.Resolve(context.Background(), "u1", "missing", nil)

	if !errors.Is(err, service.ErrConflictNotFound) {
		t.Fatalf("Resolve() = %v, want ErrConflictNotFound", err)
	}
}

// If the contact was deleted between detection and resolution, say so rather than
// silently reporting success.
func TestResolve_ContactGone(t *testing.T) {
	svc, _, _, contactRepo := setupConflict(t)
	delete(contactRepo.byUID, testAddressBookID+":"+conflictUID)

	_, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{"FN": "local"})

	if !errors.Is(err, service.ErrConflictContactGone) {
		t.Fatalf("Resolve() = %v, want ErrConflictContactGone", err)
	}
}

// A resolution with no sync state left (pipeline deleted) still updates the contact.
func TestResolve_MissingSyncStateStillUpdatesContact(t *testing.T) {
	svc, _, stateRepo, contactRepo := setupConflict(t)
	delete(stateRepo.states, remoteID)

	_, err := svc.Resolve(context.Background(), "u1", "c1", map[string]string{"FN": "remote"})
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	if !strings.Contains(contactRepo.contacts["contact-1"].VCardData, "Alice Remote") {
		t.Error("contact must still receive the resolution")
	}
}

func TestDismiss_MarksDismissedWithoutTouchingContact(t *testing.T) {
	svc, conflictRepo, _, contactRepo := setupConflict(t)

	if err := svc.Dismiss(context.Background(), "u1", "c1"); err != nil {
		t.Fatalf("Dismiss() = %v", err)
	}

	if conflictRepo.byID["c1"].Status != "dismissed" {
		t.Errorf("Status = %q, want dismissed", conflictRepo.byID["c1"].Status)
	}
	if contactRepo.updates != 0 {
		t.Error("dismiss must not modify the contact")
	}
}

func TestDismiss_RejectsAnotherUsersConflict(t *testing.T) {
	svc, _, _, _ := setupConflict(t)

	err := svc.Dismiss(context.Background(), "intruder", "c1")

	if !errors.Is(err, service.ErrConflictForbidden) {
		t.Fatalf("Dismiss() = %v, want ErrConflictForbidden", err)
	}
}
