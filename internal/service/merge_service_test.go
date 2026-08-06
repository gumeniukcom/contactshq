package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// mockDupRepo implements repository.PotentialDuplicateRepository with call counters.
type mockDupRepo struct {
	records          map[string]*domain.PotentialDuplicate
	creates          int
	updates          int
	deleteByContacts []string
}

func newMockDupRepo() *mockDupRepo {
	return &mockDupRepo{records: map[string]*domain.PotentialDuplicate{}}
}

func (m *mockDupRepo) Create(_ context.Context, d *domain.PotentialDuplicate) error {
	m.creates++
	m.records[d.ID] = d
	return nil
}

// CreateIfAbsent mirrors INSERT ... ON CONFLICT DO NOTHING on the normalised pair.
func (m *mockDupRepo) CreateIfAbsent(ctx context.Context, d *domain.PotentialDuplicate) (bool, error) {
	if existing, _ := m.GetByContacts(ctx, d.UserID, d.ContactAID, d.ContactBID); existing != nil {
		return false, nil
	}
	return true, m.Create(ctx, d)
}

func (m *mockDupRepo) GetByID(_ context.Context, id string) (*domain.PotentialDuplicate, error) {
	return m.records[id], nil
}

func (m *mockDupRepo) GetByIDWithContacts(_ context.Context, userID, id string) (*domain.PotentialDuplicate, error) {
	d, ok := m.records[id]
	if !ok || d.UserID != userID {
		return nil, nil
	}
	return d, nil
}

func (m *mockDupRepo) ListByUser(_ context.Context, _, _ string, _, _ int) ([]*domain.PotentialDuplicate, int, error) {
	out := make([]*domain.PotentialDuplicate, 0, len(m.records))
	for _, d := range m.records {
		out = append(out, d)
	}
	return out, len(out), nil
}

func (m *mockDupRepo) GetByContacts(_ context.Context, _, aID, bID string) (*domain.PotentialDuplicate, error) {
	for _, d := range m.records {
		if (d.ContactAID == aID && d.ContactBID == bID) || (d.ContactAID == bID && d.ContactBID == aID) {
			return d, nil
		}
	}
	return nil, nil
}

func (m *mockDupRepo) Update(_ context.Context, d *domain.PotentialDuplicate) error {
	m.updates++
	m.records[d.ID] = d
	return nil
}

func (m *mockDupRepo) DeleteByContact(_ context.Context, contactID string) error {
	m.deleteByContacts = append(m.deleteByContacts, contactID)
	for id, d := range m.records {
		if d.ContactAID == contactID || d.ContactBID == contactID {
			delete(m.records, id)
		}
	}
	return nil
}

func (m *mockDupRepo) CountPending(_ context.Context, _ string) (int, error) {
	n := 0
	for _, d := range m.records {
		if d.Status == "pending" {
			n++
		}
	}
	return n, nil
}

// mockSyncStateRepo is a no-op stand-in; MergeService only holds the reference today.
type mockSyncStateRepo struct{}

func (m *mockSyncStateRepo) Create(context.Context, *domain.SyncState) error { return nil }
func (m *mockSyncStateRepo) GetByRemoteID(context.Context, string, string, string) (*domain.SyncState, error) {
	return nil, nil
}
func (m *mockSyncStateRepo) GetByLocalID(context.Context, string, string, string) (*domain.SyncState, error) {
	return nil, nil
}
func (m *mockSyncStateRepo) ListByUser(context.Context, string, string) ([]*domain.SyncState, error) {
	return nil, nil
}
func (m *mockSyncStateRepo) ListAllByUser(context.Context, string) ([]*domain.SyncState, error) {
	return nil, nil
}
func (m *mockSyncStateRepo) Update(context.Context, *domain.SyncState) error    { return nil }
func (m *mockSyncStateRepo) Delete(context.Context, string) error               { return nil }
func (m *mockSyncStateRepo) DeleteByUser(context.Context, string, string) error { return nil }

const (
	winnerCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:w1\r\nFN:Ada Lovelace\r\n" +
		"N:Lovelace;Ada;;;\r\nEMAIL:ada@example.com\r\nTEL:+15550001\r\nEND:VCARD\r\n"
	loserCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:l1\r\nFN:Ada Lovelace\r\n" +
		"N:Lovelace;Ada;;;\r\nEMAIL:ada.l@example.com\r\nTEL:+15550002\r\nORG:Analytical Engines\r\nEND:VCARD\r\n"
)

func setupMerge(t *testing.T) (*service.MergeService, *mockContactRepo, *mockDupRepo) {
	t.Helper()

	contactRepo := newMockContactRepo()
	for _, c := range []*domain.Contact{
		{
			ID: "w1", AddressBookID: testAddressBookID, UID: "w1",
			FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com",
			Phone: "+15550001", VCardData: winnerCard,
		},
		{
			ID: "l1", AddressBookID: testAddressBookID, UID: "l1",
			FirstName: "Ada", LastName: "Lovelace", Email: "ada.l@example.com",
			Phone: "+15550002", Org: "Analytical Engines", VCardData: loserCard,
		},
	} {
		contactRepo.contacts[c.ID] = c
		contactRepo.byUID[c.AddressBookID+":"+c.UID] = c
	}

	abRepo := &mockAbRepo{ab: &domain.AddressBook{ID: testAddressBookID, UserID: "u1"}}
	dupRepo := newMockDupRepo()

	return service.NewMergeService(contactRepo, abRepo, dupRepo, &mockSyncStateRepo{}), contactRepo, dupRepo
}

// ── Characterisation: what Merge does today ────────────────────────────────

func TestMerge_DeletesLoserAndKeepsWinner(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)

	got, err := svc.Merge(context.Background(), "u1", service.MergeInput{
		WinnerID: "w1", LoserID: "l1",
	})
	require.NoError(t, err)
	require.Equal(t, "w1", got.ID)

	require.NotContains(t, contactRepo.contacts, "l1", "the loser must be deleted")
	require.Contains(t, contactRepo.contacts, "w1")
}

func TestMerge_RemovesDuplicateRecordsForBothSides(t *testing.T) {
	svc, _, dupRepo := setupMerge(t)
	require.NoError(t, dupRepo.Create(context.Background(), &domain.PotentialDuplicate{
		ID: "d1", UserID: "u1", ContactAID: "w1", ContactBID: "l1", Status: "pending",
	}))

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err)

	require.Contains(t, dupRepo.deleteByContacts, "l1")
	require.Contains(t, dupRepo.deleteByContacts, "w1")
	require.Empty(t, dupRepo.records, "the pair must not survive the merge")
}

func TestMerge_RejectsSameContact(t *testing.T) {
	svc, _, _ := setupMerge(t)
	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "w1"})
	require.ErrorIs(t, err, service.ErrSameContact)
}

func TestMerge_RejectsContactFromAnotherAddressBook(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)
	contactRepo.contacts["foreign"] = &domain.Contact{ID: "foreign", AddressBookID: "someone-else"}

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "foreign"})
	require.ErrorIs(t, err, service.ErrContactNotFound)
	require.Contains(t, contactRepo.contacts, "foreign", "a foreign contact must not be touched")
}

// Resolution "loser" takes the loser's value for that field.
func TestMerge_ResolutionSelectsLoserField(t *testing.T) {
	svc, _, _ := setupMerge(t)

	got, err := svc.Merge(context.Background(), "u1", service.MergeInput{
		WinnerID: "w1", LoserID: "l1",
		Resolution: map[string]string{"EMAIL": "loser"},
	})
	require.NoError(t, err)
	require.Equal(t, "ada.l@example.com", got.Email,
		"the loser's email should have been selected")
}

// ── Behaviour introduced in phase 4 ────────────────────────────────────────
//
// These were written in task 1.1 as skipped RED tests describing what Merge had to become.
// Task 4.1 made them true and removed the skips.

func TestMerge_WritesThroughSaveSoChildRowsAndChangeSeqFollow(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.NoError(t, err)

	require.Positive(t, contactRepo.saves,
		"merge must go through Save so child tables and change_seq are written in one transaction")
	require.Zero(t, contactRepo.updates,
		"Update writes only the flat row: child tables keep the pre-merge values and CardDAV never sees the change")
}

func TestMerge_RewritesWinnerChildRows(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{
		WinnerID: "w1", LoserID: "l1",
		Resolution: map[string]string{"EMAIL": "loser"},
	})
	require.NoError(t, err)

	require.NotEmpty(t, contactRepo.emailsWritten["w1"],
		"the merged emails must be persisted to contact_emails, not just the flat column")
}

// Characterisation, not RED. The plan expected an unparseable merge result to blank the
// winner's flat fields, because Merge does `mergedParsed, _ := ParseVCard(...)` and
// substitutes an empty struct when that returns nil. In practice ApplyResolution rejects the
// input first, so the blanking path is not reachable this way and the winner is left alone.
//
// Phase 4.1 must keep it that way: whatever replaces this code, a merge that cannot be
// completed must not half-write the winner.
func TestMerge_UnparseableLoserCardFailsWithoutTouchingTheWinner(t *testing.T) {
	svc, contactRepo, _ := setupMerge(t)
	contactRepo.contacts["l1"].VCardData = "NOT A VCARD AT ALL"

	_, err := svc.Merge(context.Background(), "u1", service.MergeInput{WinnerID: "w1", LoserID: "l1"})
	require.Error(t, err, "an unusable input must be refused rather than merged in part")

	winner := contactRepo.contacts["w1"]
	require.Equal(t, "Ada", winner.FirstName, "the winner's fields must survive a failed merge")
	require.Equal(t, "Lovelace", winner.LastName)
	require.Equal(t, winnerCard, winner.VCardData)
	require.Contains(t, contactRepo.contacts, "l1", "the loser must not be deleted by a failed merge")
}
