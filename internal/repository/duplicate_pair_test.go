package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func newPairDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.Migrate(context.Background(), db))
	return db
}

type seededPair struct {
	db  *bun.DB
	dup *domain.PotentialDuplicate
}

// seedPair creates two users' worth of scaffolding and one duplicate pair where A holds both
// of B's values plus one more, so B is a subset of A but not the other way round.
func seedPair(t *testing.T, db *bun.DB) seededPair {
	t.Helper()
	ctx := context.Background()

	for _, u := range []*domain.User{
		{ID: "u1", Email: "owner@example.com", PasswordHash: "x", Role: "admin"},
		{ID: "u2", Email: "other@example.com", PasswordHash: "x", Role: "user"},
	} {
		_, err := db.NewInsert().Model(u).Exec(ctx)
		require.NoError(t, err)
	}

	_, err := db.NewInsert().Model(&domain.AddressBook{ID: "ab1", UserID: "u1", Name: "Contacts"}).Exec(ctx)
	require.NoError(t, err)

	for _, c := range []*domain.Contact{
		{ID: "a", AddressBookID: "ab1", UID: "a", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"},
		{ID: "b", AddressBookID: "ab1", UID: "b", FirstName: "Ada", LastName: "L", Email: "ada@example.com"},
	} {
		_, err := db.NewInsert().Model(c).Exec(ctx)
		require.NoError(t, err)
	}

	// A: two emails and a phone. B: one email (also on A) and the same phone written
	// differently — so everything B has, A has.
	rows := []any{
		&domain.ContactEmail{ID: "ae1", ContactID: "a", Value: "ada@example.com"},
		&domain.ContactEmail{ID: "ae2", ContactID: "a", Value: "ada.work@example.com"},
		&domain.ContactPhone{ID: "ap1", ContactID: "a", Value: "+1 555 0001"},
		&domain.ContactEmail{ID: "be1", ContactID: "b", Value: "ADA@example.com"},
		&domain.ContactPhone{ID: "bp1", ContactID: "b", Value: "15550001"},
	}
	for _, row := range rows {
		_, err := db.NewInsert().Model(row).Exec(ctx)
		require.NoError(t, err)
	}

	dup := &domain.PotentialDuplicate{
		ID: "d1", UserID: "u1", ContactAID: "a", ContactBID: "b",
		Score: 1.0, MatchReasons: `[{"code":"email_match","value":"ada@example.com"}]`,
		Status: "pending", CreatedAt: time.Now(),
	}
	_, err = db.NewInsert().Model(dup).Exec(ctx)
	require.NoError(t, err)

	return seededPair{db: db, dup: dup}
}

// The merge screen needs every value of both sides; the list deliberately does not carry them.
func TestPotentialDuplicate_GetByIDWithContactsLoadsChildCollections(t *testing.T) {
	db := newPairDB(t)
	seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)

	got, err := repo.GetByIDWithContacts(context.Background(), "u1", "d1")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, got.ContactA)
	require.NotNil(t, got.ContactB)
	require.Len(t, got.ContactA.Emails, 2, "A's emails must be loaded")
	require.Len(t, got.ContactA.Phones, 1)
	require.Len(t, got.ContactB.Emails, 1)
	require.Len(t, got.ContactB.Phones, 1)
}

// Ownership is filtered inside the query, not left to the caller to remember.
func TestPotentialDuplicate_GetByIDWithContactsRejectsAnotherUser(t *testing.T) {
	db := newPairDB(t)
	seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)

	got, err := repo.GetByIDWithContacts(context.Background(), "u2", "d1")
	require.NoError(t, err)
	require.Nil(t, got, "another user's pair must not be readable")
}

func TestPotentialDuplicate_GetByIDWithContactsUnknownID(t *testing.T) {
	db := newPairDB(t)
	seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)

	got, err := repo.GetByIDWithContacts(context.Background(), "u1", "nope")
	require.NoError(t, err)
	require.Nil(t, got)
}

// The list answers "would keeping this side lose anything?" in SQL, without loading values.
func TestPotentialDuplicate_ListByUserComputesSubsetFlags(t *testing.T) {
	db := newPairDB(t)
	seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)

	dups, total, err := repo.ListByUser(context.Background(), "u1", "pending", 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, dups, 1)

	require.True(t, dups[0].BSubsetOfA,
		"B's email differs only in case and its phone only in formatting, so keeping A loses nothing")
	require.False(t, dups[0].ASubsetOfB,
		"A has an email B does not, so keeping B would lose it")

	// The list must not drag in the child collections it was told not to load.
	require.Empty(t, dups[0].ContactA.Emails)
}

// An empty status used to be replaced by "pending" further up, making dismissed pairs
// unreachable; "all" is the explicit way to ask for everything.
func TestPotentialDuplicate_ListByUserStatusAll(t *testing.T) {
	db := newPairDB(t)
	seeded := seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)
	ctx := context.Background()

	seeded.dup.Status = "dismissed"
	require.NoError(t, repo.Update(ctx, seeded.dup))

	pending, _, err := repo.ListByUser(ctx, "u1", "pending", 20, 0)
	require.NoError(t, err)
	require.Empty(t, pending)

	all, total, err := repo.ListByUser(ctx, "u1", repository.StatusAll, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, all, 1)
}

// A failed read must not hand back a half-populated model alongside the error.
func TestPotentialDuplicate_GetByIDReturnsNilForUnknownID(t *testing.T) {
	db := newPairDB(t)
	seedPair(t, db)
	repo := repository.NewBunPotentialDuplicateRepository(db)

	got, err := repo.GetByID(context.Background(), "nope")
	require.NoError(t, err)
	require.Nil(t, got)
}
