package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnRepo implements repository.ProviderConnectionRepository for testing.
type mockConnRepo struct {
	connections map[string]*domain.ProviderConnection
}

func newMockConnRepo() *mockConnRepo {
	return &mockConnRepo{connections: make(map[string]*domain.ProviderConnection)}
}

func (r *mockConnRepo) Create(_ context.Context, c *domain.ProviderConnection) error {
	r.connections[c.ID] = c
	return nil
}

func (r *mockConnRepo) GetByID(_ context.Context, id string) (*domain.ProviderConnection, error) {
	c, ok := r.connections[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (r *mockConnRepo) ListByUser(_ context.Context, userID string) ([]*domain.ProviderConnection, error) {
	var result []*domain.ProviderConnection
	for _, c := range r.connections {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *mockConnRepo) GetByUserAndType(_ context.Context, userID, providerType string) (*domain.ProviderConnection, error) {
	for _, c := range r.connections {
		if c.UserID == userID && c.ProviderType == providerType {
			return c, nil
		}
	}
	return nil, nil
}

func (r *mockConnRepo) Update(_ context.Context, c *domain.ProviderConnection) error {
	r.connections[c.ID] = c
	return nil
}

func (r *mockConnRepo) Delete(_ context.Context, id string) error {
	delete(r.connections, id)
	return nil
}

func (r *mockConnRepo) UpdateToken(_ context.Context, _, _, _ string, _ *time.Time) error {
	return nil
}

func (r *mockConnRepo) SetConnected(_ context.Context, id string, connected bool) error {
	if c, ok := r.connections[id]; ok {
		c.Connected = connected
	}
	return nil
}

func TestGetAuthURL_Success(t *testing.T) {
	repo := newMockConnRepo()
	cfg := config.GoogleConfig{
		RedirectURL: "http://localhost:8080/api/v1/auth/google/callback",
	}
	svc := service.NewGoogleOAuthService(cfg, repo)

	authURL, stateToken, err := svc.GetAuthURL(context.Background(), "user1", "test-client-id", "test-secret", "")
	require.NoError(t, err)
	assert.NotEmpty(t, authURL)
	assert.NotEmpty(t, stateToken)
	assert.Contains(t, authURL, "accounts.google.com")
	assert.Contains(t, authURL, "test-client-id")
	assert.Contains(t, authURL, "contacts")
	assert.Contains(t, authURL, "code_challenge")

	// Check that a pending connection was created
	conn, err := repo.GetByID(context.Background(), stateToken)
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, "google", conn.ProviderType)
	assert.Equal(t, "user1", conn.UserID)
	assert.False(t, conn.Connected)
	assert.Equal(t, "test-client-id", conn.ClientID)
	assert.NotEmpty(t, conn.Password) // PKCE verifier stored temporarily
}

func TestGetAuthURL_MissingCredentials(t *testing.T) {
	repo := newMockConnRepo()
	cfg := config.GoogleConfig{}
	svc := service.NewGoogleOAuthService(cfg, repo)

	_, _, err := svc.GetAuthURL(context.Background(), "user1", "", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client_id and client_secret are required")
}

func TestGetAuthURL_MissingRedirectURL(t *testing.T) {
	repo := newMockConnRepo()
	cfg := config.GoogleConfig{} // no redirect_url
	svc := service.NewGoogleOAuthService(cfg, repo)

	_, _, err := svc.GetAuthURL(context.Background(), "user1", "id", "secret", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redirect_url is required")
}

func TestHandleCallback_InvalidState(t *testing.T) {
	repo := newMockConnRepo()
	cfg := config.GoogleConfig{RedirectURL: "http://localhost/callback"}
	svc := service.NewGoogleOAuthService(cfg, repo)

	_, err := svc.HandleCallback(context.Background(), "nonexistent-state", "some-code")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state token")
}

func TestGetHTTPClient_NoTokens(t *testing.T) {
	repo := newMockConnRepo()
	cfg := config.GoogleConfig{}
	svc := service.NewGoogleOAuthService(cfg, repo)

	conn := &domain.ProviderConnection{
		ID:           "conn1",
		AccessToken:  "",
		RefreshToken: "",
	}

	_, err := svc.GetHTTPClient(context.Background(), conn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tokens")
}

func TestRevokeToken_DeletesConnection(t *testing.T) {
	repo := newMockConnRepo()
	conn := &domain.ProviderConnection{
		ID:           "conn-to-delete",
		UserID:       "user1",
		ProviderType: "google",
		Connected:    true,
		RefreshToken: "fake-refresh-token",
	}
	repo.connections["conn-to-delete"] = conn

	cfg := config.GoogleConfig{}
	svc := service.NewGoogleOAuthService(cfg, repo)

	err := svc.RevokeToken(context.Background(), conn)
	require.NoError(t, err)

	// Connection should be deleted
	c, _ := repo.GetByID(context.Background(), "conn-to-delete")
	assert.Nil(t, c)
}
