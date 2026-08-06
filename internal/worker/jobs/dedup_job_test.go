package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/gumeniukcom/contactshq/internal/worker/jobs"
)

func TestDedupJobPayload_Roundtrip(t *testing.T) {
	original := jobs.DedupJobPayload{UserID: "user-123"}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded jobs.DedupJobPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "user-123", decoded.UserID)
}

func TestDedupJobHandler_InvalidPayload(t *testing.T) {
	h := jobs.NewDedupJobHandler(nil, nil)

	err := h.Handle(context.Background(), json.RawMessage(`{invalid`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal dedup job payload")
}

type stubDuplicateScanner struct {
	calls []string
	err   error
}

func (s *stubDuplicateScanner) Detect(_ context.Context, userID string) (*service.DetectionResult, error) {
	s.calls = append(s.calls, userID)
	if s.err != nil {
		return nil, s.err
	}
	return &service.DetectionResult{Found: 2, Checked: 10}, nil
}

func TestDedupJob_RunsTheScan(t *testing.T) {
	scanner := &stubDuplicateScanner{}
	handler := jobs.NewDedupJobHandler(scanner, zap.NewNop())

	raw, err := json.Marshal(jobs.DedupJobPayload{UserID: "u1"})
	require.NoError(t, err)
	require.NoError(t, handler.Handle(context.Background(), raw))

	require.Equal(t, []string{"u1"}, scanner.calls)
}

func TestDedupJob_WrapsAFailureWithTheUserID(t *testing.T) {
	scanner := &stubDuplicateScanner{err: errors.New("detector exploded")}
	handler := jobs.NewDedupJobHandler(scanner, zap.NewNop())

	raw, err := json.Marshal(jobs.DedupJobPayload{UserID: "u1"})
	require.NoError(t, err)

	err = handler.Handle(context.Background(), raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "u1")
	require.Contains(t, err.Error(), "detector exploded")
}

func TestDedupJob_RejectsAnUnreadablePayload(t *testing.T) {
	scanner := &stubDuplicateScanner{}
	handler := jobs.NewDedupJobHandler(scanner, zap.NewNop())

	require.Error(t, handler.Handle(context.Background(), json.RawMessage("{{{")))
	require.Empty(t, scanner.calls)
}
