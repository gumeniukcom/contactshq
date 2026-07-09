package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// Requests allowed per minute, per client IP.
const (
	// Login and register each cost a 64 MiB argon2id hash, making them both a
	// brute-force target and a memory-exhaustion vector.
	CredentialRateLimit = 10

	// Refreshing only verifies an HMAC signature, so it is cheap and not
	// brute-forceable. It is limited generously, to bound abuse rather than guessing.
	RefreshRateLimit = 60
)

// RateLimiter throttles a route by client IP over a one-minute window.
//
// Behind a reverse proxy every request carries the proxy's address unless the Fiber
// app is configured with ProxyHeader, in which case the bucket is shared by all
// clients. The limits above are chosen to stay clear of normal use even then.
//
// The returned handler owns its counter: pass one instance to several routes to have
// them share a bucket, or call RateLimiter again for an independent one.
func RateLimiter(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too many attempts, try again later",
			})
		},
	})
}
