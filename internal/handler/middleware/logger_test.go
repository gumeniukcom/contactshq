package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gumeniukcom/contactshq/internal/handler/middleware"
)

func loggedApp(t *testing.T) (*fiber.App, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zap.InfoLevel)
	app := fiber.New()
	app.Use(middleware.RequestLogger(zap.New(core)))

	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/health-bad", func(c *fiber.Ctx) error {
		c.Path("/health") // pretend to be the health route while failing
		return c.Status(fiber.StatusServiceUnavailable).SendString("degraded")
	})
	app.Get("/thing", func(c *fiber.Ctx) error {
		return c.SendString(middleware.RequestIDFrom(c))
	})

	return app, logs
}

func get(t *testing.T, app *fiber.App, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp
}

// Without an id there is no way to connect "it failed at 14:32" to a line in the log.
func TestRequestLogger_EchoesASuppliedRequestID(t *testing.T) {
	app, logs := loggedApp(t)

	resp := get(t, app, "/thing", map[string]string{middleware.RequestIDHeader: "abc-123"})
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, "abc-123", resp.Header.Get(middleware.RequestIDHeader))

	entries := logs.FilterMessage("request").All()
	require.Len(t, entries, 1)
	require.Equal(t, "abc-123", entries[0].ContextMap()["request_id"])
}

func TestRequestLogger_MintsAnIDWhenNoneWasSupplied(t *testing.T) {
	app, logs := loggedApp(t)

	resp := get(t, app, "/thing", nil)
	defer func() { _ = resp.Body.Close() }()

	id := resp.Header.Get(middleware.RequestIDHeader)
	require.NotEmpty(t, id)
	require.Equal(t, id, logs.FilterMessage("request").All()[0].ContextMap()["request_id"])
}

// The id has to reach handlers, not just the log line.
func TestRequestLogger_MakesTheIDAvailableToHandlers(t *testing.T) {
	app, _ := loggedApp(t)

	resp := get(t, app, "/thing", map[string]string{middleware.RequestIDHeader: "from-proxy"})
	defer func() { _ = resp.Body.Close() }()

	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	require.Equal(t, "from-proxy", string(body[:n]))
}

// Docker polls /health every 30 seconds. Logging the successful ones buries everything else.
func TestRequestLogger_SkipsSuccessfulHealthChecks(t *testing.T) {
	app, logs := loggedApp(t)

	for i := 0; i < 5; i++ {
		resp := get(t, app, "/health", nil)
		_ = resp.Body.Close()
	}

	require.Zero(t, logs.FilterMessage("request").Len(),
		"a quiet container's log should not consist of health checks")
}

// A failing health check is exactly when the line matters.
func TestRequestLogger_LogsAFailingHealthCheck(t *testing.T) {
	app, logs := loggedApp(t)

	resp := get(t, app, "/health-bad", nil)
	_ = resp.Body.Close()

	require.Equal(t, 1, logs.FilterMessage("request").Len())
	require.Equal(t, int64(fiber.StatusServiceUnavailable),
		logs.FilterMessage("request").All()[0].ContextMap()["status"])
}

func TestRequestLogger_LogsOrdinaryRequests(t *testing.T) {
	app, logs := loggedApp(t)

	resp := get(t, app, "/thing", nil)
	_ = resp.Body.Close()

	require.Equal(t, 1, logs.FilterMessage("request").Len())
}
