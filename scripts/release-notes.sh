#!/bin/sh
# Extracts the CHANGELOG.md section for the version being released into .release-notes.md,
# which .goreleaser.yaml prepends to the GitHub release body.
#
# Without this the release page is goreleaser's raw commit list — full SHAs and subject
# lines, which says what was touched but never why it matters or what breaks. The notes are
# written by hand in CHANGELOG.md; this only picks the right section out of it.
#
# The version is derived from the tag, so `goreleaser release` needs no extra arguments.
# Failing loudly on a missing section is deliberate: a release that silently ships an empty
# body is worse than one that stops and asks for the notes to be written.
set -eu

version="${1:-$(git describe --tags --abbrev=0)}"
version="${version#v}"
out=".release-notes.md"

awk -v want="$version" '
  BEGIN { prefix = "## [" want "]" }
  # A section header ends the previous section and may start the wanted one. Compared as a
  # literal prefix, not a regex: the dots in a version number would each match any character.
  /^## \[/ { inside = (substr($0, 1, length(prefix)) == prefix); next }
  # The link definitions at the foot of the file belong to no section.
  /^\[[0-9]/ { inside = 0 }
  inside { print }
' CHANGELOG.md > "$out"

if [ ! -s "$out" ]; then
  echo "release-notes: CHANGELOG.md has no section for $version" >&2
  rm -f "$out"
  exit 1
fi

# These notes replace goreleaser's commit list rather than sitting above it, so the way back
# to the commits has to be restored by hand.
prev=$(git tag --sort=-version:refname | grep -vx "v${version}" | head -1)
if [ -n "$prev" ]; then
  printf '\n---\n\n**Full commit list:** https://github.com/gumeniukcom/contactshq/compare/%s...v%s\n' \
    "$prev" "$version" >> "$out"
fi

echo "release-notes: $(wc -l < "$out" | tr -d ' ') lines for $version"
