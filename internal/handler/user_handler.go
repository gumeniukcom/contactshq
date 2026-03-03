package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) GetMe(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	user, err := h.userService.GetByID(c.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get user"})
	}

	return c.JSON(user)
}

type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (h *UserHandler) UpdateMe(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req updateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	user, err := h.userService.UpdateProfile(c.Context(), userID, req.DisplayName, req.Email)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already taken"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update profile"})
	}

	return c.JSON(user)
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *UserHandler) ChangePassword(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "old_password and new_password are required"})
	}

	if len(req.NewPassword) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new password must be at least 8 characters"})
	}

	err := h.userService.ChangePassword(c.Context(), userID, req.OldPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "incorrect old password"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to change password"})
	}

	return c.JSON(fiber.Map{"message": "password changed"})
}

func (h *UserHandler) DeleteMe(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	if err := h.userService.Delete(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete account"})
	}

	return c.JSON(fiber.Map{"message": "account deleted"})
}
