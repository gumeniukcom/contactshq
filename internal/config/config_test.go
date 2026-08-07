package config

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestServerConfigValidate_TrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{name: "empty", proxies: nil},
		{name: "single IPv4", proxies: []string{"10.0.0.1"}},
		{name: "IPv6", proxies: []string{"::1"}},
		{name: "CIDR", proxies: []string{"10.0.0.0/8"}},
		{name: "mixed", proxies: []string{"192.168.1.1", "172.16.0.0/12", "::1"}},
		{name: "garbage", proxies: []string{"not-an-ip"}, wantErr: true},
		{name: "bad CIDR", proxies: []string{"10.0.0.0/99"}, wantErr: true},
		{name: "one good one bad", proxies: []string{"10.0.0.1", "nope"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The limits are part of a valid ServerConfig now; this test is about the
			// proxy list, so they are filled with workable values.
			cfg := ServerConfig{
				TrustedProxies: tt.proxies,
				MaxBodyBytes:   32 << 20,
				MaxImportBytes: 32 << 20,
			}
			err := cfg.validate()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidTrustedProxy) {
					t.Fatalf("validate() = %v, want ErrInvalidTrustedProxy", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "yaml list", in: []string{"10.0.0.1", "10.0.0.2"}, want: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "env csv", in: []string{"10.0.0.1,10.0.0.2"}, want: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "csv with spaces", in: []string{"10.0.0.1, 10.0.0.2 , 10.0.0.3"}, want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
		{name: "blanks dropped", in: []string{"", "10.0.0.1", "  "}, want: []string{"10.0.0.1"}},
		{name: "empty", in: nil, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitList(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitList(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitList(%v) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

// A per-route limit above the global one is a promise the server cannot keep: fasthttp
// rejects the request before the route's middleware ever runs.
func TestServerConfigValidate_Limits(t *testing.T) {
	base := func() ServerConfig {
		return ServerConfig{MaxBodyBytes: 32 << 20, MaxImportBytes: 8 << 20}
	}

	if err := base().validate(); err != nil {
		t.Fatalf("a sane configuration was rejected: %v", err)
	}

	tooBig := base()
	tooBig.MaxImportBytes = tooBig.MaxBodyBytes + 1
	if err := tooBig.validate(); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("validate() = %v, want ErrInvalidLimit", err)
	}

	zero := base()
	zero.MaxBodyBytes = 0
	if err := zero.validate(); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("validate() = %v, want ErrInvalidLimit", err)
	}

	// Restore and import run synchronously inside the request; a write deadline truncates an
	// operation that is still mutating contacts, so the server refuses to start with one.
	withWrite := base()
	withWrite.WriteTimeout = time.Second
	if err := withWrite.validate(); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("validate() = %v, want ErrInvalidLimit for a non-zero write timeout", err)
	}
}

func TestCardDAVConfigValidate(t *testing.T) {
	if err := (CardDAVConfig{MaxResourceBytes: 1 << 20}).validate(); err != nil {
		t.Fatalf("a sane configuration was rejected: %v", err)
	}
	if err := (CardDAVConfig{}).validate(); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("validate() = %v, want ErrInvalidLimit", err)
	}
}

// The token lifetime is the revocation window: there is no denylist, so a leaked token is
// valid until it expires. These defaults are a deliberate choice, not an accident, and the
// SPA's refresh interceptor is what makes the short one invisible to users.
func TestDefaults_TokenLifetimes(t *testing.T) {
	t.Setenv("CHQ_AUTH_JWT_SECRET", validSecret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	if cfg.Auth.TokenTTL != time.Hour {
		t.Errorf("auth.token_ttl = %v, want 1h", cfg.Auth.TokenTTL)
	}
	if cfg.Auth.RefreshTTL != 168*time.Hour {
		t.Errorf("auth.refresh_ttl = %v, want 168h", cfg.Auth.RefreshTTL)
	}
}
