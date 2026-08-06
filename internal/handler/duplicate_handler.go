package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

type DuplicateHandler struct {
	detector          *service.DuplicateDetector
	merger            *service.MergeService
	dupRepo           repository.PotentialDuplicateRepository
	dedupSettingsRepo repository.UserDedupSettingsRepository
	scheduler         *worker.Scheduler
	mergeLogRepo      repository.MergeLogRepository
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

// WithMergeLog enables the merge history endpoint. Optional: without it the route reports an
// empty history rather than failing.
func (h *DuplicateHandler) WithMergeLog(repo repository.MergeLogRepository) *DuplicateHandler {
	h.mergeLogRepo = repo
	return h
}

// MergeLog returns the user's recent merges, newest first.
func (h *DuplicateHandler) MergeLog(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	if h.mergeLogRepo == nil {
		return c.JSON(fiber.Map{"entries": []any{}})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	entries, err := h.mergeLogRepo.ListByUser(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list merges"})
	}
	if entries == nil {
		entries = []*domain.MergeLogEntry{}
	}
	return c.JSON(fiber.Map{"entries": entries})
}

// maxDuplicatePageSize bounds a page of pairs.
const maxDuplicatePageSize = 100

// List returns paginated potential duplicates for the authenticated user.
func (h *DuplicateHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	// fiber's Query(key, default) substitutes the default for an *empty* value too, so a
	// client clearing the filter with status= got "pending" back and could never see
	// dismissed pairs. "all" is the explicit way to ask for everything.
	status := c.Query("status", "pending")
	if status == "" {
		status = repository.StatusAll
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	// Over-large requests are clamped to the maximum, not reset to the minimum: asking for
	// 200 used to yield 20, which is how a pair past the twentieth became unreachable.
	if limit > maxDuplicatePageSize {
		limit = maxDuplicatePageSize
	}
	if limit <= 0 {
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

// Get returns one duplicate pair with both contacts and all of their values, which is what
// the merge screen needs and what the list deliberately does not carry.
func (h *DuplicateHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	id := c.Params("id")

	dup, err := h.dupRepo.GetByIDWithContacts(c.Context(), userID, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch duplicate"})
	}
	if dup == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "duplicate not found"})
	}
	// The query already filters on user_id; this is belt and braces for a future caller that
	// reaches for a different loader.
	if dup.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	// Value identifiers are minted here, not on the client. They are content hashes, and a
	// second implementation in TypeScript would have to agree with this one byte for byte
	// forever; the merge screen selects by id, so a disagreement would silently drop values.
	var candidates []vcardpkg.ValueRef
	if dup.ContactA != nil && dup.ContactB != nil {
		candidates, err = vcardpkg.Candidates(dup.ContactA.VCardData, dup.ContactB.VCardData)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read contact data"})
		}
	}
	if candidates == nil {
		candidates = []vcardpkg.ValueRef{}
	}

	return c.JSON(fiber.Map{"duplicate": dup, "candidates": candidates})
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
		// The scheduled scan and this one would both walk the whole address book; saying so
		// is more useful than running it twice.
		if errors.Is(err, service.ErrDetectionInProgress) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "duplicate detection is already running for this account",
			})
		}
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
