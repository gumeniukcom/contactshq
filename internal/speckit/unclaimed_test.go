package speckit

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate reported green over all 106 files under web/src because UNCLAIMED.md mentioned
// `web/src` while explaining a different exemption. An exemption must be claimed in the column
// meant for claims; naming a path while arguing about it must grant nothing.
func TestUnclaimed_ProseGrantsNoExemption(t *testing.T) {
	dir := t.TempDir()
	specs := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}

	const doc = `# Deliberately unowned paths

| Path | Why no spec owns it |
|---|---|
| ` + "`internal/generated/zz.go`" + ` | Generated from ` + "`web/src`" + `, which is owned file by file. |

## Not on this list

Paths a spec claims but that do not exist yet (` + "`internal/speckit/`" + `).
`
	if err := os.WriteFile(filepath.Join(specs, "UNCLAIMED.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	got := unclaimed(t, dir)

	want := map[string]bool{"internal/generated/zz.go": true}
	for _, g := range got {
		if !want[g] {
			t.Errorf("%q was exempted, but it only appears in prose or in a reason column", g)
		}
		delete(want, g)
	}
	for w := range want {
		t.Errorf("%q is a table claim and should have been exempted", w)
	}
}
