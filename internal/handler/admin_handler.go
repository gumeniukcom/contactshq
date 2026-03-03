package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type AdminHandler struct {
	userService *service.UserService
}

func NewAdminHandler(userService *service.UserService) *AdminHandler {
	return &AdminHandler{userService: userService}
}

func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	users, total, err := h.userService.List(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list users"})
	}

	return c.JSON(fiber.Map{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (h *AdminHandler) UpdateUserRole(c *fiber.Ctx) error {
	userID := c.Params("id")

	var req updateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Role != "user" && req.Role != "admin" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be 'user' or 'admin'"})
	}

	if err := h.userService.UpdateRole(c.Context(), userID, req.Role); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update role"})
	}

	return c.JSON(fiber.Map{"message": "role updated"})
}

func (h *AdminHandler) DeleteUser(c *fiber.Ctx) error {
	userID := c.Params("id")

	if err := h.userService.Delete(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete user"})
	}

	return c.JSON(fiber.Map{"message": "user deleted"})
}
