package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/handler"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// newRegistrationApp wires the real auth stack over an in-memory database, which is what it
// takes to exercise the route order: /auth/config has to be reachable without a token, and
// /admin/users has to reach a different service method than /auth/register.
func newRegistrationApp(t *testing.T, allowRegistration bool) *fiber.App {
	t.Helper()

	// One connection only: with plain ":memory:" every pooled connection gets its own
	// database, so the migration and the queries would not see each other.
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, repository.Migrate(context.Background(), db))

	userRepo := repository.NewBunUserRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)

	authService := service.NewAuthService(userRepo, abRepo, config.AuthConfig{
		JWTSecret:         "0123456789abcdef0123456789abcdef",
		TokenTTL:          time.Hour,
		RefreshTTL:        24 * time.Hour,
		AllowRegistration: allowRegistration,
	})

	app := fiber.New()
	handler.Register(app, handler.Services{
		Auth: authService,
		User: service.NewUserService(userRepo),
		DB:   db,
	})
	return app
}

func postJSON(t *testing.T, app *fiber.App, path, token string, body any) (int, map[string]any) {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var out map[string]any
	_ = json.Unmarshal(payload, &out)
	return resp.StatusCode, out
}

func register(t *testing.T, app *fiber.App, email string) (int, map[string]any) {
	t.Helper()
	return postJSON(t, app, "/api/v1/auth/register", "", map[string]string{
		"email": email, "password": "correct-horse-battery", "display_name": "X",
	})
}

// The bootstrap account is created, the next public sign-up is refused.
func TestRegistrationPolicy_ClosedAfterBootstrap(t *testing.T) {
	app := newRegistrationApp(t, false)

	code, body := register(t, app, "owner@example.com")
	require.Equal(t, fiber.StatusCreated, code, "bootstrapping the first account must always work")
	user, _ := body["user"].(map[string]any)
	require.Equal(t, "admin", user["role"])

	code, _ = register(t, app, "intruder@example.com")
	require.Equal(t, fiber.StatusForbidden, code, "public sign-up must be closed once an owner exists")
}

func TestRegistrationPolicy_OpenWhenConfigured(t *testing.T) {
	app := newRegistrationApp(t, true)

	code, _ := register(t, app, "owner@example.com")
	require.Equal(t, fiber.StatusCreated, code)

	code, _ = register(t, app, "colleague@example.com")
	require.Equal(t, fiber.StatusCreated, code)
}

// /auth/config must sit before the JWT barrier: the login screen reads it with no token.
func TestRegistrationPolicy_ConfigIsPublic(t *testing.T) {
	app := newRegistrationApp(t, false)

	get := func() map[string]any {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, fiber.StatusOK, resp.StatusCode, "/auth/config must not require a token")

		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	require.Equal(t, true, get()["registration_open"], "an empty instance still accepts its first account")

	code, _ := register(t, app, "owner@example.com")
	require.Equal(t, fiber.StatusCreated, code)

	require.Equal(t, false, get()["registration_open"])
}

// POST /admin/users used to reuse authHandler.Register, so closing public sign-up would have
// broken the admin "create user" screen along with it.
func TestRegistrationPolicy_AdminCanCreateUsersWhileClosed(t *testing.T) {
	app := newRegistrationApp(t, false)

	code, _ := register(t, app, "owner@example.com")
	require.Equal(t, fiber.StatusCreated, code)

	code, body := postJSON(t, app, "/api/v1/auth/login", "", map[string]string{
		"email": "owner@example.com", "password": "correct-horse-battery",
	})
	require.Equal(t, fiber.StatusOK, code)
	token, _ := body["access_token"].(string)
	require.NotEmpty(t, token)

	code, body = postJSON(t, app, "/api/v1/admin/users", token, map[string]string{
		"email": "colleague@example.com", "password": "correct-horse-battery", "display_name": "Colleague",
	})
	require.Equal(t, fiber.StatusCreated, code,
		"an administrator must still be able to add users on a closed instance")
	user, _ := body["user"].(map[string]any)
	require.Equal(t, "user", user["role"])

	// And the public path is still shut for everyone else.
	code, _ = register(t, app, "intruder@example.com")
	require.Equal(t, fiber.StatusForbidden, code)
}

// A non-admin must not reach the bypass route.
func TestRegistrationPolicy_NonAdminCannotUseAdminCreate(t *testing.T) {
	app := newRegistrationApp(t, true)

	code, _ := register(t, app, "owner@example.com")
	require.Equal(t, fiber.StatusCreated, code)
	code, _ = register(t, app, "plain@example.com")
	require.Equal(t, fiber.StatusCreated, code)

	code, body := postJSON(t, app, "/api/v1/auth/login", "", map[string]string{
		"email": "plain@example.com", "password": "correct-horse-battery",
	})
	require.Equal(t, fiber.StatusOK, code)
	token, _ := body["access_token"].(string)

	code, _ = postJSON(t, app, "/api/v1/admin/users", token, map[string]string{
		"email": "sneaky@example.com", "password": "correct-horse-battery",
	})
	require.Equal(t, fiber.StatusForbidden, code)
}
