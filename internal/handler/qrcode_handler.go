package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type QRCodeHandler struct {
	qrcodeService  *service.QRCodeService
	contactService *service.ContactService
}

func NewQRCodeHandler(qrcodeService *service.QRCodeService, contactService *service.ContactService) *QRCodeHandler {
	return &QRCodeHandler{
		qrcodeService:  qrcodeService,
		contactService: contactService,
	}
}

func (h *QRCodeHandler) GenerateQR(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	contactID := c.Params("id")
	size, _ := strconv.Atoi(c.Query("size", "256"))

	contact, err := h.contactService.GetByID(c.Context(), userID, contactID)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get contact"})
	}

	png, err := h.qrcodeService.GenerateVCardQR(contact, size)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate QR code"})
	}

	c.Set("Content-Type", "image/png")
	return c.Send(png)
}
