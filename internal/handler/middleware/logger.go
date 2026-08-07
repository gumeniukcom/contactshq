package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RequestIDHeader is where a request id is read from and echoed back.
const RequestIDHeader = "X-Request-Id"

// RequestIDKey is where the id is stored for handlers to pick up.
const RequestIDKey = "requestID"

// healthPath is polled by the container health check every 30 seconds.
const healthPath = "/health"

// RequestLogger logs one line per request, with an id that ties it to a report.
//
// Two deliberate behaviours:
//
//   - Every request gets an id — reused from X-Request-Id when a proxy supplied one, minted
//     otherwise — which is put in the response header and in c.Locals. Without it there was
//     no way to connect "the app failed at 14:32" to a line in the log.
//   - A *successful* health check is not logged. Docker polls /health every 30 seconds, so
//     an idle container's log was nothing but health checks, and a real line was buried among
//     thousands. A failing one is always logged: that is exactly when it matters.
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		requestID := c.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Locals(RequestIDKey, requestID)
		c.Set(RequestIDHeader, requestID)

		err := c.Next()

		status := c.Response().StatusCode()
		if c.Path() == healthPath && status < 400 {
			return err
		}

		logger.Info("request",
			zap.String("request_id", requestID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.IP()),
		)
		return err
	}
}

// RequestIDFrom returns the id assigned to this request, if the logger ran.
func RequestIDFrom(c *fiber.Ctx) string {
	if id, ok := c.Locals(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
