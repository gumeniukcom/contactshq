package middleware

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// BodyLimits describes how large a request body may be, per route.
type BodyLimits struct {
	// Default applies to everything without a more specific rule.
	Default int
	// Overrides maps a path prefix, relative to the group the middleware is mounted on, to
	// its own limit. Import is the reason this exists: a large body is the point there.
	Overrides map[string]int
}

// BodyLimit enforces a per-route body size.
//
// # What this is and is not
//
// This is a POLICY, not a memory guard. Fiber's own BodyLimit is
// `app.server.MaxRequestBodySize` — one number for the whole application, not settable per
// route — and fasthttp reads the entire body into memory before a handler ever runs. By the
// time `len(c.Body())` can be measured the allocation has already happened, and the
// Content-Length check below is skippable with a chunked request.
//
// So the only thing that actually bounds memory is the global limit, and the only thing this
// buys is a clear 413 with a useful message on routes where a large body is meaningless. That
// is worth having — a contact JSON has no business being 30 MB — but it must not be mistaken
// for a defence.
//
// The practical consequence: `server.max_body_bytes` should be set to what import genuinely
// needs and no more. With N concurrent uploads the worst case is N × max_body_bytes resident.
//
// # Why one middleware rather than one per group
//
// Mounting a narrow limit on a parent group and a wider one on a child does not work: both
// run, the parent runs first, and its 413 wins — so the child's larger allowance is
// unreachable. Resolving the limit per request keeps the intent in one place and removes the
// ordering trap.
func BodyLimit(limits BodyLimits) fiber.Handler {
	return func(c *fiber.Ctx) error {
		maxBytes := limitFor(c, limits)
		if maxBytes <= 0 {
			return c.Next()
		}

		// Cheap path: an honest client announces the size and is turned away before the body
		// is read at all.
		if declared := c.Request().Header.ContentLength(); declared > maxBytes {
			return tooLarge(c, maxBytes)
		}
		// A chunked request declares nothing, so the body is measured after the fact. This is
		// the check that costs the allocation it is supposedly preventing.
		if len(c.Body()) > maxBytes {
			return tooLarge(c, maxBytes)
		}
		return c.Next()
	}
}

// limitFor picks the most specific limit for the request's path.
func limitFor(c *fiber.Ctx, limits BodyLimits) int {
	path := c.Path()
	best, bestLen := limits.Default, -1

	for prefix, limit := range limits.Overrides {
		if len(prefix) > bestLen && strings.Contains(path, prefix) {
			best, bestLen = limit, len(prefix)
		}
	}
	return best
}

func tooLarge(c *fiber.Ctx, maxBytes int) error {
	return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
		"error": fmt.Sprintf("request body exceeds the %d byte limit for this endpoint", maxBytes),
	})
}
