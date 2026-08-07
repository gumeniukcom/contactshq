package speckit

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// testToken matches the Go test and benchmark names cited in "## Enforced By".
var testToken = regexp.MustCompile(`\b((?:Test|Benchmark)[A-Z]\w*)`)

// testDefinition matches the real thing in a _test.go file.
var testDefinition = regexp.MustCompile(`(?m)^func ((?:Test|Benchmark)\w+)`)

// TestCitedEnforcersExist keeps "## Enforced By" from becoming a wish list.
//
// A spec cites tests as the reason to believe its requirements hold. When a test is renamed or
// deleted, the citation is the only thing left saying the requirement is covered — and it now
// says something false. This is the check that turns a rename into a failing build rather than
// a silent downgrade of every claim that named it.
//
// A cited token also passes if it is a prefix of a real test name: CI runs
// `go test ./internal/repository/ -run TestPostgres`, and `TestPostgres` is a filter for the
// whole PostgreSQL family, not a function anyone declared.
func TestCitedEnforcersExist(t *testing.T) {
	root := repoRoot(t)

	real := map[string]bool{}
	for _, file := range trackedFiles(t, root) {
		if !strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range testDefinition.FindAllStringSubmatch(string(data), -1) {
			real[m[1]] = true
		}
	}
	if len(real) == 0 {
		t.Fatal("no Go tests found in the repository — the enforcer check has nothing to verify against")
	}

	names := make([]string, 0, len(real))
	for name := range real {
		names = append(names, name)
	}

	missing := map[string][]string{}
	for _, s := range loadSpecs(t, root) {
		cited := map[string]bool{}
		for _, line := range s.sections["Enforced By"] {
			for _, m := range testToken.FindAllStringSubmatch(line, -1) {
				cited[m[1]] = true
			}
		}
		for token := range cited {
			if real[token] || isRunFilter(token, names) {
				continue
			}
			missing[token] = append(missing[token], s.name)
		}
	}

	for token, specs := range missing {
		sort.Strings(specs)
		t.Errorf("%s cites %s in \"## Enforced By\", but no such test exists — "+
			"either the test was renamed and the citation is now false, or the requirement is "+
			"unenforced and belongs in \"## Known Divergences\"",
			strings.Join(specs, " and "), token)
	}
}

// isRunFilter reports whether token is a `go test -run` prefix rather than a function name.
// It must match at least one real test and not be the whole of it, so a typo that happens to
// be a prefix of nothing still fails.
func isRunFilter(token string, names []string) bool {
	for _, name := range names {
		if name != token && strings.HasPrefix(name, token) {
			return true
		}
	}
	return false
}

// TestShippedSpecDeclaresItsDivergences is constitution Principle VI made mechanical.
//
// The hazard of a retrospective spec is that it launders whatever the code happens to do into a
// stated requirement. An empty "Known Divergences" on shipped software is therefore a claim that
// implementation and intent match exactly — rare enough that it should be written down
// deliberately, as the words "None known", rather than reached by leaving the section blank.
func TestShippedSpecDeclaresItsDivergences(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		if s.header["Status"] == "draft" {
			continue // FR-012: a draft waives content assertions
		}

		var body []string
		for _, line := range s.sections["Known Divergences"] {
			if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "<!--") {
				body = append(body, trimmed)
			}
		}

		if len(body) == 0 {
			t.Errorf("%s is %s but its \"## Known Divergences\" is empty. If the implementation "+
				"really matches the spec, say \"None known.\" explicitly",
				s.name, s.header["Status"])
		}
	}
}
