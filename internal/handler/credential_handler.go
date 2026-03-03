package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

type CredentialHandler struct {
	repo repository.ProviderConnectionRepository
}

func NewCredentialHandler(repo repository.ProviderConnectionRepository) *CredentialHandler {
	return &CredentialHandler{repo: repo}
}

type credentialRequest struct {
	Name          string `json:"name"`
	ProviderType  string `json:"provider_type"`
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}

// List returns all saved credentials for the authenticated user (passwords omitted via json:"-").
func (h *CredentialHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	conns, err := h.repo.ListByUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list credentials"})
	}
	if conns == nil {
		conns = []*domain.ProviderConnection{}
	}
	return c.JSON(fiber.Map{"credentials": conns})
}

// Create saves a new credential profile.
func (h *CredentialHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req credentialRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.ProviderType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider_type is required"})
	}
	if req.Endpoint == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "endpoint is required"})
	}
	if req.Name == "" {
		req.Name = req.ProviderType + " server"
	}

	now := time.Now()
	conn := &domain.ProviderConnection{
		ID:            uuid.New().String(),
		UserID:        userID,
		ProviderType:  req.ProviderType,
		Name:          req.Name,
		Endpoint:      req.Endpoint,
		Username:      req.Username,
		Password:      req.Password,
		SkipTLSVerify: req.SkipTLSVerify,
		Connected:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.repo.Create(c.Context(), conn); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create credential"})
	}
	return c.Status(fiber.StatusCreated).JSON(conn)
}

// Get returns a single credential (no password).
func (h *CredentialHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	conn, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil || conn == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if conn.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.JSON(conn)
}

// Update modifies an existing credential. Password is only updated when non-empty.
func (h *CredentialHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	conn, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil || conn == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if conn.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	var req credentialRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	conn.Name = req.Name
	conn.Endpoint = req.Endpoint
	conn.Username = req.Username
	conn.SkipTLSVerify = req.SkipTLSVerify
	if req.Password != "" {
		conn.Password = req.Password
	}
	conn.UpdatedAt = time.Now()

	if err := h.repo.Update(c.Context(), conn); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update credential"})
	}
	return c.JSON(conn)
}

// Delete removes a credential profile.
func (h *CredentialHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	conn, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil || conn == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if conn.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	if err := h.repo.Delete(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete credential"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
