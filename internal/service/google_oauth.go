package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const googleContactsScope = "https://www.googleapis.com/auth/contacts"

// GoogleOAuthService handles OAuth2 flows for Google People API.
type GoogleOAuthService struct {
	cfg      config.GoogleConfig
	connRepo repository.ProviderConnectionRepository
}

func NewGoogleOAuthService(cfg config.GoogleConfig, connRepo repository.ProviderConnectionRepository) *GoogleOAuthService {
	return &GoogleOAuthService{cfg: cfg, connRepo: connRepo}
}

// GetAuthURL creates a pending ProviderConnection and returns the Google OAuth2 authorization URL.
// The stateToken is the connection ID used to match the callback.
func (s *GoogleOAuthService) GetAuthURL(ctx context.Context, userID, clientID, clientSecret, redirectURL string) (authURL, stateToken string, err error) {
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("client_id and client_secret are required")
	}
	if redirectURL == "" {
		redirectURL = s.cfg.RedirectURL
	}
	if redirectURL == "" {
		return "", "", fmt.Errorf("redirect_url is required")
	}

	// Generate PKCE code verifier (43-128 chars, RFC 7636)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// code_challenge = BASE64URL(SHA256(code_verifier))
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	connID := uuid.New().String()
	now := time.Now()

	conn := &domain.ProviderConnection{
		ID:           connID,
		UserID:       userID,
		ProviderType: "google",
		Name:         "Google Contacts",
		Connected:    false,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       googleContactsScope,
		// Store redirect URL so HandleCallback uses the exact same value
		Endpoint: redirectURL,
		// Store code_verifier in Password field temporarily (reused for PKCE exchange)
		Password:  codeVerifier,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Delete any existing pending (disconnected) Google connection for this user
	// so we don't accumulate stale OAuth state records.
	if existing, _ := s.connRepo.GetByUserAndType(ctx, userID, "google"); existing != nil && !existing.Connected {
		_ = s.connRepo.Delete(ctx, existing.ID)
	}

	if err := s.connRepo.Create(ctx, conn); err != nil {
		return "", "", fmt.Errorf("create pending connection: %w", err)
	}

	// Bun ORM skips zero-value bools on INSERT when the column has default:true,
	// so the row gets connected=true instead of false. Force it to false.
	if err := s.connRepo.SetConnected(ctx, connID, false); err != nil {
		return "", "", fmt.Errorf("mark connection pending: %w", err)
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{googleContactsScope},
	}

	url := oauthCfg.AuthCodeURL(
		connID,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	return url, connID, nil
}

// HandleCallback exchanges the authorization code for tokens and activates the connection.
func (s *GoogleOAuthService) HandleCallback(ctx context.Context, stateToken, code string) (*domain.ProviderConnection, error) {
	conn, err := s.connRepo.GetByID(ctx, stateToken)
	if err != nil {
		return nil, fmt.Errorf("lookup connection: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("invalid state token")
	}
	if conn.Connected {
		return conn, nil // already processed
	}

	codeVerifier := conn.Password

	// Use the exact redirect URL that was used during GetAuthURL (stored in Endpoint)
	redirectURL := conn.Endpoint
	if redirectURL == "" {
		redirectURL = s.cfg.RedirectURL
	}
	oauthCfg := &oauth2.Config{
		ClientID:     conn.ClientID,
		ClientSecret: conn.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{googleContactsScope},
	}

	token, err := oauthCfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("exchange code for token: %w", err)
	}

	conn.AccessToken = token.AccessToken
	conn.RefreshToken = token.RefreshToken
	expiry := token.Expiry
	conn.TokenExpiry = &expiry
	conn.Connected = true
	conn.Password = "" // clear PKCE verifier
	conn.LastError = ""
	conn.UpdatedAt = time.Now()

	if err := s.connRepo.Update(ctx, conn); err != nil {
		return nil, fmt.Errorf("save tokens: %w", err)
	}

	return conn, nil
}

// GetHTTPClient returns an http.Client that auto-refreshes tokens using the stored
// refresh token. Refreshed tokens are persisted back to the database.
func (s *GoogleOAuthService) GetHTTPClient(ctx context.Context, conn *domain.ProviderConnection) (*http.Client, error) {
	if conn.AccessToken == "" && conn.RefreshToken == "" {
		return nil, fmt.Errorf("connection %s has no tokens", conn.ID)
	}

	oauthCfg := &oauth2.Config{
		ClientID:     conn.ClientID,
		ClientSecret: conn.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{googleContactsScope},
	}

	var expiry time.Time
	if conn.TokenExpiry != nil {
		expiry = *conn.TokenExpiry
	}

	token := &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		Expiry:       expiry,
		TokenType:    "Bearer",
	}

	// Wrap the token source to persist refreshed tokens
	baseSource := oauthCfg.TokenSource(ctx, token)
	persistSource := &persistingTokenSource{
		base:      baseSource,
		connRepo:  s.connRepo,
		connID:    conn.ID,
		lastToken: token.AccessToken,
	}

	return oauth2.NewClient(ctx, persistSource), nil
}

// RevokeToken revokes the OAuth2 token at Google and deletes the connection.
func (s *GoogleOAuthService) RevokeToken(ctx context.Context, conn *domain.ProviderConnection) error {
	// Best-effort revocation — don't fail if Google is unreachable
	if conn.RefreshToken != "" {
		revokeURL := "https://oauth2.googleapis.com/revoke?token=" + conn.RefreshToken
		resp, err := http.Post(revokeURL, "application/x-www-form-urlencoded", nil) //nolint:noctx
		if err == nil {
			resp.Body.Close()
		}
	}
	return s.connRepo.Delete(ctx, conn.ID)
}

// persistingTokenSource wraps an oauth2.TokenSource and persists refreshed tokens.
type persistingTokenSource struct {
	base      oauth2.TokenSource
	connRepo  repository.ProviderConnectionRepository
	connID    string
	lastToken string
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}

	// Persist if the access token changed (was refreshed)
	if token.AccessToken != s.lastToken {
		s.lastToken = token.AccessToken
		expiry := token.Expiry
		_ = s.connRepo.UpdateToken(context.Background(), s.connID, token.AccessToken, token.RefreshToken, &expiry)
	}

	return token, nil
}
