package sync

import (
	"errors"
	"net/http"
	"testing"

	"google.golang.org/api/googleapi"
)

// A 410 from the People API means the sync token is too old; the engine must be told to
// re-list in full rather than treat it as a hard failure.
func TestIsExpiredSyncToken(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"410 Gone", &googleapi.Error{Code: http.StatusGone}, true},
		{"wrapped 410", errWrap(&googleapi.Error{Code: http.StatusGone}), true},
		{"403 Forbidden", &googleapi.Error{Code: http.StatusForbidden}, false},
		{"500 error", &googleapi.Error{Code: http.StatusInternalServerError}, false},
		{"plain error", errors.New("network down"), false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpiredSyncToken(tt.err); got != tt.want {
				t.Errorf("isExpiredSyncToken(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type wrapped struct{ err error }

func (w wrapped) Error() string { return "wrapped: " + w.err.Error() }
func (w wrapped) Unwrap() error { return w.err }

func errWrap(err error) error { return wrapped{err} }
