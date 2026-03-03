package service

import (
	"context"
	"errors"
	"time"

	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already taken")
	ErrUserNotFound       = errors.New("user not found")
)

type AuthService struct {
	userRepo repository.UserRepository
	abRepo   repository.AddressBookRepository
	cfg      config.AuthConfig
}

func NewAuthService(userRepo repository.UserRepository, abRepo repository.AddressBookRepository, cfg config.AuthConfig) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		abRepo:   abRepo,
		cfg:      cfg,
	}
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func (s *AuthService) Register(ctx context.Context, email, password, displayName string) (*domain.User, error) {
	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now()
	user := &domain.User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Role:         "user",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	ab := &domain.AddressBook{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Name:      "Contacts",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.abRepo.Create(ctx, ab); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if !verifyPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return s.generateTokenPair(user)
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string) (*TokenPair, error) {
	claims, err := s.ValidateToken(refreshTokenStr)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return s.generateTokenPair(user)
}

func (s *AuthService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidCredentials
	}

	return claims, nil
}

func (s *AuthService) generateTokenPair(user *domain.User) (*TokenPair, error) {
	now := time.Now()
	expiresAt := now.Add(s.cfg.TokenTTL)

	accessClaims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   user.ID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenStr, err := accessToken.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	refreshClaims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.RefreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   user.ID,
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenStr, err := refreshToken.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
		ExpiresAt:    expiresAt.Unix(),
	}, nil
}

// Argon2id password hashing

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, b64Salt, b64Hash), nil
}

func verifyPassword(password, encodedHash string) bool {
	var memory uint32
	var time uint32
	var threads uint8
	var b64Salt, b64Hash string

	_, err := fmt.Sscanf(encodedHash, "$argon2id$v=19$m=%d,t=%d,p=%d$%s",
		&memory, &time, &threads, &b64Salt)
	if err != nil {
		return false
	}

	// Split salt$hash from b64Salt
	parts := splitLast(b64Salt, "$")
	if len(parts) != 2 {
		return false
	}
	b64Salt = parts[0]
	b64Hash = parts[1]

	salt, err := base64.RawStdEncoding.DecodeString(b64Salt)
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(b64Hash)
	if err != nil {
		return false
	}

	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, argonKeyLen)

	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1
}

func splitLast(s, sep string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
