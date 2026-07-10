package handler_test

import (
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
