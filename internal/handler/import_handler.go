package handler

import (
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type ImportHandler struct {
	importerService *service.ImporterService
}

func NewImportHandler(importerService *service.ImporterService) *ImportHandler {
	return &ImportHandler{importerService: importerService}
}

func (h *ImportHandler) ImportVCard(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	file, err := c.FormFile("file")
	if err != nil {
		// Try reading body directly
		body := c.Body()
		if len(body) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no file or body provided"})
		}
		result, err := h.importerService.ImportVCard(c.Context(), userID, string(body))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "import failed"})
		}
		return c.JSON(result)
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file"})
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read file"})
	}

	result, err := h.importerService.ImportVCard(c.Context(), userID, string(data))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "import failed"})
	}

	return c.JSON(result)
}

func (h *ImportHandler) ImportCSV(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	file, err := c.FormFile("file")
	if err != nil {
		body := c.Body()
		if len(body) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no file or body provided"})
		}
		result, err := h.importerService.ImportCSV(c.Context(), userID, string(body))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "import failed"})
		}
		return c.JSON(result)
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file"})
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read file"})
	}

	result, err := h.importerService.ImportCSV(c.Context(), userID, string(data))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "import failed"})
	}

	return c.JSON(result)
}
