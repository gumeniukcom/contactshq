package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	"github.com/gumeniukcom/contactshq/internal/worker"
	"github.com/gumeniukcom/contactshq/internal/worker/jobs"
)

type SyncHandler struct {
	syncRunRepo      repository.SyncRunRepository
	syncStateRepo    repository.SyncStateRepository
	syncConflictRepo repository.SyncConflictRepository
	providerConnRepo repository.ProviderConnectionRepository
	conflictSvc      *service.SyncConflictService
	worker           worker.TaskWorker
	// endpointPolicy decides which provider URLs this deployment will fetch.
	endpointPolicy chqsync.EndpointPolicy
}

// WithEndpointPolicy sets what this handler will accept as a provider URL.
func (h *SyncHandler) WithEndpointPolicy(policy chqsync.EndpointPolicy) *SyncHandler {
	h.endpointPolicy = policy
	return h
}

func NewSyncHandler(
	syncRunRepo repository.SyncRunRepository,
	syncStateRepo repository.SyncStateRepository,
	syncConflictRepo repository.SyncConflictRepository,
	providerConnRepo repository.ProviderConnectionRepository,
	conflictSvc *service.SyncConflictService,
	w worker.TaskWorker,
) *SyncHandler {
	return &SyncHandler{
		syncRunRepo:      syncRunRepo,
		syncStateRepo:    syncStateRepo,
		syncConflictRepo: syncConflictRepo,
		providerConnRepo: providerConnRepo,
		conflictSvc:      conflictSvc,
		worker:           w,
	}
}

// ListProviders returns the user's configured sync connections.
func (h *SyncHandler) ListProviders(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	if h.providerConnRepo == nil {
		return c.JSON(fiber.Map{"providers": []fiber.Map{}})
	}

	connections, err := h.providerConnRepo.ListByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list providers"})
	}

	providers := make([]fiber.Map, 0, len(connections))
	for _, conn := range connections {
		providers = append(providers, fiber.Map{
			"id":            conn.ID,
			"provider_type": conn.ProviderType,
			"name":          conn.Name,
			"endpoint":      conn.Endpoint,
			"connected":     conn.Connected,
			"last_sync_at":  conn.LastSyncAt,
			"last_error":    conn.LastError,
			"created_at":    conn.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"providers": providers})
}

func (h *SyncHandler) GoogleConnect(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "Google OAuth2 integration not yet configured",
	})
}

func (h *SyncHandler) GoogleTrigger(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "Google sync not yet configured",
	})
}

type carddavConnectRequest struct {
	URL           string `json:"url"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}

// CardDAVConnect saves (or updates) a CardDAV connection for the user.
func (h *SyncHandler) CardDAVConnect(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req carddavConnectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := chqsync.ValidateProviderEndpoint(req.URL, h.endpointPolicy); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.providerConnRepo == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "provider store unavailable"})
	}

	// Upsert: one CardDAV connection per user (unique on user_id + provider_type).
	existing, _ := h.providerConnRepo.GetByUserAndType(c.Context(), userID, "carddav")
	now := time.Now()

	if existing != nil {
		existing.Endpoint = req.URL
		existing.Username = req.Username
		existing.Password = req.Password
		existing.SkipTLSVerify = req.SkipTLSVerify
		existing.Connected = true
		existing.LastError = ""
		existing.UpdatedAt = now
		if err := h.providerConnRepo.Update(c.Context(), existing); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save connection"})
		}
		return c.JSON(fiber.Map{"message": "CardDAV connection updated", "id": existing.ID})
	}

	conn := &domain.ProviderConnection{
		ID:            uuid.New().String(),
		UserID:        userID,
		ProviderType:  "carddav",
		Name:          "CardDAV Server",
		Endpoint:      req.URL,
		Username:      req.Username,
		Password:      req.Password,
		SkipTLSVerify: req.SkipTLSVerify,
		Connected:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.providerConnRepo.Create(c.Context(), conn); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save connection"})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "CardDAV server connected", "id": conn.ID})
}

// CardDAVTrigger enqueues a manual sync job. Credentials are loaded from the
// stored connection unless explicitly supplied in the request body.
func (h *SyncHandler) CardDAVTrigger(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req carddavConnectRequest
	_ = c.BodyParser(&req) // body is optional

	// The endpoint may arrive straight from the request body here and is used immediately,
	// leaving no stored row behind — so an attempt through this route is invisible after the
	// fact. It gets the same check as the stored one.
	if req.URL != "" {
		if err := chqsync.ValidateProviderEndpoint(req.URL, h.endpointPolicy); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if req.URL == "" && h.providerConnRepo != nil {
		conn, err := h.providerConnRepo.GetByUserAndType(c.Context(), userID, "carddav")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load connection"})
		}
		if conn == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no CardDAV connection configured"})
		}
		req.URL = conn.Endpoint
		req.Username = conn.Username
		req.Password = conn.Password
		req.SkipTLSVerify = conn.SkipTLSVerify
	}

	if err := chqsync.ValidateProviderEndpoint(req.URL, h.endpointPolicy); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	cfgBytes, _ := json.Marshal(map[string]any{
		"endpoint":        req.URL,
		"username":        req.Username,
		"password":        req.Password,
		"skip_tls_verify": req.SkipTLSVerify,
	})
	payload := jobs.SyncJobPayload{
		UserID:       userID,
		ProviderType: "carddav",
		Config:       string(cfgBytes),
	}
	if err := h.worker.Enqueue(c.Context(), "sync", payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to enqueue sync"})
	}
	return c.JSON(fiber.Map{"message": "sync job enqueued"})
}

// DisconnectProvider removes a saved provider connection.
func (h *SyncHandler) DisconnectProvider(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	id := c.Params("id")

	if h.providerConnRepo == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	conn, err := h.providerConnRepo.GetByID(c.Context(), id)
	if err != nil || conn == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "connection not found"})
	}
	if conn.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	if err := h.providerConnRepo.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to disconnect"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *SyncHandler) Status(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	runs, err := h.syncRunRepo.ListActiveByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch sync status"})
	}
	if runs == nil {
		runs = []*domain.SyncRun{}
	}
	return c.JSON(fiber.Map{"syncs": runs})
}

func (h *SyncHandler) History(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	runs, err := h.syncRunRepo.ListByUser(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch sync history"})
	}
	if runs == nil {
		return c.JSON(fiber.Map{"history": []fiber.Map{}})
	}
	return c.JSON(fiber.Map{"history": runs})
}

func (h *SyncHandler) ListConflicts(c *fiber.Ctx) error {
	if h.syncConflictRepo == nil {
		return c.JSON(fiber.Map{"conflicts": []*domain.SyncConflict{}, "total": 0})
	}
	userID := c.Locals("userID").(string)
	status := c.Query("status", "")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	conflicts, total, err := h.syncConflictRepo.ListByUser(c.Context(), userID, status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch conflicts"})
	}
	if conflicts == nil {
		conflicts = []*domain.SyncConflict{}
	}
	return c.JSON(fiber.Map{"conflicts": conflicts, "total": total})
}

func (h *SyncHandler) CountConflicts(c *fiber.Ctx) error {
	if h.syncConflictRepo == nil {
		return c.JSON(fiber.Map{"pending": 0})
	}
	userID := c.Locals("userID").(string)
	count, err := h.syncConflictRepo.CountPending(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to count conflicts"})
	}
	return c.JSON(fiber.Map{"pending": count})
}

func (h *SyncHandler) GetConflict(c *fiber.Ctx) error {
	if h.syncConflictRepo == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	userID := c.Locals("userID").(string)
	id := c.Params("id")
	conflict, err := h.syncConflictRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conflict not found"})
	}
	if conflict.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.JSON(conflict)
}

type resolveConflictRequest struct {
	Resolution map[string]string `json:"resolution"`
}

func (h *SyncHandler) ResolveConflict(c *fiber.Ctx) error {
	if h.conflictSvc == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	userID := c.Locals("userID").(string)

	var req resolveConflictRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	conflict, err := h.conflictSvc.Resolve(c.Context(), userID, c.Params("id"), req.Resolution)
	if err != nil {
		return conflictError(c, err)
	}

	return c.JSON(fiber.Map{"message": "conflict resolved", "resolved_vcard": conflict.ResolvedVCard})
}

// conflictError maps the service's sentinel errors onto HTTP statuses.
func conflictError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrConflictNotFound), errors.Is(err, service.ErrConflictContactGone):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrConflictForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	case errors.Is(err, service.ErrConflictNotPending):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to resolve conflict"})
	}
}

func (h *SyncHandler) DismissConflict(c *fiber.Ctx) error {
	if h.conflictSvc == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	userID := c.Locals("userID").(string)

	if err := h.conflictSvc.Dismiss(c.Context(), userID, c.Params("id")); err != nil {
		return conflictError(c, err)
	}
	return c.JSON(fiber.Map{"message": "conflict dismissed"})
}
