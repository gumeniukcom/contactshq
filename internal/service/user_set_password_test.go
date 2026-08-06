package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gumeniukcom/contactshq/internal/carddav"
	"github.com/gumeniukcom/contactshq/internal/domain"
)

func newUserServiceWith(t *testing.T, users ...*domain.User) (*UserService, *mockUserRepo) {
	t.Helper()
	repo := &mockUserRepo{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
	for _, u := range users {
		repo.byID[u.ID] = u
		repo.byEmail[u.Email] = u
	}
	return NewUserService(repo), repo
}

func seedUser(t *testing.T) *domain.User {
	t.Helper()
	hash, err := hashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	return &domain.User{
		ID: "u1", Email: "owner@example.com", PasswordHash: hash, Role: RoleAdmin,
	}
}

func TestSetPassword_ReplacesTheHashAndKeepsEverythingElse(t *testing.T) {
	user := seedUser(t)
	svc, _ := newUserServiceWith(t, user)

	if err := svc.SetPassword(context.Background(), "owner@example.com", "brand-new-password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	if !verifyPassword("brand-new-password", user.PasswordHash) {
		t.Fatal("the new password does not verify against the stored hash")
	}
	if verifyPassword("old-password", user.PasswordHash) {
		t.Fatal("the old password still verifies")
	}
	if user.Role != RoleAdmin {
		t.Fatalf("role changed to %q — recovering access must never alter privileges", user.Role)
	}
	if user.Email != "owner@example.com" {
		t.Fatalf("email changed to %q", user.Email)
	}
}

// Setting a password must not be a back door to creating an account.
func TestSetPassword_UnknownEmailIsAnError(t *testing.T) {
	svc, repo := newUserServiceWith(t, seedUser(t))

	err := svc.SetPassword(context.Background(), "nobody@example.com", "brand-new-password")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("SetPassword = %v, want ErrUserNotFound", err)
	}
	if len(repo.byEmail) != 1 {
		t.Fatal("SetPassword created an account")
	}
}

// The CLI must not be a way around the length rule the HTTP handler enforces.
func TestSetPassword_RejectsAShortPassword(t *testing.T) {
	user := seedUser(t)
	before := user.PasswordHash
	svc, _ := newUserServiceWith(t, user)

	err := svc.SetPassword(context.Background(), "owner@example.com", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("SetPassword = %v, want ErrPasswordTooShort", err)
	}
	if user.PasswordHash != before {
		t.Fatal("a rejected password still rewrote the hash")
	}
}

func TestSetPassword_InvokesTheCredentialInvalidatorOnce(t *testing.T) {
	user := seedUser(t)
	svc, _ := newUserServiceWith(t, user)

	var got []string
	svc.WithCredentialInvalidator(func(email string) { got = append(got, email) })

	if err := svc.SetPassword(context.Background(), "owner@example.com", "brand-new-password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	if len(got) != 1 || got[0] != "owner@example.com" {
		t.Fatalf("invalidator calls = %v, want exactly [owner@example.com]", got)
	}
}

func TestChangePassword_InvokesTheCredentialInvalidator(t *testing.T) {
	user := seedUser(t)
	svc, _ := newUserServiceWith(t, user)

	var got []string
	svc.WithCredentialInvalidator(func(email string) { got = append(got, email) })

	if err := svc.ChangePassword(context.Background(), "u1", "old-password", "brand-new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("invalidator calls = %v, want exactly one", got)
	}
}

// The callback is optional; nothing may panic without it.
func TestSetPassword_WithoutAnInvalidatorDoesNotPanic(t *testing.T) {
	svc, _ := newUserServiceWith(t, seedUser(t))

	if err := svc.SetPassword(context.Background(), "owner@example.com", "brand-new-password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
}

// There are two argon2id verifiers in this codebase — service.verifyPassword and
// carddav.verifyArgon2id — because service must not import carddav. They read the encoded
// parameters differently (one takes the key length from a constant, the other derives it from
// the stored hash), so a hash that only one of them accepts would let a user log in over HTTP
// but not over CardDAV, or the reverse. Every password-writing path has to satisfy both.
func TestSetPassword_HashIsAcceptedByBothVerifiers(t *testing.T) {
	user := seedUser(t)
	svc, _ := newUserServiceWith(t, user)

	const pw = "brand-new-password"
	if err := svc.SetPassword(context.Background(), "owner@example.com", pw); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	if !verifyPassword(pw, user.PasswordHash) {
		t.Fatal("service.verifyPassword rejected the hash it must accept")
	}
	if !carddav.VerifyArgon2id(pw, user.PasswordHash) {
		t.Fatal("carddav's verifier rejected the hash — HTTP login would work but CardDAV would not")
	}

	// And neither accepts the wrong password.
	if verifyPassword("wrong-password", user.PasswordHash) ||
		carddav.VerifyArgon2id("wrong-password", user.PasswordHash) {
		t.Fatal("a wrong password verified")
	}
}
