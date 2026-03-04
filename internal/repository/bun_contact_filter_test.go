package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func setupFilterTest(t *testing.T) (context.Context, *repository.BunContactRepository, string) {
	t.Helper()
	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))
	repo := repository.NewBunContactRepository(db)

	userID := uuid.New().String()
	abID := uuid.New().String()

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "filter@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, userID, "ab")
	require.NoError(t, err)

	now := time.Now()
	contacts := []struct {
		firstName, lastName, email, phone, org string
		categories                             []string
	}{
		{"Alice", "Smith", "alice@example.com", "+1111", "Acme Corp", []string{"vip", "customer"}},
		{"Bob", "Jones", "", "+2222", "Acme Corp", []string{"customer"}},
		{"Charlie", "Brown", "charlie@example.com", "", "Other Inc", nil},
		{"Diana", "Prince", "diana@example.com", "+4444", "", []string{"vip"}},
	}

	for i, c := range contacts {
		cID := uuid.New().String()
		contact := &domain.Contact{
			ID: cID, AddressBookID: abID, UID: uuid.New().String(),
			ETag: "e", VCardData: "BEGIN:VCARD\nEND:VCARD",
			FirstName: c.firstName, LastName: c.lastName,
			Email: c.email, Phone: c.phone, Org: c.org,
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
			UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, repo.Create(ctx, contact))
		if len(c.categories) > 0 {
			var cats []*domain.ContactCategory
			for _, cat := range c.categories {
				cats = append(cats, &domain.ContactCategory{Value: cat})
			}
			require.NoError(t, repo.ReplaceCategories(ctx, cID, cats))
		}
		if c.email != "" {
			require.NoError(t, repo.ReplaceEmails(ctx, cID, []*domain.ContactEmail{{Value: c.email, Type: "work"}}))
		}
		if c.phone != "" {
			require.NoError(t, repo.ReplacePhones(ctx, cID, []*domain.ContactPhone{{Value: c.phone, Type: "cell"}}))
		}
	}

	return ctx, repo, abID
}

func TestContactList_SortByNameAsc(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{SortBy: "name", SortDir: "asc"})
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	require.Len(t, results, 4)
	assert.Equal(t, "Charlie", results[0].FirstName) // Brown
	assert.Equal(t, "Bob", results[1].FirstName)     // Jones
	assert.Equal(t, "Diana", results[2].FirstName)   // Prince
	assert.Equal(t, "Alice", results[3].FirstName)   // Smith
}

func TestContactList_SortByNameDesc(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{SortBy: "name", SortDir: "desc"})
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	require.Len(t, results, 4)
	assert.Equal(t, "Alice", results[0].FirstName)   // Smith
	assert.Equal(t, "Diana", results[1].FirstName)   // Prince
	assert.Equal(t, "Bob", results[2].FirstName)     // Jones
	assert.Equal(t, "Charlie", results[3].FirstName) // Brown
}

func TestContactList_SortByCreatedAt(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	results, _, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{SortBy: "created_at", SortDir: "desc"})
	require.NoError(t, err)
	require.Len(t, results, 4)
	assert.Equal(t, "Diana", results[0].FirstName)
	assert.Equal(t, "Charlie", results[1].FirstName)
}

func TestContactList_FilterByCategory(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{Category: []string{"vip"}})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
}

func TestContactList_FilterByOrg(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{Org: "Acme Corp"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
}

func TestContactList_FilterHasEmail(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	hasEmail := true
	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{HasEmail: &hasEmail})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, results, 3)
}

func TestContactList_FilterHasPhone(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	hasPhone := true
	results, total, err := repo.List(ctx, abID, 10, 0, repository.ListFilters{HasPhone: &hasPhone})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, results, 3)
}

func TestContactFacets(t *testing.T) {
	ctx, repo, abID := setupFilterTest(t)

	facets, err := repo.Facets(ctx, abID)
	require.NoError(t, err)
	assert.Equal(t, 4, facets.Total)
	assert.ElementsMatch(t, []string{"customer", "vip"}, facets.Categories)
	assert.ElementsMatch(t, []string{"Acme Corp", "Other Inc"}, facets.Orgs)
}
