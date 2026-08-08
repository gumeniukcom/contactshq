package sync

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// A conflict with no attributable field diffs is the ordinary manual-mode case: two sides
// edited different properties. Marshalling that as `null` rather than `[]` sent the browser a
// value it parsed without error and then called .forEach on, blanking the conflict screens.
func TestFieldDiffs_MarshalAsAnEmptyListNotNull(t *testing.T) {
	var nilSlice []FieldConflict
	raw, err := json.Marshal(nilSlice)
	require.NoError(t, err)
	require.Equal(t, "null", string(raw),
		"guard: a nil slice still marshals to null, which is why recordConflict must not use one")

	empty := []FieldConflict{}
	raw, err = json.Marshal(empty)
	require.NoError(t, err)
	require.Equal(t, "[]", string(raw))
}
