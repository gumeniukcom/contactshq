package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type ExportHandler struct {
	exporterService *service.ExporterService
}

func NewExportHandler(exporterService *service.ExporterService) *ExportHandler {
	return &ExportHandler{exporterService: exporterService}
}

func (h *ExportHandler) ExportVCard(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	data, err := h.exporterService.ExportVCard(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "export failed"})
	}

	c.Set("Content-Type", "text/vcard; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=\"contacts.vcf\"")
	return c.SendString(data)
}

func (h *ExportHandler) ExportCSV(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	data, err := h.exporterService.ExportCSV(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "export failed"})
	}

	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=\"contacts.csv\"")
	return c.SendString(data)
}

func (h *ExportHandler) ExportJSON(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	data, err := h.exporterService.ExportJSON(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "export failed"})
	}

	c.Set("Content-Type", "application/json")
	c.Set("Content-Disposition", "attachment; filename=\"contacts.json\"")
	return c.SendString(data)
}
