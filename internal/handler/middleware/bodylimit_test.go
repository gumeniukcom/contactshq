package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"github.com/gumeniukcom/contactshq/internal/handler/middleware"
)

func appWithLimit(limit int) *fiber.App {
	app := fiber.New()
	app.Use(middleware.BodyLimit(middleware.BodyLimits{Default: limit}))
	app.Post("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"received": len(c.Body())})
	})
	return app
}

func TestBodyLimit_AllowsABodyWithinTheLimit(t *testing.T) {
	app := appWithLimit(100)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), 50)))
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// The cheap path: an honest client announces the size and is turned away before the body is
// read at all.
func TestBodyLimit_RejectsOnDeclaredContentLength(t *testing.T) {
	app := appWithLimit(100)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), 500)))
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusRequestEntityTooLarge, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "exceeds")
	require.Contains(t, string(body), "100", "the message names the limit")
}

// The second check: a request that declares no size is measured after the body has been read.
// That is the check which costs the allocation it supposedly prevents, and the reason this
// middleware is documented as policy rather than protection.
//
// Driven through fasthttp directly: fiber's test harness always writes a Content-Length, so a
// request without one cannot be forged through app.Test.
func TestBodyLimit_RejectsABodyThatDeclaredNoLength(t *testing.T) {
	app := fiber.New()
	handler := middleware.BodyLimit(middleware.BodyLimits{Default: 100})

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.SetRequestURI("/")
	fctx.Request.Header.SetMethod(fiber.MethodPost)
	fctx.Request.SetBody(bytes.Repeat([]byte("x"), 500))
	// What a chunked request looks like from here: a body with no declared length.
	fctx.Request.Header.SetContentLength(-1)

	c := app.AcquireCtx(fctx)
	defer app.ReleaseCtx(c)

	require.NoError(t, handler(c))
	require.Equal(t, fiber.StatusRequestEntityTooLarge, c.Response().StatusCode(),
		"a body that hides its size must still be refused")
}

func TestBodyLimit_AllowsAnEmptyBody(t *testing.T) {
	app := appWithLimit(100)

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// A route group with its own, larger limit must override an outer one — that is what lets
// import accept an upload the rest of the API would refuse.
func TestBodyLimit_AnOverrideRaisesTheLimitForItsPath(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	api.Use(middleware.BodyLimit(middleware.BodyLimits{
		Default:   100,
		Overrides: map[string]int{"/import/": 1000},
	}))

	imp := api.Group("/import")
	imp.Post("/vcard", func(c *fiber.Ctx) error { return c.SendString("ok") })

	api.Post("/contacts", func(c *fiber.Ctx) error { return c.SendString("ok") })

	big := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bytes.Repeat([]byte("x"), 500)))
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	require.Equal(t, fiber.StatusOK, big("/api/import/vcard"),
		"import is the one place a large body is the point")
	require.Equal(t, fiber.StatusRequestEntityTooLarge, big("/api/contacts"),
		"an ordinary endpoint keeps the narrow limit")
}
