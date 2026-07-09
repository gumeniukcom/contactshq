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
