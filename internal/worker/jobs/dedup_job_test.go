package jobs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
