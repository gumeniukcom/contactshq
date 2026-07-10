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
			err := ServerConfig{TrustedProxies: tt.proxies}.validate()
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
