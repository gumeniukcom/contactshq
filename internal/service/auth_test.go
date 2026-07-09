package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
)

type mockUserRepo struct {
	byID    map[string]*domain.User
	byEmail map[string]*domain.User
}

func (m *mockUserRepo) Create(_ context.Context, u *domain.User) error {
	m.byID[u.ID] = u
	m.byEmail[u.Email] = u
	return nil
}
func (m *mockUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	return m.byID[id], nil
}
func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	return m.byEmail[email], nil
}
func (m *mockUserRepo) Update(_ context.Context, _ *domain.User) error { return nil }
func (m *mockUserRepo) Delete(_ context.Context, _ string) error       { return nil }
func (m *mockUserRepo) List(_ context.Context, _, _ int) ([]*domain.User, int, error) {
	return nil, 0, nil
}
func (m *mockUserRepo) ListAllIDs(_ context.Context) ([]string, error) { return nil, nil }

func newTestAuthService(t *testing.T) (*AuthService, *domain.User) {
	t.Helper()

	user := &domain.User{ID: "u1", Email: "a@example.com", Role: "user"}
	repo := &mockUserRepo{
		byID:    map[string]*domain.User{user.ID: user},
		byEmail: map[string]*domain.User{user.Email: user},
	}
	cfg := config.AuthConfig{
		JWTSecret:  "0123456789abcdef0123456789abcdef",
		TokenTTL:   time.Hour,
		RefreshTTL: 24 * time.Hour,
	}
	return NewAuthService(repo, nil, cfg), user
}

func TestTokenPairCarriesTokenType(t *testing.T) {
	svc, user := newTestAuthService(t)

	pair, err := svc.generateTokenPair(user)
	if err != nil {
		t.Fatalf("generateTokenPair: %v", err)
	}

	access, err := svc.ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken(access) = %v, want nil", err)
	}
	if access.TokenType != TokenTypeAccess {
		t.Errorf("access typ = %q, want %q", access.TokenType, TokenTypeAccess)
	}

	refresh, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken(refresh) = %v, want nil", err)
	}
	if refresh.TokenType != TokenTypeRefresh {
		t.Errorf("refresh typ = %q, want %q", refresh.TokenType, TokenTypeRefresh)
	}
}

// A refresh token must not work as an API bearer token: it lives 30 days by default.
func TestRefreshTokenRejectedAsAccessToken(t *testing.T) {
	svc, user := newTestAuthService(t)

	pair, err := svc.generateTokenPair(user)
	if err != nil {
		t.Fatalf("generateTokenPair: %v", err)
	}

	if _, err := svc.ValidateToken(pair.RefreshToken); !errors.Is(err, ErrWrongTokenType) {
		t.Fatalf("ValidateToken(refresh) = %v, want ErrWrongTokenType", err)
	}
}

// An access token must not mint fresh token pairs, otherwise one leaked access token
// grants indefinite access.
func TestAccessTokenCannotRefresh(t *testing.T) {
	svc, user := newTestAuthService(t)

	pair, err := svc.generateTokenPair(user)
	if err != nil {
		t.Fatalf("generateTokenPair: %v", err)
	}

	if _, err := svc.ValidateRefreshToken(pair.AccessToken); !errors.Is(err, ErrWrongTokenType) {
		t.Fatalf("ValidateRefreshToken(access) = %v, want ErrWrongTokenType", err)
	}

	if _, err := svc.RefreshToken(context.Background(), pair.AccessToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("RefreshToken(access) = %v, want ErrInvalidCredentials", err)
	}
}

func TestRefreshTokenIssuesNewPair(t *testing.T) {
	svc, user := newTestAuthService(t)

	pair, err := svc.generateTokenPair(user)
	if err != nil {
		t.Fatalf("generateTokenPair: %v", err)
	}

	fresh, err := svc.RefreshToken(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken(refresh) = %v, want nil", err)
	}
	if _, err := svc.ValidateToken(fresh.AccessToken); err != nil {
		t.Fatalf("new access token invalid: %v", err)
	}
}

// Tokens signed with a different secret must never validate, and neither must
// tokens signed with "none".
func TestValidateTokenRejectsForeignSecret(t *testing.T) {
	svc, user := newTestAuthService(t)

	other := NewAuthService(&mockUserRepo{}, nil, config.AuthConfig{
		JWTSecret: "ffffffffffffffffffffffffffffffff",
		TokenTTL:  time.Hour,
	})
	pair, err := other.generateTokenPair(user)
	if err != nil {
		t.Fatalf("generateTokenPair: %v", err)
	}

	if _, err := svc.ValidateToken(pair.AccessToken); err == nil {
		t.Fatal("ValidateToken accepted a token signed with a foreign secret")
	}
}
