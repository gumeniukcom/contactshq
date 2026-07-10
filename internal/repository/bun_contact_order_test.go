package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// setupOrderTest inserts contacts whose insertion order is deliberately different from
// every sort order under test, so a query that loses its ORDER BY cannot pass by luck.
func setupOrderTest(t *testing.T) (context.Context, *bun.DB, *repository.BunContactRepository, string) {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))
	repo := repository.NewBunContactRepository(db)

	userID := uuid.New().String()
	abID := uuid.New().String()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES (?,?,?)`, userID, "order@x.com", "h")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO address_books (id, user_id, name) VALUES (?,?,?)`, abID, userID, "ab")
	require.NoError(t, err)

	now := time.Now()
	rows := []struct{ first, last, email string }{
		{"Mallory", "Zimmerman", "m@example.com"},
		{"Alice", "Anderson", "a@example.com"},
		{"Trent", "Miller", "t@example.com"},
		{"Bob", "Brown", "b@example.com"},
	}
	for i, r := range rows {
		c := &domain.Contact{
			ID:            uuid.New().String(),
			AddressBookID: abID,
			UID:           uuid.New().String(),
			FirstName:     r.first,
			LastName:      r.last,
			Email:         r.email,
			CreatedAt:     now.Add(time.Duration(i) * time.Minute),
			UpdatedAt:     now,
		}
		require.NoError(t, repo.Create(ctx, c))
		require.NoError(t, repo.ReplaceEmails(ctx, c.ID, []*domain.ContactEmail{
			{Value: r.email, Type: "work"},
		}))
	}

	return ctx, db, repo, abID
}

func lastNames(contacts []*domain.Contact) []string {
	out := make([]string, len(contacts))
	for i, c := range contacts {
		out[i] = c.LastName
	}
	return out
}

// ListWithRelations re-queried the parent rows to attach children, and that second query
// carried no ORDER BY. Bun resets the slice and refills it in scan order, so the sort the
// caller asked for was thrown away — the contacts list ignored its sort controls, and
// pagination returned unstable pages.
func TestListWithRelations_PreservesSortOrder(t *testing.T) {
	ctx, _, repo, abID := setupOrderTest(t)

	tests := []struct {
		name    string
		filters repository.ListFilters
		want    []string
	}{
		{
			name:    "name ascending",
			filters: repository.ListFilters{SortBy: "name", SortDir: "asc"},
			want:    []string{"Anderson", "Brown", "Miller", "Zimmerman"},
		},
		{
			name:    "name descending",
			filters: repository.ListFilters{SortBy: "name", SortDir: "desc"},
			want:    []string{"Zimmerman", "Miller", "Brown", "Anderson"},
		},
		{
			// Emails are a@, b@, m@, t@ — a different sequence from the name order,
			// which is the point: the sort column has to actually drive the result.
			name:    "email ascending",
			filters: repository.ListFilters{SortBy: "email", SortDir: "asc"},
			want:    []string{"Anderson", "Brown", "Zimmerman", "Miller"},
		},
		{
			name:    "created_at descending",
			filters: repository.ListFilters{SortBy: "created_at", SortDir: "desc"},
			want:    []string{"Brown", "Miller", "Anderson", "Zimmerman"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contacts, total, err := repo.ListWithRelations(ctx, abID, 10, 0, tt.filters)
			require.NoError(t, err)
			assert.Equal(t, 4, total)
			assert.Equal(t, tt.want, lastNames(contacts))
		})
	}
}

// The relations must still arrive; preserving order is worthless if the children are lost.
func TestListWithRelations_StillLoadsChildren(t *testing.T) {
	ctx, _, repo, abID := setupOrderTest(t)

	contacts, _, err := repo.ListWithRelations(ctx, abID, 10, 0, repository.ListFilters{SortBy: "name"})
	require.NoError(t, err)
	require.Len(t, contacts, 4)

	for _, c := range contacts {
		assert.Len(t, c.Emails, 1, "contact %s should carry its child email rows", c.LastName)
		if len(c.Emails) == 1 {
			assert.Equal(t, c.Email, c.Emails[0].Value)
		}
	}
}

// Paging must slice the sorted sequence, not an arbitrary one.
func TestListWithRelations_PaginationIsStable(t *testing.T) {
	ctx, _, repo, abID := setupOrderTest(t)
	filters := repository.ListFilters{SortBy: "name", SortDir: "asc"}

	page1, total, err := repo.ListWithRelations(ctx, abID, 2, 0, filters)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	assert.Equal(t, []string{"Anderson", "Brown"}, lastNames(page1))

	page2, _, err := repo.ListWithRelations(ctx, abID, 2, 2, filters)
	require.NoError(t, err)
	assert.Equal(t, []string{"Miller", "Zimmerman"}, lastNames(page2))
}

func TestSearchWithRelations_PreservesSortOrder(t *testing.T) {
	ctx, _, repo, abID := setupOrderTest(t)

	contacts, _, err := repo.SearchWithRelations(ctx, abID, "example.com", 10, 0,
		repository.ListFilters{SortBy: "name", SortDir: "desc"})
	require.NoError(t, err)

	assert.Equal(t, []string{"Zimmerman", "Miller", "Brown", "Anderson"}, lastNames(contacts))
	for _, c := range contacts {
		assert.Len(t, c.Emails, 1)
	}
}
