package sync_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

// buildVCard is a test helper that creates a minimal vCard string.
// fields is a map of field-name → value (e.g. "TEL" → "+1234567890").
func buildVCard(uid, fn string, fields map[string]string) string {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCARD\r\n")
	sb.WriteString("VERSION:3.0\r\n")
	sb.WriteString("UID:" + uid + "\r\n")
	sb.WriteString("FN:" + fn + "\r\n")
	for k, v := range fields {
		sb.WriteString(k + ":" + v + "\r\n")
	}
	sb.WriteString("END:VCARD\r\n")
	return sb.String()
}

// TestMergeVCards_NoChange verifies that when base == local == remote the
// merge auto-succeeds and produces no conflicts.
func TestMergeVCards_NoChange(t *testing.T) {
	card := buildVCard("uid1", "Alice", nil)
	result, err := chqsync.MergeVCards(card, card, card)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Empty(t, result.Conflicts)
	assert.NotEmpty(t, result.MergedVCard)
}

// TestMergeVCards_OnlyRemoteChanged verifies remote change is auto-applied.
func TestMergeVCards_OnlyRemoteChanged(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	result, err := chqsync.MergeVCards(base, local, remote)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Empty(t, result.Conflicts)
	assert.Contains(t, result.MergedVCard, "Alice Remote")
}

// TestMergeVCards_OnlyLocalChanged verifies local change is kept.
func TestMergeVCards_OnlyLocalChanged(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice", nil)

	result, err := chqsync.MergeVCards(base, local, remote)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Empty(t, result.Conflicts)
	assert.Contains(t, result.MergedVCard, "Alice Local")
}

// TestMergeVCards_BothChanged verifies a conflict is raised when both sides differ.
func TestMergeVCards_BothChanged(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	result, err := chqsync.MergeVCards(base, local, remote)
	require.NoError(t, err)
	assert.False(t, result.AutoMerged)
	require.NotEmpty(t, result.Conflicts)
	assert.Equal(t, "FN", result.Conflicts[0].Field)
	assert.Contains(t, result.Conflicts[0].Local, "Alice Local")
	assert.Contains(t, result.Conflicts[0].Remote, "Alice Remote")
}

// TestMergeVCards_NoBase_BothDiffer verifies conflict when there is no base and sides differ.
func TestMergeVCards_NoBase_BothDiffer(t *testing.T) {
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	result, err := chqsync.MergeVCards("", local, remote)
	require.NoError(t, err)
	assert.False(t, result.AutoMerged)
	assert.NotEmpty(t, result.Conflicts)
}

// TestMergeVCards_NoBase_BothSame verifies auto-merge when sides agree and base is empty.
func TestMergeVCards_NoBase_BothSame(t *testing.T) {
	card := buildVCard("uid1", "Alice", nil)

	result, err := chqsync.MergeVCards("", card, card)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Empty(t, result.Conflicts)
}

// TestMergeVCards_ExtraFieldRemoteAdded verifies new remote field is auto-merged.
func TestMergeVCards_ExtraFieldRemoteAdded(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice", nil)
	remote := buildVCard("uid1", "Alice", map[string]string{"TEL": "+1234567890"})

	result, err := chqsync.MergeVCards(base, local, remote)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Contains(t, result.MergedVCard, "+1234567890")
}

// TestMergeVCards_UIDPreserved verifies UID is always taken from local.
func TestMergeVCards_UIDPreserved(t *testing.T) {
	base := buildVCard("uid-base", "Alice", nil)
	local := buildVCard("uid-local", "Alice", nil)
	remote := buildVCard("uid-remote", "Alice", nil)

	result, err := chqsync.MergeVCards(base, local, remote)
	require.NoError(t, err)
	assert.True(t, result.AutoMerged)
	assert.Contains(t, result.MergedVCard, "uid-local")
	assert.NotContains(t, result.MergedVCard, "uid-remote")
}

// TestApplyResolution_LocalChoice verifies "local" choice applies local value.
func TestApplyResolution_LocalChoice(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	resolved, err := chqsync.ApplyResolution(base, local, remote, map[string]string{
		"FN": "local",
	})
	require.NoError(t, err)
	assert.Contains(t, resolved, "Alice Local")
	assert.NotContains(t, resolved, "Alice Remote")
}

// TestApplyResolution_RemoteChoice verifies "remote" choice applies remote value.
func TestApplyResolution_RemoteChoice(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	resolved, err := chqsync.ApplyResolution(base, local, remote, map[string]string{
		"FN": "remote",
	})
	require.NoError(t, err)
	assert.Contains(t, resolved, "Alice Remote")
	assert.NotContains(t, resolved, "Alice Local")
}

// TestApplyResolution_DefaultsToLocal verifies unset fields default to local.
func TestApplyResolution_DefaultsToLocal(t *testing.T) {
	base := buildVCard("uid1", "Alice", nil)
	local := buildVCard("uid1", "Alice Local", nil)
	remote := buildVCard("uid1", "Alice Remote", nil)

	// Empty resolution map → all fields default to local
	resolved, err := chqsync.ApplyResolution(base, local, remote, map[string]string{})
	require.NoError(t, err)
	assert.Contains(t, resolved, "Alice Local")
}

// TestApplyResolution_UIDAlwaysLocal verifies UID is taken from local even if resolution says remote.
func TestApplyResolution_UIDAlwaysLocal(t *testing.T) {
	base := buildVCard("uid-base", "Alice", nil)
	local := buildVCard("uid-local", "Alice", nil)
	remote := buildVCard("uid-remote", "Alice", nil)

	resolved, err := chqsync.ApplyResolution(base, local, remote, map[string]string{
		"UID": "remote", // should be ignored by ApplyResolution (UID is in skipFields)
	})
	require.NoError(t, err)
	assert.Contains(t, resolved, "uid-local")
}
