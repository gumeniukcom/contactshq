package middleware

import (
	"github.com/gofiber/fiber/v2"
)

func AdminOnly() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
		}
		return c.Next()
	}
}
