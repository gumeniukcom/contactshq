package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type AppPasswordHandler struct {
	svc *service.AppPasswordService
}

func NewAppPasswordHandler(svc *service.AppPasswordService) *AppPasswordHandler {
	return &AppPasswordHandler{svc: svc}
}

type createAppPasswordRequest struct {
	Label string `json:"label"`
}

func (h *AppPasswordHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req createAppPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "label is required"})
	}

	token, ap, err := h.svc.Create(c.Context(), userID, req.Label)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create app password"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         ap.ID,
		"label":      ap.Label,
		"token":      token,
		"created_at": ap.CreatedAt,
	})
}

func (h *AppPasswordHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	passwords, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list app passwords"})
	}

	return c.JSON(fiber.Map{"app_passwords": passwords})
}

func (h *AppPasswordHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	id := c.Params("id")

	if err := h.svc.Delete(c.Context(), userID, id); err != nil {
		if err == service.ErrAppPasswordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "app password not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete app password"})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
