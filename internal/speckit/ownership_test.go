package speckit

import (
	"sort"
	"strings"
	"testing"
)

// TestEveryTrackedPathIsClaimed is spec 000's FR-008 and FR-010.
//
// A path owned by no spec is a change nobody has to justify against a stated requirement. The
// failure is resolvable in exactly two ways, and the message says both: claim it in a spec's
// "## Code Paths", or list it in specs/UNCLAIMED.md with a written reason.
func TestEveryTrackedPathIsClaimed(t *testing.T) {
	root := repoRoot(t)
	specs := loadSpecs(t, root)
	exempt := unclaimed(t, root)

	var allClaims []string
	for _, s := range specs {
		allClaims = append(allClaims, s.claims()...)
	}

	var orphans []string
	for _, file := range trackedFiles(t, root) {
		if claimedBy(allClaims, file) || claimedBy(exempt, file) {
			continue
		}
		orphans = append(orphans, file)
	}

	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Errorf("%d tracked path(s) are owned by no spec.\n"+
			"Claim each one in a spec's \"## Code Paths\", or add it to specs/UNCLAIMED.md with\n"+
			"the reason it is deliberately unowned:\n  %s",
			len(orphans), strings.Join(orphans, "\n  "))
	}
}

// TestNoPathIsClaimedTwice is FR-008's other half, and constitution Principle VII.
//
// Two owners is worse than none: when the specs disagree about a file, there is no rule for
// deciding which one the code is supposed to satisfy. It is never resolved by preferring the
// lower spec number.
func TestNoPathIsClaimedTwice(t *testing.T) {
	root := repoRoot(t)
	specs := loadSpecs(t, root)

	owners := map[string][]string{}
	for _, s := range specs {
		seen := map[string]bool{}
		for _, claim := range s.claims() {
			if seen[claim] {
				continue // a spec repeating itself is untidy, not ambiguous
			}
			seen[claim] = true
			owners[claim] = append(owners[claim], s.name)
		}
	}

	for claim, specNames := range owners {
		if len(specNames) > 1 {
			sort.Strings(specNames)
			t.Errorf("%q is claimed by %s — exactly one spec must own a path",
				claim, strings.Join(specNames, " and "))
		}
	}

	// Overlapping claims of different shapes are the same defect wearing a disguise: a spec
	// claiming internal/vcard while another claims internal/vcard/encoder.go leaves the second
	// file with two owners even though the strings differ.
	var claims []string
	for claim := range owners {
		claims = append(claims, claim)
	}
	sort.Strings(claims)

	for _, file := range trackedFiles(t, root) {
		var hits []string
		for _, claim := range claims {
			if covers(claim, file) {
				hits = append(hits, claim+" ("+owners[claim][0]+")")
			}
		}
		if len(hits) > 1 {
			t.Errorf("%s is covered by %d claims: %s", file, len(hits), strings.Join(hits, ", "))
		}
	}
}

// TestNoBareDenseDirectoryClaim is FR-009.
//
// This is the rule that keeps the first test honest. A claim on `internal/service` would make
// every file added there afterwards owned automatically, so coverage would stay at 100% while
// meaning nothing.
func TestNoBareDenseDirectoryClaim(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		for _, claim := range s.claims() {
			normalised := strings.TrimSuffix(strings.TrimSuffix(claim, "/**"), "/")
			for _, tree := range denseTrees {
				if normalised == tree {
					t.Errorf("%s claims %q as a bare directory; %s is dense and cross-domain, "+
						"so entries must be per file or per subpackage", s.name, claim, tree)
				}
			}
		}
	}
}

// TestClaimsAreLiteralPaths catches the failure that hid fourteen migrations: a claim written
// as `migrations/013_x.{up,down}.sql` reads correctly to a human and resolves to nothing at all
// under a path lookup, so both halves silently became unowned.
func TestClaimsAreLiteralPaths(t *testing.T) {
	root := repoRoot(t)

	for _, s := range loadSpecs(t, root) {
		for _, claim := range s.claims() {
			if strings.ContainsAny(claim, "{}") {
				t.Errorf("%s claims %q using brace expansion; write each path on its own line — "+
					"a checker matches literal paths, not shell syntax", s.name, claim)
			}
		}
	}
}

func claimedBy(claims []string, file string) bool {
	for _, claim := range claims {
		if covers(claim, file) {
			return true
		}
	}
	return false
}
