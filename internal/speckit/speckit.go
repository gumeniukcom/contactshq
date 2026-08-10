// Package speckit enforces the spec tree under specs/ against the rules in
// .specify/memory/constitution.md.
//
// It contains no production code and is imported by nothing. It exists as a package of tests
// so the checks ride the harness the project already runs — `go test ./... -race` — rather
// than becoming a separate linter that has to be remembered and wired in.
//
// What it enforces, and why each rule is worth a failing build:
//
//   - Every tracked path is claimed by exactly one spec (constitution VII). A path owned by
//     nobody means a change to it contradicts no stated requirement; a path owned by two means
//     neither spec is trustworthy when they disagree.
//   - No bare claim on a dense tree. A blanket directory claim silently adopts every file added
//     inside it later, which disarms the first rule permanently and invisibly.
//   - Every test named in a spec's "Enforced By" section exists. A spec that cites a test that
//     was renamed away is claiming an assurance it does not have.
//   - Every spec keeps the house template's shape, so the sections these checks read are
//     actually there.
//
// Ownership assertions apply regardless of a spec's Status, including `draft`: a draft spec
// still claims paths, and waiving the rule would leave holes exactly where work is happening.
package speckit

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// denseTrees may not be claimed as a bare directory. The list is fixed by constitution
// Principle VII; changing it here without changing the constitution is a bug in this file.
var denseTrees = []string{
	"internal/handler",
	"internal/service",
	"internal/repository",
	"internal/domain",
	"web/src",
}

// auditedPrefixes bounds what must be owned. Everything else in the repository — the Makefile,
// CI workflows, README — is documentation or build plumbing that no spec claims to specify.
var auditedPrefixes = []string{
	"cmd/",
	"internal/",
	"migrations/",
	"web/src/",
	".specify/",
	".claude/",
}

// backtickPath matches the `path/like/this` tokens the templates use for every path and test
// name, which is why both readers below can share one expression.
var backtickPath = regexp.MustCompile("`([^`]+)`")

// repoRoot walks up from the test's working directory to the module root. Tests run with their
// package directory as the working directory, so nothing here may assume otherwise.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found in any parent directory")
		}
		dir = parent
	}
}

// trackedFiles is the audit surface: everything git would keep, which is tracked files plus
// untracked ones that are not ignored.
//
// Both halves matter. Asking git rather than walking the filesystem keeps generated output
// (web/node_modules, internal/web/static/spa) and per-machine state (.specify/feature.json) from
// being demanded of the specs — those are ignored, and ignoring them is a decision already
// recorded elsewhere. Including the untracked-but-unignored half is what makes the checks useful
// while work is still in progress: a new file is claimed, and a new test counts as an enforcer,
// before anything is staged. Without it the gate reported seven freshly written tests as missing
// purely because they had not been added yet.
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()

	seen := map[string]bool{}
	var files []string

	for _, args := range [][]string{
		{"ls-files", "-z"},
		{"ls-files", "-z", "--others", "--exclude-standard"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("git %v unavailable (%v) — the ownership map cannot be audited here", args, err)
		}

		for _, f := range strings.Split(string(out), "\x00") {
			if f == "" || seen[f] {
				continue
			}
			for _, prefix := range auditedPrefixes {
				if strings.HasPrefix(f, prefix) {
					seen[f] = true
					files = append(files, f)
					break
				}
			}
		}
	}

	if len(files) == 0 {
		t.Fatal("no files under the audited prefixes — the audit surface cannot be empty")
	}
	return files
}

// spec is one specs/NNN-slug/spec.md, parsed only as far as these checks need.
type spec struct {
	name     string            // directory name, e.g. "004-carddav-service"
	path     string            // repo-relative path to spec.md
	header   map[string]string // Kind / Status / Constitution
	sections map[string][]string
	order    []string // section headings in the order they appear
}

// specHeader matches the house template's "Kind: journey" header lines.
var specHeader = regexp.MustCompile(`^(Kind|Status|Constitution):\s*(.+)$`)

func loadSpecs(t *testing.T, root string) []spec {
	t.Helper()

	dirs, err := filepath.Glob(filepath.Join(root, "specs", "*", "spec.md"))
	if err != nil {
		t.Fatalf("glob specs: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("no specs found under specs/*/spec.md")
	}

	specs := make([]spec, 0, len(dirs))
	for _, file := range dirs {
		f, err := os.Open(file)
		if err != nil {
			t.Fatalf("open %s: %v", file, err)
		}

		s := spec{
			name:     filepath.Base(filepath.Dir(file)),
			path:     strings.TrimPrefix(file, root+string(os.PathSeparator)),
			header:   map[string]string{},
			sections: map[string][]string{},
		}

		current := ""
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "## ") {
				current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
				s.order = append(s.order, current)
				continue
			}
			if current == "" {
				if m := specHeader.FindStringSubmatch(line); m != nil {
					s.header[m[1]] = strings.TrimSpace(m[2])
				}
				continue
			}
			s.sections[current] = append(s.sections[current], line)
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		_ = f.Close()

		specs = append(specs, s)
	}
	return specs
}

// claims returns the paths a spec owns: the backtick-quoted tokens on list items inside
// "## Code Paths". Prose in that section is allowed and ignored, so a claim must be a list item
// to count — which is also what makes the section readable by a human.
func (s spec) claims() []string {
	var out []string
	for _, line := range s.sections["Code Paths"] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		if m := backtickPath.FindStringSubmatch(trimmed); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

// covers reports whether a claim owns a given file.
//
// Three forms are accepted, in the order a reader would expect: an exact path, a shell-style
// pattern within one segment (`.claude/skills/speckit-*/SKILL.md`), and a directory prefix.
// A "directory" is a claim ending in "/" or "/**", or one whose last segment has no extension —
// that last form is a convenience the specs already use (`internal/vcard`), not a licence for
// the dense trees, which are refused separately.
func covers(claim, file string) bool {
	claim = strings.TrimSuffix(claim, "/**")

	if claim == file {
		return true
	}
	if ok, err := filepath.Match(claim, file); err == nil && ok {
		return true
	}

	dir := strings.HasSuffix(claim, "/") || !strings.Contains(filepath.Base(claim), ".")
	if dir {
		return strings.HasPrefix(file, strings.TrimSuffix(claim, "/")+"/")
	}
	return false
}

// unclaimed reads the deliberate exemptions from specs/UNCLAIMED.md — the FIRST COLUMN of its
// table rows, and nothing else.
//
// Reading every backtick in the file, which is what this did first, let prose grant exemptions.
// The row explaining why the Vite output is exempt mentions `web/src` in its reason, and that one
// word silently excused all 106 files under it: the gate reported green over the largest tree in
// the repository while checking none of it. An exemption has to be claimed in the column meant
// for claims, so that mentioning a path while arguing about it stays free.
func unclaimed(t *testing.T, root string) []string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, "specs", "UNCLAIMED.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read UNCLAIMED.md: %v", err)
	}

	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) < 2 {
			continue // a separator row, or a table with no reason column
		}
		if m := backtickPath.FindStringSubmatch(cells[0]); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}
