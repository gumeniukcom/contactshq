package sync_test

import (
	"errors"
	"testing"

	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

func TestParseSyncMode(t *testing.T) {
	tests := []struct {
		in      string
		want    chqsync.SyncMode
		wantErr bool
	}{
		{in: "import", want: chqsync.SyncModeImport},
		{in: "export", want: chqsync.SyncModeExport},
		{in: "two_way", want: chqsync.SyncModeTwoWay},
		// Legacy vocabulary, still present in rows written before migration 019.
		{in: "pull", want: chqsync.SyncModeImport},
		{in: "push", want: chqsync.SyncModeExport},
		{in: "bidirectional", want: chqsync.SyncModeTwoWay},
		// An unset column must not mean "do nothing".
		{in: "", want: chqsync.SyncModeImport},
		{in: "sideways", wantErr: true},
		{in: "IMPORT", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := chqsync.ParseSyncMode(tt.in)

			if tt.wantErr {
				if !errors.Is(err, chqsync.ErrUnknownSyncMode) {
					t.Fatalf("ParseSyncMode(%q) error = %v, want ErrUnknownSyncMode", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSyncMode(%q) = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseSyncMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateStep(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		dest       string
		wantErr    bool
		wantReason string
	}{
		{name: "carddav import", source: "carddav", dest: "internal"},
		{name: "google import", source: "google", dest: "internal"},
		{name: "internal as source", source: "internal", dest: "carddav", wantErr: true},
		{name: "provider to provider", source: "carddav", dest: "google", wantErr: true},
		{name: "internal to internal", source: "internal", dest: "internal", wantErr: true},
		{name: "missing source", source: "", dest: "internal", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := chqsync.ValidateStep(tt.source, "", tt.dest, "", chqsync.EndpointPolicy{})

			if tt.wantErr {
				if !errors.Is(err, chqsync.ErrInvalidStep) {
					t.Fatalf("ValidateStep(%q, %q) = %v, want ErrInvalidStep", tt.source, tt.dest, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateStep(%q, %q) = %v, want nil", tt.source, tt.dest, err)
			}
		})
	}
}
