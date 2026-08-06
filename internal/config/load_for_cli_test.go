package config

import (
	"errors"
	"reflect"
	"testing"
)

// A subcommand that only touches the database must not require the signing secret: an
// operator recovering access should not have to reconstruct the secret the running server
// holds in its environment. The server itself must still refuse to start without it.
func TestLoadForCLI_DoesNotRequireTheSigningSecret(t *testing.T) {
	t.Setenv("CHQ_DATABASE_DSN", "contactshq.db")

	if _, err := LoadForCLI(); err != nil {
		t.Fatalf("LoadForCLI() without a secret = %v, want nil", err)
	}

	if _, err := Load(); !errors.Is(err, ErrWeakJWTSecret) {
		t.Fatalf("Load() without a secret = %v, want ErrWeakJWTSecret", err)
	}
}

// Relaxing the secret check must not relax anything else.
func TestLoadForCLI_StillValidatesTheDatabase(t *testing.T) {
	t.Setenv("CHQ_DATABASE_DRIVER", "mysql")

	if _, err := LoadForCLI(); !errors.Is(err, ErrInvalidDatabase) {
		t.Fatalf("LoadForCLI() with an unsupported driver = %v, want ErrInvalidDatabase", err)
	}
}

// The two entry points must not drift into different defaults, or a subcommand would operate
// on a different database than the server it is meant to repair.
func TestLoadForCLI_MatchesLoadOnEveryOtherField(t *testing.T) {
	t.Setenv("CHQ_AUTH_JWT_SECRET", validSecret)
	t.Setenv("CHQ_DATABASE_DSN", "/var/lib/contactshq/app.db")
	t.Setenv("CHQ_SERVER_PORT", "9090")

	server, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	cli, err := LoadForCLI()
	if err != nil {
		t.Fatalf("LoadForCLI(): %v", err)
	}

	if !reflect.DeepEqual(server, cli) {
		t.Fatalf("LoadForCLI() = %+v, Load() = %+v — the two must read identical configuration", cli, server)
	}
}
