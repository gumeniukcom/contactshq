package config

import (
	"errors"
	"strings"
	"testing"
)

func TestAuthConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "empty", secret: "", wantErr: true},
		{name: "whitespace only", secret: "   ", wantErr: true},
		{name: "shipped placeholder", secret: "change-me-in-production", wantErr: true},
		{name: "placeholder case insensitive", secret: "Change-Me-In-Production", wantErr: true},
		{name: "other known weak value", secret: "secret", wantErr: true},
		{name: "too short", secret: strings.Repeat("a", MinJWTSecretLen-1), wantErr: true},
		{name: "minimum length", secret: strings.Repeat("a", MinJWTSecretLen), wantErr: false},
		{name: "hex 32 bytes", secret: strings.Repeat("0f", 32), wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AuthConfig{JWTSecret: tt.secret}.validate()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() = nil, want error")
				}
				if !errors.Is(err, ErrWeakJWTSecret) {
					t.Fatalf("validate() error = %v, want ErrWeakJWTSecret", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestConfigValidateRejectsDefaultSecret(t *testing.T) {
	// The value that shipped in docker-compose.yml and configs/config.example.yaml.
	cfg := Config{Auth: AuthConfig{JWTSecret: "change-me-in-production"}}

	if err := cfg.Validate(); !errors.Is(err, ErrWeakJWTSecret) {
		t.Fatalf("Validate() = %v, want ErrWeakJWTSecret", err)
	}
}
