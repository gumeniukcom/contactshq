package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"github.com/gumeniukcom/contactshq/internal/handler"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	return bun.NewDB(sqldb, sqlitedialect.New())
}

func getHealth(t *testing.T, svc handler.Services) (int, map[string]any) {
	t.Helper()

	app := fiber.New()
	handler.Register(app, svc)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	return resp.StatusCode, body
}

func TestHealth_ReportsDatabaseReachable(t *testing.T) {
	db := newTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	status, body := getHealth(t, handler.Services{Version: "1.2.3", BuildTime: "today", DB: db})

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["database"])
	assert.Equal(t, "1.2.3", body["version"])
	assert.Equal(t, "today", body["build_time"])
}

// A health check that only proves the process is alive told orchestrators everything was
// fine while every request failed against a database the server could not reach.
func TestHealth_ReportsDatabaseUnreachable(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	status, body := getHealth(t, handler.Services{DB: db})

	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, "degraded", body["status"])
	assert.Equal(t, "unreachable", body["database"])
}

func TestHealth_WithoutDatabaseStillReportsVersion(t *testing.T) {
	status, body := getHealth(t, handler.Services{Version: "dev"})

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "ok", body["status"])
	assert.NotContains(t, body, "database")
}

// The first thing an operator asks after an upgrade is which schema is actually applied.
func TestHealth_ReportsSchemaVersion(t *testing.T) {
	// One connection: with a plain ":memory:" pool the migration and the query land on
	// different databases, and schema_migrations would not exist for the reader.
	db := newMigratedTestDB(t)

	code, body := getHealth(t, handler.Services{DB: db})

	require.Equal(t, fiber.StatusOK, code)
	version, ok := body["schema_version"].(string)
	require.True(t, ok, "schema_version must be present: %v", body)

	want, err := repository.SchemaVersion(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, want, version)
	require.NotEmpty(t, version, "migrations have run, so there is a version")
}

// A full queue must not turn the health check red: the container would be restarted, and a
// restart is exactly what loses the queued jobs.
func TestHealth_QueueDepthDoesNotFailTheCheck(t *testing.T) {
	db := newMigratedTestDB(t)

	code, body := getHealth(t, handler.Services{DB: db, Worker: &fullQueueWorker{}})

	require.Equal(t, fiber.StatusOK, code)
	require.Equal(t, "ok", body["status"])
	require.Equal(t, float64(100), body["queue_depth"])
}

type fullQueueWorker struct{}

func (fullQueueWorker) Enqueue(context.Context, string, any) error { return nil }
func (fullQueueWorker) Start(context.Context) error                { return nil }
func (fullQueueWorker) Stop(context.Context) error                 { return nil }
func (fullQueueWorker) QueueDepth() int                            { return 100 }

// newMigratedTestDB returns a migrated in-memory database on a single connection.
func newMigratedTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.Migrate(context.Background(), db))
	return db
}
