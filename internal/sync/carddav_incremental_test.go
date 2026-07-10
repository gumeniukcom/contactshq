package sync_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	chqcarddav "github.com/gumeniukcom/contactshq/internal/carddav"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

// Migrate() reads the embedded migrations, but a couple of helpers still resolve paths
// relative to the module root, so chdir there.
func TestMain(m *testing.M) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			_ = os.Chdir(dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	os.Exit(m.Run())
}

// This drives the CardDAV client's ListChanges against our own CardDAV server, so it
// exercises C (the sync-collection client) and E (the sync-collection server) as one
// round trip: exactly what a pipeline pulling from another ContactsHQ instance does.
func TestCardDAVIncremental_AgainstOwnServer(t *testing.T) {
	const email = "user@example.com"
	const password = "correct-horse-battery-staple"

	ctx := context.Background()

	sqldb, err := sql.Open(sqliteshim.ShimName, "file:carddav_inc?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, repository.Migrate(ctx, db))

	userRepo := repository.NewBunUserRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)
	contactRepo := repository.NewBunContactRepository(db)
	appPwRepo := repository.NewBunAppPasswordRepository(db)

	authSvc := service.NewAuthService(userRepo, abRepo, config.AuthConfig{
		JWTSecret: "0123456789abcdef0123456789abcdef", TokenTTL: time.Hour,
	})
	_, err = authSvc.Register(ctx, email, password, "User")
	require.NoError(t, err)

	const prefix = "/dav"
	backend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, prefix)
	davServer := chqcarddav.NewServer(backend, userRepo, appPwRepo, prefix)

	app := fiber.New(fiber.Config{
		RequestMethods: append(fiber.DefaultMethods, "PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"),
	})
	app.Use(prefix, adaptor.HTTPHandler(davServer))

	httpServer := httptest.NewServer(adaptor.FiberApp(app))
	t.Cleanup(httpServer.Close)

	provider, err := chqsync.NewCardDAVClientProviderWithOptions(
		httpServer.URL+prefix+"/"+email+"/addressbooks/contacts/", email, password, false)
	require.NoError(t, err)

	// Seed two contacts directly through the backend context.
	seed := func(uid, name string) {
		card := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\nEND:VCARD\r\n"
		put := httptest.NewRequest(http.MethodPut,
			prefix+"/"+email+"/addressbooks/contacts/"+uid+".vcf", nil)
		_ = put
		// Use the client provider's Put so the round trip is realistic.
		_, err := provider.Put(ctx, chqsync.SyncItem{RemoteID: uid, VCardData: card})
		require.NoError(t, err)
	}
	seed("c1", "Alice")
	seed("c2", "Bob")

	// First sync: empty cursor, full listing, fresh token.
	first, err := provider.ListChanges(ctx, "")
	require.NoError(t, err)
	assert.True(t, first.Full, "the first sync is a full listing")
	assert.Len(t, first.Updated, 2)
	require.NotEmpty(t, first.Cursor, "the server must hand back a sync token")

	// Change one, delete the other.
	_, err = provider.Put(ctx, chqsync.SyncItem{RemoteID: "c1",
		VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c1\r\nFN:Alice Renamed\r\nEND:VCARD\r\n"})
	require.NoError(t, err)
	require.NoError(t, provider.Delete(ctx, "c2"))

	// Second sync from the stored cursor: only the delta.
	second, err := provider.ListChanges(ctx, first.Cursor)
	require.NoError(t, err)
	assert.False(t, second.Full, "a sync from a token is a delta")

	require.Len(t, second.Updated, 1, "only the changed contact comes back")
	assert.Equal(t, "c1", second.Updated[0].RemoteID)
	assert.Contains(t, second.Updated[0].VCardData, "Alice Renamed", "the card body is fetched via MultiGET")

	assert.Equal(t, []string{"c2"}, second.Deleted, "the deleted contact is named explicitly")
	assert.NotEqual(t, first.Cursor, second.Cursor, "the token advances")

	// Third sync: nothing changed.
	third, err := provider.ListChanges(ctx, second.Cursor)
	require.NoError(t, err)
	assert.Empty(t, third.Updated)
	assert.Empty(t, third.Deleted)
}

// A cursor the server rejects surfaces as ErrCursorExpired so the engine re-lists fully.
func TestCardDAVIncremental_BadCursorIsExpired(t *testing.T) {
	const email = "user2@example.com"
	const password = "correct-horse-battery-staple"

	ctx := context.Background()

	sqldb, err := sql.Open(sqliteshim.ShimName, "file:carddav_inc2?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.Migrate(ctx, db))

	userRepo := repository.NewBunUserRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)
	contactRepo := repository.NewBunContactRepository(db)
	appPwRepo := repository.NewBunAppPasswordRepository(db)

	authSvc := service.NewAuthService(userRepo, abRepo, config.AuthConfig{
		JWTSecret: "0123456789abcdef0123456789abcdef", TokenTTL: time.Hour,
	})
	_, err = authSvc.Register(ctx, email, password, "User")
	require.NoError(t, err)

	const prefix = "/dav"
	backend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, prefix)
	davServer := chqcarddav.NewServer(backend, userRepo, appPwRepo, prefix)
	app := fiber.New(fiber.Config{
		RequestMethods: append(fiber.DefaultMethods, "PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"),
	})
	app.Use(prefix, adaptor.HTTPHandler(davServer))
	httpServer := httptest.NewServer(adaptor.FiberApp(app))
	t.Cleanup(httpServer.Close)

	provider, err := chqsync.NewCardDAVClientProviderWithOptions(
		httpServer.URL+prefix+"/"+email+"/addressbooks/contacts/", email, password, false)
	require.NoError(t, err)

	_, err = provider.ListChanges(ctx, "urn:contactshq:sync:9999")
	assert.ErrorIs(t, err, chqsync.ErrCursorExpired)
}
