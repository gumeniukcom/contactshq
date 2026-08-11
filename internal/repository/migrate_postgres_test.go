package repository_test

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

// setupPostgres connects to the database named by TEST_POSTGRES_DSN and hands back an
// empty schema. Every migration is written in SQLite-flavoured SQL, yet docker-compose —
// the documented way to install this — runs PostgreSQL. Nothing verified that the two
// agreed until these tests existed.
//
// The test skips when the variable is unset, so `go test ./...` still works offline.
func setupPostgres(t *testing.T) *bun.DB {
	t.Helper()

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set; skipping PostgreSQL migration tests")
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.Ping(), "cannot reach the PostgreSQL named by TEST_POSTGRES_DSN")

	// Each test gets a clean public schema.
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)

	return db
}

// expectedTables is the full schema as it must exist after every migration has run.
//
// Adding a table means adding it here in the same PR. The assertion below compares sets in
// both directions on purpose: a one-way "does it exist" loop silently tolerated five tables
// that were never checked on PostgreSQL at all (potential_duplicates, user_backup_settings,
// user_dedup_settings, sync_cursors, contact_tombstones).
//
// `jobs` is deliberately absent: migration 020 drops it.
var expectedTables = []string{
	"address_books",
	"app_passwords",
	"backup_runs",
	"contact_addresses",
	"contact_categories",
	"contact_dates",
	"contact_emails",
	"contact_ims",
	"contact_phones",
	"contact_tombstones",
	"contact_urls",
	"contacts",
	"merge_log",
	"pipeline_steps",
	"pipelines",
	"potential_duplicates",
	"provider_connections",
	"schema_migrations",
	"sync_conflicts",
	"sync_cursors",
	"sync_runs",
	"sync_states",
	"user_backup_settings",
	"user_dedup_settings",
	"users",
}

func TestPostgres_MigrateAppliesEverySchemaObject(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))

	rows, err := db.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var actual []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		actual = append(actual, name)
	}
	require.NoError(t, rows.Err())

	sort.Strings(actual)
	want := append([]string(nil), expectedTables...)
	sort.Strings(want)

	assert.Equal(t, want, actual,
		"the PostgreSQL schema does not match expectedTables — a new table must be added to that list in the same PR")
}

func TestPostgres_MigrateIsIdempotent(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, repository.Migrate(ctx, db))
	require.NoError(t, repository.Migrate(ctx, db), "a second run must apply nothing")

	var applied int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied))
	assert.Greater(t, applied, 15)
}

// Migration 019 rewrites rows with substr() and || string concatenation. Those behave
// differently across dialects, and it had only ever run on SQLite.
func TestPostgres_Migration019SwapsInvertedRows(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()

	require.NoError(t, migrateTo(ctx, db, "018_sync_conflict_remote_etag"))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role)
		 VALUES ('u1', 'a@example.com', 'x', 'A', 'user')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO pipelines (id, user_id, name, enabled) VALUES ('p1', 'u1', 'P', true)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO pipeline_steps (id, pipeline_id, step_order, source_type, source_config,
		    dest_type, dest_config, conflict_mode, direction)
		 VALUES ('s1', 'p1', 1, 'internal', '{}', 'carddav', '{"endpoint":"x"}', 'source_wins', 'pull')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_states (id, user_id, provider_type, remote_id, local_id, remote_etag, local_etag, content_hash)
		 VALUES ('st1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'etag-internal', 'etag-google', 'h')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO sync_conflicts (id, user_id, provider_type, remote_id, local_contact_id, status)
		 VALUES ('c1', 'u1', 'internal->google', 'local-uid', 'people/c1', 'pending')`)
	require.NoError(t, err)

	require.NoError(t, migrateTo(ctx, db, "019_normalize_pipeline_direction"))

	var source, dest, direction, conflict string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT source_type, dest_type, direction, conflict_mode FROM pipeline_steps WHERE id='s1'").
		Scan(&source, &dest, &direction, &conflict))
	assert.Equal(t, "carddav", source)
	assert.Equal(t, "internal", dest)
	assert.Equal(t, "export", direction)
	assert.Equal(t, "dest_wins", conflict)

	var providerType, remoteID, localID string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT provider_type, remote_id, local_id FROM sync_states WHERE id='st1'").
		Scan(&providerType, &remoteID, &localID))
	assert.Equal(t, "google->internal", providerType, "substr() and || must behave as on SQLite")
	assert.Equal(t, "people/c1", remoteID)
	assert.Equal(t, "local-uid", localID)

	var conflicts int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_conflicts").Scan(&conflicts))
	assert.Equal(t, 0, conflicts, "unresolvable conflicts on inverted pipelines are dropped")
}

// The contact write path fans out across eight tables in one transaction; PostgreSQL
// enforces foreign keys strictly, so a mistake there shows up here and not on SQLite.
func TestPostgres_SaveContactWithChildren(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO address_books (id, user_id, name) VALUES ('ab1', 'u1', 'Contacts')`)
	require.NoError(t, err)

	repo := repository.NewBunContactRepository(db)
	c := newContact("ab1")

	require.NoError(t, repo.Save(ctx, c, childRecordsFixture()))

	got, err := repo.GetByIDWithRelations(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Emails, 1)
	assert.Len(t, got.Phones, 1)
	assert.Len(t, got.Categories, 1)

	// Deleting the contact must cascade to its children.
	require.NoError(t, repo.Delete(ctx, c.ID))

	var orphans int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM contact_emails WHERE contact_id = ?", c.ID).Scan(&orphans))
	assert.Equal(t, 0, orphans, "child rows must cascade on delete")
}

func childRecordsFixture() domain.ChildRecords {
	return domain.ChildRecords{
		Emails:     []*domain.ContactEmail{{Value: "jane@example.com", Type: "work"}},
		Phones:     []*domain.ContactPhone{{Value: "+15551234567", Type: "cell"}},
		Categories: []*domain.ContactCategory{{Value: "vip"}},
	}
}

// The change journal bumps a counter with UPDATE ... RETURNING inside the write's
// transaction. RETURNING is the kind of thing that differs between dialects.
func TestPostgres_ChangeJournal(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO address_books (id, user_id, name) VALUES ('ab1', 'u1', 'Contacts')`)
	require.NoError(t, err)

	repo := repository.NewBunContactRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)

	start, err := abRepo.ChangeSeq(ctx, "ab1")
	require.NoError(t, err)

	c := newContact("ab1")
	c.UID = "u-1"
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{}))

	afterWrite, err := abRepo.ChangeSeq(ctx, "ab1")
	require.NoError(t, err)
	assert.Greater(t, afterWrite, start, "UPDATE ... RETURNING must advance the counter")

	baseline, err := repo.ChangesSince(ctx, "ab1", 0)
	require.NoError(t, err)
	require.Len(t, baseline.Updated, 1)

	require.NoError(t, repo.Delete(ctx, c.ID))

	changes, err := repo.ChangesSince(ctx, "ab1", baseline.Seq)
	require.NoError(t, err)
	assert.Equal(t, []string{"u-1"}, changes.DeletedUIDs, "the tombstone must be readable on PostgreSQL")
}

// The cursor store upserts with ON CONFLICT ... DO UPDATE, whose exact form differs
// between dialects.
func TestPostgres_SyncCursorUpsert(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)

	repo := repository.NewBunSyncCursorRepository(db)

	require.NoError(t, repo.Set(ctx, "u1", "google->internal", "token-1"))
	require.NoError(t, repo.Set(ctx, "u1", "google->internal", "token-2"))

	got, err := repo.Get(ctx, "u1", "google->internal")
	require.NoError(t, err)
	assert.Equal(t, "token-2", got, "the second Set must update, not insert a duplicate")
}

// merge_log is written on every merge and pruned by retention; ON CONFLICT-free but still
// worth proving on PostgreSQL, since a table that only ever ran on SQLite is how the five
// tables missing from expectedTables went unnoticed.
func TestPostgres_MergeLogRoundTrip(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.NewInsert().Model(&domain.User{
		ID: "u1", Email: "owner@example.com", PasswordHash: "x", Role: "admin",
	}).Exec(ctx)
	require.NoError(t, err)

	repo := repository.NewBunMergeLogRepository(db)
	require.NoError(t, repo.Create(ctx, &domain.MergeLogEntry{
		ID: "m1", UserID: "u1", WinnerID: "w", LoserUID: "l", Resolution: "{}",
		MergedAt: time.Now().AddDate(0, 0, -40),
	}))
	require.NoError(t, repo.Create(ctx, &domain.MergeLogEntry{
		ID: "m2", UserID: "u1", WinnerID: "w", LoserUID: "l2", Resolution: "{}",
	}))

	entries, err := repo.ListByUser(ctx, "u1", 50)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	removed, err := repo.DeleteOlderThan(ctx, time.Now().AddDate(0, 0, -30))
	require.NoError(t, err)
	require.Equal(t, 1, removed)
}

// ON CONFLICT with an explicit target behaves differently across databases; on PostgreSQL an
// unnamed target lets the clause swallow a primary-key collision instead of the pair
// collision it was written for. CI runs only TestPostgres* in this package, so without this
// the whole insert path would be exercised on SQLite alone.
func TestPostgresPotentialDuplicateUniqueIndex(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.NewInsert().Model(&domain.User{
		ID: "u1", Email: "owner@example.com", PasswordHash: "x", Role: "admin",
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&domain.AddressBook{ID: "ab1", UserID: "u1", Name: "Contacts"}).Exec(ctx)
	require.NoError(t, err)
	for _, id := range []string{"a", "b"} {
		_, err = db.NewInsert().Model(&domain.Contact{ID: id, AddressBookID: "ab1", UID: id}).Exec(ctx)
		require.NoError(t, err)
	}

	repo := repository.NewBunPotentialDuplicateRepository(db)
	pair := func(id string) *domain.PotentialDuplicate {
		return &domain.PotentialDuplicate{
			ID: id, UserID: "u1", ContactAID: "a", ContactBID: "b",
			Score: 1.0, MatchReasons: "[]", Status: "pending", CreatedAt: time.Now(),
		}
	}

	created, err := repo.CreateIfAbsent(ctx, pair("d1"))
	require.NoError(t, err)
	require.True(t, created)

	// Same pair, different primary key: the pair index must reject it, quietly.
	created, err = repo.CreateIfAbsent(ctx, pair("d2"))
	require.NoError(t, err)
	require.False(t, created, "the same pair must not be recorded twice")

	_, total, err := repo.ListByUser(ctx, "u1", repository.StatusAll, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
}

// TestPostgresListDedupValues is the only PostgreSQL coverage the dedup value projection gets:
// CI runs `go test ./internal/repository/ -run TestPostgres`, so a test outside package
// `repository` or without this name prefix never touches PostgreSQL at all.
//
// Two things are asserted, and both are the point of the method rather than of the query
// engine: the two projections are scoped to one address book, and they carry nothing but a
// contact id and a value. The second matters because the whole reason this is a side read
// instead of Relation("Emails") is that a relation load drags vcard_data and photo_uri along.
func TestPostgresListDedupValues(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.NewInsert().Model(&domain.User{
		ID: "u1", Email: "owner@example.com", PasswordHash: "x", Role: "admin",
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&domain.User{
		ID: "u2", Email: "other@example.com", PasswordHash: "x", Role: "user",
	}).Exec(ctx)
	require.NoError(t, err)
	for _, ab := range []struct{ id, user string }{{"ab1", "u1"}, {"ab2", "u2"}} {
		_, err = db.NewInsert().Model(&domain.AddressBook{ID: ab.id, UserID: ab.user, Name: "Contacts"}).Exec(ctx)
		require.NoError(t, err)
	}

	repo := repository.NewBunContactRepository(db)

	// Card data on the row, so a projection that widened to whole contacts would show it.
	mine := newContact("ab1")
	mine.VCardData = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Jane Doe\r\nEND:VCARD\r\n"
	mine.PhotoURI = "data:image/png;base64,iVBORw0KGgo="
	require.NoError(t, repo.Save(ctx, mine, domain.ChildRecords{
		Emails: []*domain.ContactEmail{
			{Value: "jane@example.com", Type: "work"},
			{Value: "jane.doe@example.org", Type: "home"},
		},
		Phones: []*domain.ContactPhone{{Value: "+1 (555) 9000", Type: "cell"}},
	}))

	// Another user's address book, with values that must not leak into the result.
	theirs := newContact("ab2")
	require.NoError(t, repo.Save(ctx, theirs, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "someone@elsewhere.example", Type: "work"}},
		Phones: []*domain.ContactPhone{{Value: "+1 555 1111", Type: "cell"}},
	}))

	emails, phones, err := repo.ListDedupValues(ctx, "ab1")
	require.NoError(t, err)

	gotEmails := make([]string, 0, len(emails))
	for _, e := range emails {
		assert.Equal(t, mine.ID, e.ContactID, "the projection is scoped to one address book")
		gotEmails = append(gotEmails, e.Value)
	}
	sort.Strings(gotEmails)
	assert.Equal(t, []string{"jane.doe@example.org", "jane@example.com"}, gotEmails)

	require.Len(t, phones, 1)
	assert.Equal(t, mine.ID, phones[0].ContactID)
	assert.Equal(t, "+1 (555) 9000", phones[0].Value, "normalisation belongs to the detector")

	// The projection is two columns by construction. Widening the struct would silently
	// widen every read, which is exactly what the narrow-read comment on ListForDedup and
	// ListDedupValues exists to prevent.
	assert.Equal(t, 2, reflect.TypeOf(domain.ContactValueRef{}).NumField(),
		"ContactValueRef must stay a two-column projection: no card data")
}

// LIKE is case-sensitive on PostgreSQL and folds ASCII on SQLite. The whole suite runs on
// SQLite, so a bare LIKE passed everywhere while search silently missed most matches on the
// engine docker-compose provisions. This test has to be named TestPostgres… and live in this
// package, or CI's `-run TestPostgres` filter never executes it.
func TestPostgres_SearchIsCaseInsensitive(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO address_books (id, user_id, name) VALUES ('ab1', 'u1', 'Contacts')`)
	require.NoError(t, err)

	repo := repository.NewBunContactRepository(db)
	c := newContact("ab1")
	c.FirstName = "John"
	c.LastName = "Smith"
	require.NoError(t, repo.Save(ctx, c, domain.ChildRecords{
		Emails: []*domain.ContactEmail{{Value: "John.Smith@Example.COM", Type: "work"}},
	}))

	for _, query := range []string{"john", "JOHN", "JoHn", "smith", "example.com"} {
		found, count, err := repo.Search(ctx, "ab1", query, 50, 0, repository.ListFilters{})
		require.NoError(t, err, "query %q", query)
		assert.Equal(t, 1, count, "query %q should match regardless of case", query)
		assert.Len(t, found, 1, "query %q", query)
	}
}

// A result that matched nothing must serialise as [] rather than null: the contact list view
// reads .length off the response, and null blanked the screen on any no-match search.
func TestPostgres_EmptyListAndSearchAreNotNil(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	require.NoError(t, repository.Migrate(ctx, db))

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('u1', 'a@example.com', 'x')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO address_books (id, user_id, name) VALUES ('ab1', 'u1', 'Contacts')`)
	require.NoError(t, err)

	repo := repository.NewBunContactRepository(db)

	list, count, err := repo.List(ctx, "ab1", 50, 0, repository.ListFilters{})
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.NotNil(t, list, "an empty page must marshal to [] and not null")

	found, count, err := repo.Search(ctx, "ab1", "nothing-matches-this", 50, 0, repository.ListFilters{})
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.NotNil(t, found, "an empty search must marshal to [] and not null")
}
