package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type DuplicateHandler struct {
	detector *service.DuplicateDetector
	merger   *service.MergeService
	dupRepo  repository.PotentialDuplicateRepository
}

func NewDuplicateHandler(
	detector *service.DuplicateDetector,
	merger *service.MergeService,
	dupRepo repository.PotentialDuplicateRepository,
) *DuplicateHandler {
	return &DuplicateHandler{detector: detector, merger: merger, dupRepo: dupRepo}
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
