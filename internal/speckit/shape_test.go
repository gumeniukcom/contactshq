package speckit

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// houseSections are the six the ContactsHQ override appends after Assumptions, in order.
// The ownership and enforcer checks read three of them, so their absence is not a style
// complaint — it is the checker losing its inputs.
var houseSections = []string{
	"Status",
	"Code Paths",
	"References",
	"Enforced By",
	"Known Divergences",
	"Amendments",
}

var validStatus = map[string]bool{"draft": true, "shipped": true, "partial": true}
var validKind = map[string]bool{"journey": true, "component": true, "meta": true}

func TestSpecHeaderIsWellFormed(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		if !validKind[s.header["Kind"]] {
			t.Errorf("%s: Kind is %q, want journey, component or meta", s.name, s.header["Kind"])
		}
		// FR-012: the vocabulary is fixed so `draft` can waive content assertions elsewhere
		// without every reader inventing a synonym for it.
		if !validStatus[s.header["Status"]] {
			t.Errorf("%s: Status is %q, want draft, shipped or partial", s.name, s.header["Status"])
		}
		if !strings.HasPrefix(s.header["Constitution"], "v") {
			t.Errorf("%s: Constitution is %q, want a version like v1.0.0", s.name, s.header["Constitution"])
		}
	}
}

func TestSpecCarriesTheHouseSections(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		positions := map[string]int{}
		for i, heading := range s.order {
			if _, ok := positions[heading]; !ok {
				positions[heading] = i
			}
		}

		previous := -1
		for _, want := range houseSections {
			at, ok := positions[want]
			if !ok {
				t.Errorf("%s has no \"## %s\" section", s.name, want)
				continue
			}
			if at < previous {
				t.Errorf("%s: \"## %s\" appears out of order; the house template fixes the order %s",
					s.name, want, strings.Join(houseSections, " → "))
			}
			previous = at
		}
	}
}

// placeholders are the template shapes that mean nobody finished the file. The override deletes
// two of them outright because this repo creates no branch per spec.
var placeholders = []string{"$ARGUMENTS", "[FEATURE NAME]", "**Feature Branch**", "**Input**"}

func TestSpecHasNoTemplatePlaceholders(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		data, err := os.ReadFile(filepath.Join(root, s.path))
		if err != nil {
			t.Fatalf("read %s: %v", s.path, err)
		}

		for _, line := range strings.Split(string(data), "\n") {
			// An Amendments row recording that a placeholder was removed necessarily contains
			// its name. Excluding the table keeps the check from punishing the honesty.
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				continue
			}
			for _, p := range placeholders {
				if strings.Contains(line, p) {
					t.Errorf("%s still contains the template placeholder %q: %s",
						s.name, p, strings.TrimSpace(line))
				}
			}
		}
	}
}

// cyrillic guards constitution "Language": these artefacts are English regardless of the
// language of the conversation that produced them.
var cyrillic = regexp.MustCompile(`\p{Cyrillic}`)

func TestSpecArtefactsAreEnglish(t *testing.T) {
	root := repoRoot(t)

	cmd := exec.Command("git", "ls-files", "-z", "specs", ".specify")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files unavailable: %v", err)
	}

	var offenders []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" || !strings.HasSuffix(f, ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if cyrillic.Match(data) {
			offenders = append(offenders, f)
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("these artefacts contain Cyrillic text; specs and .specify are English only:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
