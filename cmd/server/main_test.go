package main

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// An internal error must not reach the client verbatim: its text can name tables, file paths
// or hosts. It belongs in the log, and the client gets a fixed string.
func TestErrorHandler_InternalErrorIsNotDisclosed(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)

	app := fiber.New(fiber.Config{ErrorHandler: newErrorHandler(zap.New(core))})
	app.Get("/boom", func(_ *fiber.Ctx) error {
		return errors.New("pq: relation \"users\" does not exist at /var/lib/secret")
	})

	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("X-Request-ID", "req-42")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	require.NotContains(t, string(body), "relation")
	require.NotContains(t, string(body), "/var/lib/secret")
	// The body shape is part of the contract: web/src/api/client.ts reads {"error": "..."}.
	require.JSONEq(t, `{"error":"internal server error"}`, string(body))

	entries := logs.FilterMessage("unhandled request error").All()
	require.Len(t, entries, 1, "the cause must be logged")
	require.Contains(t, entries[0].ContextMap()["error"], "relation")
	require.Equal(t, "req-42", entries[0].ContextMap()["request_id"])
}

// A *fiber.Error carries a message this application chose, so it stays visible — otherwise
// every 404 and 413 would degrade into the same opaque string.
func TestErrorHandler_FiberErrorKeepsItsMessageAndStatus(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)

	app := fiber.New(fiber.Config{ErrorHandler: newErrorHandler(zap.New(core))})
	app.Get("/nope", func(_ *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "contact not found")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/nope", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	require.JSONEq(t, `{"error":"contact not found"}`, string(body))
	require.Zero(t, logs.Len(), "a deliberate 4xx is not an internal failure")
}

// Fiber's own 404 for an unrouted path travels the same handler and must keep its status.
func TestErrorHandler_UnroutedPathStays404(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: newErrorHandler(zap.NewNop())})

	resp, err := app.Test(httptest.NewRequest("GET", "/no/such/route", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}
