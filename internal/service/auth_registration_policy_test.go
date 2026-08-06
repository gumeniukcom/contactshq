package service

import (
	"context"
	"errors"
	"testing"
)

// Bootstrapping must work even with registration closed, or a fresh deployment has no way in
// at all: no account exists, and every path to creating one requires an account.
func TestRegister_FirstAccountIsAllowedWhenRegistrationIsClosed(t *testing.T) {
	svc := newEmptyAuthService()

	user, err := svc.Register(context.Background(), "owner@example.com", "correct-horse", "Owner")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Fatalf("first user role = %q, want %q", user.Role, RoleAdmin)
	}
}

// The actual fix: once the instance has an owner, public sign-up is refused. Before this,
// anyone who could reach the port got an account for the price of one HTTP request.
func TestRegister_SecondAccountIsRefusedWhenRegistrationIsClosed(t *testing.T) {
	svc := newEmptyAuthService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
		t.Fatalf("Register (bootstrap): %v", err)
	}

	_, err := svc.Register(ctx, "intruder@example.com", "correct-horse", "Intruder")
	if !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("Register error = %v, want ErrRegistrationClosed", err)
	}
}

func TestRegister_SecondAccountIsAllowedWhenRegistrationIsOpen(t *testing.T) {
	svc := newOpenAuthService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
		t.Fatalf("Register (bootstrap): %v", err)
	}

	user, err := svc.Register(ctx, "colleague@example.com", "correct-horse", "Colleague")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Role != RoleUser {
		t.Fatalf("role = %q, want %q", user.Role, RoleUser)
	}
}

// An administrator adding a colleague goes through a route that already requires admin, so
// the public sign-up policy must not stand in the way.
func TestRegisterBypassPolicy_WorksWhenRegistrationIsClosed(t *testing.T) {
	svc := newEmptyAuthService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
		t.Fatalf("Register (bootstrap): %v", err)
	}

	user, err := svc.RegisterBypassPolicy(ctx, "colleague@example.com", "correct-horse", "Colleague")
	if err != nil {
		t.Fatalf("RegisterBypassPolicy: %v", err)
	}
	if user.Role != RoleUser {
		t.Fatalf("role = %q, want %q", user.Role, RoleUser)
	}
}

// A duplicate email must still be reported as such, not masked by the policy check.
func TestRegisterBypassPolicy_StillRejectsDuplicateEmail(t *testing.T) {
	svc := newEmptyAuthService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
		t.Fatalf("Register (bootstrap): %v", err)
	}

	_, err := svc.RegisterBypassPolicy(ctx, "owner@example.com", "correct-horse", "Again")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("error = %v, want ErrEmailTaken", err)
	}
}

func TestRegistrationOpen(t *testing.T) {
	ctx := context.Background()

	t.Run("open on an empty instance even when the flag is off", func(t *testing.T) {
		svc := newEmptyAuthService()
		open, err := svc.RegistrationOpen(ctx)
		if err != nil {
			t.Fatalf("RegistrationOpen: %v", err)
		}
		if !open {
			t.Fatal("registration must be open while no account exists")
		}
	})

	t.Run("closed once an account exists and the flag is off", func(t *testing.T) {
		svc := newEmptyAuthService()
		if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		open, err := svc.RegistrationOpen(ctx)
		if err != nil {
			t.Fatalf("RegistrationOpen: %v", err)
		}
		if open {
			t.Fatal("registration must be closed after bootstrap when the flag is off")
		}
	})

	t.Run("open when the flag is on", func(t *testing.T) {
		svc := newOpenAuthService()
		if _, err := svc.Register(ctx, "owner@example.com", "correct-horse", "Owner"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		open, err := svc.RegistrationOpen(ctx)
		if err != nil {
			t.Fatalf("RegistrationOpen: %v", err)
		}
		if !open {
			t.Fatal("registration must be open when the flag is on")
		}
	})
}
