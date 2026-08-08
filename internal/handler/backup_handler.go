package handler

import (
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

type BackupHandler struct {
	backupService *service.BackupService
	scheduler     *worker.Scheduler
	logger        *zap.Logger
	runRepo       repository.BackupRunRepository
}

func NewBackupHandler(backupService *service.BackupService, scheduler *worker.Scheduler, logger *zap.Logger) *BackupHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BackupHandler{backupService: backupService, scheduler: scheduler, logger: logger}
}

// WithRunRepo enables the history endpoints.
func (h *BackupHandler) WithRunRepo(repo repository.BackupRunRepository) *BackupHandler {
	h.runRepo = repo
	return h
}

// maxBackupRunPageSize bounds a page of history.
const maxBackupRunPageSize = 200

// Runs returns the user's backup history, newest first.
func (h *BackupHandler) Runs(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	if h.runRepo == nil {
		return c.JSON(fiber.Map{"runs": []any{}})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit > maxBackupRunPageSize {
		limit = maxBackupRunPageSize
	}
	if limit <= 0 {
		limit = 50
	}

	runs, err := h.runRepo.ListByUser(c.Context(), userID, limit)
	if err != nil {
		h.logger.Error("failed to list backup runs", zap.String("user_id", userID), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list backup runs"})
	}
	if runs == nil {
		runs = []*domain.BackupRun{}
	}
	return c.JSON(fiber.Map{"runs": runs})
}

// Status answers "is my backup working?" in one request.
//
// Deliberately behind authentication and per user: /health is registered on the app rather
// than under the JWT barrier, so putting any of this there would publish one user's backup
// state to anyone who can reach the port.
func (h *BackupHandler) Status(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	body := fiber.Map{"last_success": nil, "last_run": nil, "next_run": nil}

	if h.runRepo != nil {
		lastSuccess, err := h.runRepo.LastSuccess(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read backup status"})
		}
		lastRun, err := h.runRepo.LastRun(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read backup status"})
		}
		body["last_success"] = lastSuccess
		body["last_run"] = lastRun
	}

	if h.scheduler != nil {
		if next, ok := h.scheduler.NextBackupRun(userID); ok {
			body["next_run"] = next
		}
	}

	return c.JSON(body)
}

func (h *BackupHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	info, err := h.backupService.Create(c.Context(), userID)
	if err != nil {
		// The generic 500 stays, but the cause has to land somewhere: a manual backup that
		// fails left no trace at all in the logs before this.
		h.logger.Error("manual backup failed", zap.String("user_id", userID), zap.Error(err))
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
	// Same shape as the export endpoints. The name is safe to interpolate because it is one
	// this service wrote: isBackupFilename only checks the suffix, but GetPath then stats the
	// file inside the user's own directory, so a name carrying a quote or a CR does not exist.
	// That guarantee ends the moment the backup directory becomes writable by anything else.
	c.Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
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

	if input.Enabled && input.Schedule != "" {
		if err := worker.ValidateCron(input.Schedule); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid cron expression"})
		}
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
