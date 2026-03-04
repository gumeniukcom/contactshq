package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type GoogleHandler struct {
	oauth    *service.GoogleOAuthService
	connRepo repository.ProviderConnectionRepository
}

func NewGoogleHandler(oauth *service.GoogleOAuthService, connRepo repository.ProviderConnectionRepository) *GoogleHandler {
	return &GoogleHandler{oauth: oauth, connRepo: connRepo}
}

type googleInitRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
}

// InitAuth saves the user's Google OAuth client credentials and returns the authorization URL.
func (h *GoogleHandler) InitAuth(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var req googleInitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.ClientID == "" || req.ClientSecret == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client_id and client_secret are required"})
	}

	// Auto-detect redirect URL from request if not provided
	redirectURL := req.RedirectURL
	if redirectURL == "" {
		proto := c.Protocol()
		host := c.Hostname()
		redirectURL = proto + "://" + host + "/api/v1/auth/google/callback"
	}

	authURL, _, err := h.oauth.GetAuthURL(c.Context(), userID, req.ClientID, req.ClientSecret, redirectURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"auth_url": authURL})
}

// Callback handles the Google OAuth2 redirect.
func (h *GoogleHandler) Callback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		errMsg := c.Query("error", "missing code or state")
		return c.Redirect("/app/settings/google?error="+url.QueryEscape(errMsg), fiber.StatusTemporaryRedirect)
	}

	_, err := h.oauth.HandleCallback(c.Context(), state, code)
	if err != nil {
		return c.Redirect("/app/settings/google?error="+url.QueryEscape(err.Error()), fiber.StatusTemporaryRedirect)
	}

	return c.Redirect("/app/settings/google?connected=true", fiber.StatusTemporaryRedirect)
}

// Status returns the current Google connection status for the user.
func (h *GoogleHandler) Status(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	conn, err := h.connRepo.GetByUserAndType(c.Context(), userID, "google")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check status"})
	}

	if conn == nil || !conn.Connected || (conn.AccessToken == "" && conn.RefreshToken == "") {
		return c.JSON(fiber.Map{"connected": false})
	}

	resp := fiber.Map{
		"connected":  true,
		"name":       conn.Name,
		"created_at": conn.CreatedAt,
	}
	if conn.LastSyncAt != nil {
		resp["last_sync_at"] = conn.LastSyncAt
	}
	if conn.LastError != "" {
		resp["last_error"] = conn.LastError
	}
	return c.JSON(resp)
}

// Disconnect revokes the Google OAuth token and removes the connection.
func (h *GoogleHandler) Disconnect(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	conn, err := h.connRepo.GetByUserAndType(c.Context(), userID, "google")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to lookup connection"})
	}
	if conn == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not connected"})
	}

	if err := h.oauth.RevokeToken(c.Context(), conn); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
