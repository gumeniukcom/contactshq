# The spec tree

This directory states what ContactsHQ is *supposed* to do. The code states what it does. When
they disagree, one of them is a bug — and which one is a decision somebody has to make
deliberately, which is the whole reason this tree exists.

Nine specifications cover everything shipped through v0.5.0. Eight of them were written
**retrospectively**, by reading the implementation, so they carry a standing hazard: a spec
written after the fact can quietly turn an accident into a stated requirement. Two habits are
what keep that honest, and both are enforced — see *The gate* below.

## What is here

| Path | What it is |
|---|---|
| `000-speckit-adoption/` | The tree's own spec: ownership rules, numbering, gates. `partial` — its `## Status` says which half is built |
| `001`–`008` | One domain each: identity, contacts, vCard, CardDAV, bulk transfer, sync, duplicates, runtime |
| `UNCLAIMED.md` | The argued list of paths no spec owns *on purpose* |
| `../.specify/memory/constitution.md` | The rules that outrank convenience. Read it first |
| `../.specify/templates/overrides/spec-template.md` | The house template. `resolve_template()` prefers it over upstream's |

## Reading a spec

The house template adds six sections after the upstream ones. Three carry most of the weight:

- **`## Code Paths`** — the authoritative ownership list. Exactly one spec owns a path. This is
  the answer to "who says what this file is supposed to do?", and it is a lookup, not a search.
- **`## Known Divergences`** — where shipped behaviour differs from stated intent, on purpose or
  otherwise. **A spec whose Known Divergences is empty should be read with suspicion, not
  confidence.** Across the eight product specs there are over 150 entries; that is the section
  working, not a sign of a bad codebase.
- **`## Enforced By`** — the tests that make the claims true. A requirement with no enforcer is
  either review-only, and says so, or a gap, and says that louder.

The header carries `Kind:` (`journey`, `component`, `meta`), `Status:` and the constitution
version the spec was written against.

## Status vocabulary

Exactly three values, because a fourth would be invented per-author and mean nothing:

| Status | Meaning |
|---|---|
| `draft` | Unfinished. Content assertions are **waived**, so a half-written spec never reddens the build. Ownership assertions still apply — a draft spec still claims paths, or the map has holes exactly where work is happening |
| `shipped` | The described behaviour is in the released software |
| `partial` | Some of it is. The spec body MUST say which parts, item by item, in `## Status` |

## Adding a spec

1. Take the highest existing number **plus one**. `009` is next.
2. The number is **permanent** once merged. It is the spec's identifier: things link to it, and
   renumbering breaks those links for no gain.
3. If two branches both claim `009`, **the branch being rebased is renumbered**, never the one
   already merged. This is the same rule migrations follow — numbers are assigned at merge time,
   not planning time — and the project has been bitten by the migration version of it before.
4. Claim your files in `## Code Paths`, or the gate fails. Claim them per file: a bare directory
   claim on `internal/handler`, `internal/service`, `internal/repository`, `internal/domain` or
   `web/src` is refused, because it would silently adopt everything added there later and
   permanently disarm the check.

## Where a fact goes

`.specify/memory/constitution.md` carries the authoritative table under **"Where a fact lives"**,
and it is not restated here — two copies of an anti-duplication rule would be a joke at its own
expense. In outline: the constitution owns rules that constrain future work, specs own what a
capability must do and where it diverges, `CLAUDE.md` owns traps for an agent editing the repo,
`README.md` owns what an operator must know, `docs/` owns deep explanation of one subsystem, and
`CHANGELOG.md` owns what changed in a release.

Where the same fact legitimately appears twice, the constitution names which copy is
authoritative: **the code wins, and whichever document was wrong is fixed in the same change that
discovered it.**

## The gate

`internal/speckit` is a package of tests, imported by nothing, that fails the build when the tree
stops being true. Run it alone with `make specs`; CI runs it inside `go test ./...`.

It checks that every path git would keep is owned by exactly one spec or listed in
`UNCLAIMED.md`; that no dense tree is claimed bare; that claims are literal paths (a
`{up,down}.sql` brace once hid fourteen migrations); that every spec keeps the house shape and is
written in English; that every test named in `## Enforced By` exists; and that a shipped spec
does not leave `## Known Divergences` blank.

When it fails on an unowned path there are exactly two honest fixes: claim it in a spec, or add
it to `UNCLAIMED.md` **with the reason**. There is no third option, and that is deliberate.

**What the gate cannot check** is whether a spec's prose is true. It reads structure. `## Status`
in particular is maintained by hand, and spec 000 spent two days claiming its own gate was
unbuilt after the gate shipped.
