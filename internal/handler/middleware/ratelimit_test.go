package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func postN(t *testing.T, app *fiber.App, path string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestRateLimiterBlocksAfterMax(t *testing.T) {
	app := fiber.New()
	app.Post("/login", RateLimiter(3), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	for i := 0; i < 3; i++ {
		if got := postN(t, app, "/login"); got != fiber.StatusOK {
			t.Fatalf("request %d = %d, want 200", i+1, got)
		}
	}

	if got := postN(t, app, "/login"); got != fiber.StatusTooManyRequests {
		t.Fatalf("request 4 = %d, want 429", got)
	}
}

// Register and login are handed the same handler so that they gate one shared budget
// of argon2id work; refresh must not draw from it.
func TestRateLimiterInstanceSharesBucket(t *testing.T) {
	app := fiber.New()
	shared := RateLimiter(2)
	app.Post("/login", shared, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Post("/register", shared, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Post("/refresh", RateLimiter(2), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	if got := postN(t, app, "/login"); got != fiber.StatusOK {
		t.Fatalf("login = %d, want 200", got)
	}
	if got := postN(t, app, "/register"); got != fiber.StatusOK {
		t.Fatalf("register = %d, want 200", got)
	}
	// Third credential request across the shared pair exhausts the bucket.
	if got := postN(t, app, "/login"); got != fiber.StatusTooManyRequests {
		t.Fatalf("login after shared budget = %d, want 429", got)
	}

	// The independent limiter on refresh is untouched.
	if got := postN(t, app, "/refresh"); got != fiber.StatusOK {
		t.Fatalf("refresh = %d, want 200", got)
	}
}

func TestRateLimiterConstants(t *testing.T) {
	if CredentialRateLimit >= RefreshRateLimit {
		t.Fatalf("credential limit (%d) must be stricter than refresh limit (%d)",
			CredentialRateLimit, RefreshRateLimit)
	}
}

// postXFF sends a request through app with the given X-Forwarded-For and returns the status.
func postXFF(t *testing.T, app *fiber.App, xff string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// Behind a trusted proxy the limiter must key on the forwarded client, so two clients each
// get their own budget rather than sharing one.
func TestRateLimiter_KeysPerForwardedClientWhenProxyTrusted(t *testing.T) {
	// app.Test presents a remote IP of 0.0.0.0; trusting it makes the header believed.
	app := fiber.New(fiber.Config{
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"0.0.0.0"},
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableIPValidation:      true,
	})
	app.Post("/login", RateLimiter(2), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	// Client A exhausts its own budget.
	require2xx := func(client string) {
		if got := postXFF(t, app, client); got != fiber.StatusOK {
			t.Fatalf("client %s first = %d, want 200", client, got)
		}
		if got := postXFF(t, app, client); got != fiber.StatusOK {
			t.Fatalf("client %s second = %d, want 200", client, got)
		}
	}
	require2xx("203.0.113.1")
	if got := postXFF(t, app, "203.0.113.1"); got != fiber.StatusTooManyRequests {
		t.Fatalf("client A third = %d, want 429", got)
	}

	// A different client is unaffected: separate bucket.
	if got := postXFF(t, app, "203.0.113.2"); got != fiber.StatusOK {
		t.Fatalf("client B first = %d, want 200 — buckets must be per client", got)
	}
}

// A forwarded header from an untrusted peer must be ignored, so a client cannot dodge the
// limit by spoofing X-Forwarded-For.
func TestRateLimiter_IgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	// The test peer is 0.0.0.0, which is NOT in the trusted list, so the header is ignored.
	app := fiber.New(fiber.Config{
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"192.0.2.1"},
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableIPValidation:      true,
	})
	app.Post("/login", RateLimiter(2), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	// Every request keys on the real peer regardless of the spoofed header.
	_ = postXFF(t, app, "1.1.1.1")
	_ = postXFF(t, app, "2.2.2.2")
	if got := postXFF(t, app, "3.3.3.3"); got != fiber.StatusTooManyRequests {
		t.Fatalf("third spoofed request = %d, want 429 — spoofed X-Forwarded-For must not create new buckets", got)
	}
}
