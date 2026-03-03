package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

type BackupHandler struct {
	backupService *service.BackupService
	scheduler     *worker.Scheduler
}

func NewBackupHandler(backupService *service.BackupService, scheduler *worker.Scheduler) *BackupHandler {
	return &BackupHandler{backupService: backupService, scheduler: scheduler}
}

func (h *BackupHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	info, err := h.backupService.Create(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create backup"})
	}
	return c.Status(fiber.StatusCreated).JSON(info)
}

func (h *BackupHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	backups, err := h.backupService.List(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list backups"})
	}
	return c.JSON(fiber.Map{"backups": backups})
}

func (h *BackupHandler) Download(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	backupID := c.Params("id")
	path, err := h.backupService.GetPath(c.Context(), userID, backupID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "backup not found"})
	}
	return c.SendFile(path)
}

func (h *BackupHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	backupID := c.Params("id")
	if err := h.backupService.Delete(c.Context(), userID, backupID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "backup not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *BackupHandler) Restore(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	backupID := c.Params("id")
	mode := c.Query("mode", "merge")
	if mode != "merge" && mode != "replace" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "mode must be 'merge' or 'replace'"})
	}
	result, err := h.backupService.Restore(c.Context(), userID, backupID, mode)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

func (h *BackupHandler) GetSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	settings, err := h.backupService.GetSettings(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get settings"})
	}
	return c.JSON(settings)
}

func (h *BackupHandler) SaveSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var input domain.UserBackupSettings
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid input"})
	}

	if err := h.backupService.SaveSettings(c.Context(), userID, &input); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save settings"})
	}

	// Re-register scheduler with the new schedule.
	if h.scheduler != nil {
		if input.Enabled && input.Schedule != "" {
			h.scheduler.ReregisterBackupForUser(input.Schedule, userID)
		} else {
			h.scheduler.RemoveBackupForUser(userID)
		}
	}

	return c.JSON(&input)
}
