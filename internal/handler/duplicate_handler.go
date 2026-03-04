package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

type DuplicateHandler struct {
	detector         *service.DuplicateDetector
	merger           *service.MergeService
	dupRepo          repository.PotentialDuplicateRepository
	dedupSettingsRepo repository.UserDedupSettingsRepository
	scheduler        *worker.Scheduler
}

func NewDuplicateHandler(
	detector *service.DuplicateDetector,
	merger *service.MergeService,
	dupRepo repository.PotentialDuplicateRepository,
	dedupSettingsRepo repository.UserDedupSettingsRepository,
	scheduler *worker.Scheduler,
) *DuplicateHandler {
	return &DuplicateHandler{
		detector:          detector,
		merger:            merger,
		dupRepo:           dupRepo,
		dedupSettingsRepo: dedupSettingsRepo,
		scheduler:         scheduler,
	}
}

// List returns paginated potential duplicates for the authenticated user.
func (h *DuplicateHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	status := c.Query("status", "pending")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	dups, total, err := h.dupRepo.ListByUser(c.Context(), userID, status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch duplicates"})
	}
	if dups == nil {
		dups = []*domain.PotentialDuplicate{}
	}
	return c.JSON(fiber.Map{"duplicates": dups, "total": total})
}

// Count returns the number of pending duplicate pairs for the authenticated user.
func (h *DuplicateHandler) Count(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	count, err := h.dupRepo.CountPending(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to count duplicates"})
	}
	return c.JSON(fiber.Map{"pending": count})
}

// Detect runs the duplicate detection algorithm and returns how many new pairs were found.
func (h *DuplicateHandler) Detect(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	result, err := h.detector.Detect(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "detection failed"})
	}
	return c.JSON(result)
}

// Dismiss marks a potential duplicate as dismissed without merging.
func (h *DuplicateHandler) Dismiss(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	id := c.Params("id")

	dup, err := h.dupRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch duplicate"})
	}
	if dup == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "duplicate not found"})
	}
	if dup.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	dup.Status = "dismissed"
	if updateErr := h.dupRepo.Update(c.Context(), dup); updateErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to dismiss duplicate"})
	}
	return c.JSON(fiber.Map{"message": "dismissed"})
}

// Merge merges two contacts (winner keeps, loser is deleted).
func (h *DuplicateHandler) Merge(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var input service.MergeInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if input.WinnerID == "" || input.LoserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "winner_id and loser_id are required"})
	}

	merged, err := h.merger.Merge(c.Context(), userID, input)
	if err != nil {
		switch err {
		case service.ErrContactNotFound:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		case service.ErrSameContact:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "merge failed"})
		}
	}
	return c.JSON(merged)
}

// GetSettings returns the dedup schedule settings for the authenticated user.
func (h *DuplicateHandler) GetSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	s, err := h.dedupSettingsRepo.Get(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch settings"})
	}
	if s == nil {
		return c.JSON(domain.UserDedupSettings{
			UserID:   userID,
			Schedule: "0 2 * * *",
			Enabled:  false,
		})
	}
	return c.JSON(s)
}

// SaveSettings upserts dedup schedule settings and updates the scheduler.
func (h *DuplicateHandler) SaveSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var input struct {
		Schedule string `json:"schedule"`
		Enabled  bool   `json:"enabled"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if input.Enabled && input.Schedule != "" {
		if err := worker.ValidateCron(input.Schedule); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid cron expression"})
		}
	}

	s := &domain.UserDedupSettings{
		UserID:    userID,
		Schedule:  input.Schedule,
		Enabled:   input.Enabled,
		UpdatedAt: time.Now(),
	}
	if err := h.dedupSettingsRepo.Upsert(c.Context(), s); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save settings"})
	}

	if h.scheduler != nil {
		if s.Enabled && s.Schedule != "" {
			h.scheduler.ReregisterDedupForUser(s.Schedule, userID)
		} else {
			h.scheduler.RemoveDedupForUser(userID)
		}
	}

	return c.JSON(s)
}
