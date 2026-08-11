# Feature Specification: Spec Kit Adoption

Kind: meta
Status: shipped
Constitution: v1.0.0

Written against the tree at `23a167c` (`v0.4.0`) and amended as the adoption landed, which it now
has: every requirement below cites a file that can be read today. `## Status` lists what shipped
and in which commit, `## Known Divergences` states what the tree still cannot check about itself,
and `tasks.md` beside this file records the order it was built in. This is the only spec in the
tree that carries a `tasks.md`.

## User Scenarios & Testing *(mandatory)*

The "users" of this feature are the maintainer and the AI agents working in this repository.
There is no runtime behaviour and no end-user surface.

### User Story 1 - Find the spec that owns the file I am about to change (Priority: P1)

A contributor (human or agent) opens `internal/service/backup.go` and needs to know which
specification states the intended behaviour, so the change is checked against a written
intent rather than against whatever the code happens to do.

**Why this priority**: This is the entire reason the tree exists. Without a path→spec lookup
the specs are prose that nobody consults, and the second the code moves they are wrong. Every
other story in this feature exists to keep this one true.

**Independent Test**: With only `specs/` present and no lint package, ask "who owns
`internal/service/backup.go`?" and answer it by reading exactly one `## Code Paths` section.

**Acceptance Scenarios**:

1. **Given** any tracked file in the repository, **When** a contributor searches the
   `## Code Paths` sections of `specs/*/spec.md`, **Then** exactly one spec claims it, or
   `specs/UNCLAIMED.md` lists it with a written reason.
2. **Given** a file inside one of the dense trees (`internal/handler`, `internal/service`,
   `internal/repository`, `internal/domain`, `web/src`), **When** the contributor looks it up,
   **Then** it is claimed by an explicit file entry, never by a bare directory claim on the
   tree — those trees hold 19, 35, 36, 17 and 106 entries respectively and serve several
   domains each (verified by `ls` / `find`).
3. **Given** a file whose behaviour spans two domains, **When** ownership is assigned,
   **Then** the file is split so each part has a single owner (see FR-018..FR-020), rather
   than being claimed twice.

---

### User Story 2 - The map fails the build when it stops being true (Priority: P1)

A contributor adds `internal/service/webhook.go` and forgets to claim it. CI fails with a
message naming the unowned path and the two ways to resolve it.

**Why this priority**: A documentation map with no enforcement decays within weeks. The
enforcement is what separates this from the `docs/` directory the project already has. It is
P1 alongside Story 1 because Story 1 without Story 2 is a one-off snapshot.

**Independent Test**: Add an empty `.go` file in a claimed tree, run the lint target, observe
a non-zero exit naming the file. Delete it, observe a pass.

**Acceptance Scenarios**:

1. **Given** a tracked path claimed by no spec and absent from `specs/UNCLAIMED.md`,
   **When** the ownership check runs, **Then** it fails and names the path.
2. **Given** a path claimed by two specs, **When** the check runs, **Then** it fails and
   names both claimants.
3. **Given** a spec with `Status: draft`, **When** the check runs, **Then** content
   assertions against that spec are skipped, so a half-written spec never reddens the build.
4. **Given** a path listed in `specs/UNCLAIMED.md` with a reason, **When** the check runs,
   **Then** it passes — the deferral is recorded, not hidden.

---

### User Story 3 - Run a `/speckit-*` command against the spec I mean (Priority: P2)

A contributor runs `/speckit-plan` and it operates on the spec they intended, not on whatever
spec the last session happened to touch.

**Why this priority**: A real footgun, but it costs a wasted command rather than a wrong
merge. Below the two integrity stories.

**Independent Test**: Point the session at spec A, run a planning command, confirm the output
lands in spec A's directory; repoint to B and repeat.

**Acceptance Scenarios**:

1. **Given** a fresh clone or a new machine, **When** a contributor runs `/speckit-plan`
   without first selecting a spec, **Then** the session-state file is absent or stale and the
   selection step (`make specs-use`) is the documented prerequisite.
2. **Given** `make specs-use <slug>` has been run, **When** any of `/speckit-plan`,
   `/speckit-tasks`, `/speckit-analyze`, `/speckit-checklist`, `/speckit-implement` runs,
   **Then** it acts on that spec.
3. **Given** two machines sharing the repository, **When** each selects a different spec,
   **Then** neither overwrites the other's selection in version control — the state file is
   per-machine and untracked (`.gitignore:68` ignores `.specify/feature.json`).

---

### User Story 4 - Know which document owns a statement (Priority: P2)

A contributor has a fact to record — a new env var, a breaking change, a rule about
migrations — and needs to know where it goes, without duplicating it in four places that
then drift.

**Why this priority**: The repository already carries six documentation surfaces
(`CLAUDE.md`, `README.md`, `CHANGELOG.md`, `docs/sync.md`, `docs/reverse-proxy.md`, and now
`specs/`). Adding a seventh without an ownership rule makes the drift worse, not better.

**Independent Test**: Take three recent facts from `CHANGELOG.md` v0.4.0 and place each
using the table in FR-002 alone.

**Acceptance Scenarios**:

1. **Given** the document-ownership table, **When** a contributor has a fact to record,
   **Then** the table names exactly one primary home for it.
2. **Given** a fact that legitimately appears in two documents, **When** it changes,
   **Then** the table names which copy is authoritative.

---

### User Story 5 - Add a new spec without colliding with a parallel branch (Priority: P3)

Two branches each add a spec. Neither silently reuses the other's number.

**Why this priority**: Low frequency, cheap to fix at rebase time, but the failure mode
(two different `009-` directories merged into one tree) is confusing enough to be worth a
written rule. The project has been bitten by exactly this shape of problem before, with
migration numbers (`CLAUDE.md`, "Numbers are assigned at merge time, not planning time").

**Independent Test**: Create a spec on a branch, create another on a second branch, rebase
the second, confirm the rule produces distinct numbers.

**Acceptance Scenarios**:

1. **Given** the highest existing spec number is N, **When** a new spec is created,
   **Then** it takes N+1.
2. **Given** a spec has been merged, **When** anything about it changes, **Then** its number
   never changes — it is a permanent identifier.
3. **Given** a branch's spec number collides on rebase, **When** the branch is rebased,
   **Then** the branch's spec is renumbered, not the merged one.

---

### User Story 6 - Amend a template without forking upstream (Priority: P3)

The project needs a `## Code Paths` section that upstream Spec Kit does not have. A future
`specify` upgrade must not silently discard it.

**Why this priority**: Only bites at upgrade time, and the damage is recoverable from git
history. But an amendment placed in the upstream-owned file is guaranteed to be lost.

**Independent Test**: Place an amended template in the overrides directory, confirm the
resolver prefers it over the upstream copy.

**Acceptance Scenarios**:

1. **Given** an amended template exists in the overrides directory, **When** a `/speckit-*`
   command resolves a template, **Then** the override is used — `resolve_template()` checks
   `$repo_root/.specify/templates/overrides/<name>.md` first and returns on a hit
   (`.specify/scripts/bash/common.sh:406-413`).
2. **Given** the upstream toolkit is re-run or upgraded, **When** it rewrites its own
   template files, **Then** the overrides directory is untouched.

---

### Edge Cases

- **What happens when a tracked path is claimed by no spec?** The ownership check must fail
  and name exactly two remedies: claim it, or record it in `specs/UNCLAIMED.md` with a reason
  (FR-010). Silence is not an option, because a silently unowned path is indistinguishable
  from a deliberate deferral.
- **What happens when a path is claimed by two specs?** It fails and names both claimants
  (FR-008). It is never resolved by preferring the lower spec number — two claims mean the
  boundary itself is wrong.
- **What happens when a spec is still being written?** `draft` waives content assertions but
  **not** ownership assertions (FR-013): a draft spec still claims paths, or the map has holes
  exactly where work is happening.
- **What happens to a path that is generated, vendored or ignored?** `internal/web/static/spa`
  is produced by the frontend build and `.specify/feature.json` is per-machine and untracked;
  neither is a source of intent, so the check operates on tracked paths only.
- **What happens when a spec is deleted?** Its claims vanish with it and every path it owned
  becomes unowned, which the check reports on the next run. That is the intended behaviour: a
  removed spec must not leave its map entries behind.

## Requirements *(mandatory)*

### Functional Requirements

> **Traceability note.** Each parenthetical names the artefact the requirement lives in and
> marks it either `[present, uncommitted]` — it exists in the working tree and can be read
> today, but is not part of any commit — or `[not built]`. FR-018..FR-023 constrain files that
> were already committed at `23a167c` and cite them with line numbers. FR-024..FR-025 are
> coverage requirements on the rest of the tree.

**The tree and its meaning**

- **FR-001**: The repository MUST carry a `specs/` directory whose purpose, artefact
  vocabulary and gates are stated in `specs/README.md`, so a newcomer can understand the tree
  without reading the tooling. (`specs/README.md`)

- **FR-002**: `specs/README.md` MUST carry a document-ownership table assigning exactly one
  primary home to each kind of statement. The constitution already carries the authoritative
  copy of this table (`.specify/memory/constitution.md`, "Where a fact lives"); `specs/README.md`
  points at it rather than restating it. The six surfaces and their roles:

  | Document | Owns | Verified |
  |---|---|---|
  | `.specify/memory/constitution.md` | Non-negotiable engineering rules that outrank any single spec | exists [present, uncommitted]; seven principles, "Where a fact lives", "Language", "Governance" |
  | `specs/NNN-*/spec.md` | What one domain does and why, plus its `## Code Paths` ownership claim | exists [present, uncommitted]; `000`–`008` |
  | `README.md` | How an operator installs, configures and runs the system | exists; `## Getting started`, `## Configuration`, `## Development`, `## Testing` |
  | `docs/*.md` | Deep operational explanation of one subsystem | exists; `docs/sync.md`, `docs/reverse-proxy.md` |
  | `CHANGELOG.md` | What changed in each release, and what breaks | exists; `[0.4.0]`, `[0.3.0]`, `[0.2.0]`, `[0.1.0]` |
  | `CLAUDE.md` | Conventions and gotchas an agent must know before editing | exists; "Conventions & gotchas" |

- **FR-003**: Where the same fact appears in two documents, the table MUST name which copy is
  authoritative. Satisfied in the constitution ("When these conflict, the code wins, and
  whichever document was wrong gets fixed in the same change that discovered it");
  `specs/README.md` MUST NOT contradict it. (`specs/README.md`)

**Numbering and identity**

- **FR-004**: A new spec directory MUST take the highest existing number plus one.
  (`specs/README.md`)
- **FR-005**: A spec's number MUST be permanent once merged; it is the spec's identifier.
  (`specs/README.md`)
- **FR-006**: When two branches claim the same number, the branch being rebased MUST be
  renumbered. This mirrors the migration-numbering rule already enforced by convention
  (`CLAUDE.md`, "Numbers are assigned at merge time, not planning time"; 25 migration pairs
  verified in `migrations/`, and the same rule restated as constitution Principle I).
  (`specs/README.md`)

**Ownership**

- **FR-007**: Each `specs/NNN-*/spec.md` MUST carry a `## Code Paths` section, and that
  section MUST be the authoritative statement of which code the spec owns. No other file may
  assert ownership. Restated as constitution Principle VII.
  (`.specify/templates/overrides/spec-template.md:92-106` [present, uncommitted])
- **FR-008**: Every tracked path MUST be claimed by exactly **one** spec. Two claims on the
  same path is a failure, not a merge. (`internal/speckit/ownership_test.go`:
  `TestEveryTrackedPathIsClaimed`, `TestNoPathIsClaimedTwice`)
- **FR-009**: A `## Code Paths` entry MUST NOT be a bare directory claim on any of
  `internal/handler`, `internal/service`, `internal/repository`, `internal/domain`, `web/src`.
  Those trees are dense and cross-domain — verified counts 19, 35, 36, 17 entries and 106 files
  respectively; `internal/service` alone holds auth, contacts, backup, import/export,
  duplicates, merge, Google OAuth and sync-conflict logic. Entries there MUST be per file or
  per subpackage. (`internal/speckit/ownership_test.go`: `TestNoBareDenseDirectoryClaim`;
  the tree list is fixed by constitution
  Principle VII — see Known Divergences for the earlier four-tree wording this replaces)
- **FR-010**: An unowned path MUST fail the ownership check. The failure MUST be resolvable in
  exactly two ways: claim the path in a spec's `## Code Paths`, or list it in
  `specs/UNCLAIMED.md` with a written reason. (`internal/speckit/ownership_test.go`:
  `TestEveryTrackedPathIsClaimed`; `specs/UNCLAIMED.md`)
- **FR-011**: The ownership check MUST be runnable locally through a `make` target and MUST
  run in CI. `make specs` runs it alone (`Makefile`); CI runs the same checks inside
  `go test ./... -count=1 -race` (`.github/workflows/ci.yml`). No separate CI step was added —
  it would run the package twice for the same signal.

**Status lifecycle**

- **FR-012**: A spec's `Status` MUST be one of `draft`, `shipped`, or `partial`. `draft` MUST
  waive content assertions, so an unfinished spec never fails the build. `partial` MUST state
  in the spec body what is and is not built. The template already encodes this
  (`.specify/templates/overrides/spec-template.md:4,25-26,88-90`).
  (`specs/README.md`; `internal/speckit/shape_test.go`: `TestSpecHeaderIsWellFormed`)
- **FR-013**: Ownership assertions (FR-008, FR-009, FR-010) MUST apply regardless of status —
  a draft spec still claims paths, or the map has holes exactly where work is happening.
  (`internal/speckit/ownership_test.go` — the ownership tests read no Status field)

**Toolchain**

- **FR-014**: The toolkit version MUST be pinned and recorded, together with the init options
  it was materialised with. Verified on disk: `speckit_version` `0.15.1.dev0`, `script` `sh`,
  `here: true`, `ai: claude`, `ai_skills: true`, `feature_numbering: sequential`
  (`.specify/init-options.json`), and `invoke_separator` `-` — hence `/speckit-plan`, not
  `/speckit.plan` (`.specify/integration.json`). [both present, uncommitted]
- **FR-015**: Command surface: ten skills at `.claude/skills/speckit-*/SKILL.md`, each invoked
  as `/speckit-<verb>`. Verified on disk, all ten carrying exactly one `SKILL.md`: `analyze`,
  `checklist`, `clarify`, `constitution`, `converge`, `implement`, `plan`, `specify`, `tasks`,
  `taskstoissues`. Two of these (`converge`, `taskstoissues`) are beyond the upstream set,
  which is why the count is ten and not eight. [present, uncommitted; unblocked by FR-021]
- **FR-016**: Session state MUST live in `.specify/feature.json` and MUST be treated as
  per-machine. A spec MUST be selected with `make specs-use <slug>` before every
  `/speckit-plan`, `/speckit-tasks`, `/speckit-analyze`, `/speckit-checklist` or
  `/speckit-implement`. `.gitignore:68` ignores `.specify/feature.json` with the reason written
  above it. The target takes the slug as a variable — `make specs-use SPEC=004` — because a bare
  positional argument to `make` is a phony target, and a prefix that matches more than one spec
  is an error rather than a guess. `make specs-current` prints the selection. (`Makefile`)
- **FR-017**: Template amendments MUST live in `.specify/templates/overrides/`, because
  `resolve_template()` checks that directory before any preset or upstream copy and returns on
  the first hit (`.specify/scripts/bash/common.sh:406-413`). Editing the upstream template in
  place is forbidden — an upgrade overwrites it.
  (`.specify/templates/overrides/spec-template.md` [present, uncommitted])

**Prerequisite changes to files that exist today**

- **FR-018**: `internal/carddav/server.go` MUST be split so that credential verification is
  separable from WebDAV request routing. Done: the authentication surface moved to
  `internal/carddav/auth.go` (owned by 001), leaving the transport surface with 004. It was one
  file of 328 lines, holding both the
  transport surface (`ServeHTTP:71`, `serveSyncExtensions:118`, `isAddressBookPath:145`) and
  the authentication surface (`authenticate:160`, `verifyCredentials:190`,
  `verifyAppPassword:221`, `VerifyArgon2id:260`). The CardDAV domain and the auth domain
  cannot both claim this path under FR-008.
- **FR-019**: `cmd/server/startup.go` MUST be split along the backup/sync seam. Done:
  `pruneSyncRuns` moved to `cmd/server/startup_sync.go` (owned by 006). It was one file of 168
  lines, holding `reconcileInterruptedRuns:27` and `catchUpMissedBackups:78`
  (backup domain) alongside `pruneSyncRuns:53` (sync domain).
- **FR-020**: `cmd/server/cli.go` MUST be split so the `set-password` command is separable
  from subcommand dispatch. Done: `set-password` and its helpers moved to
  `cmd/server/set_password.go` (owned by 001). It was one file of 257 lines, holding the
  dispatch table
  (`subcommands:34`, `looksLikeSubcommand:43`, `runCLI:49`, `parseInterleaved:69`,
  `printUsage:84`) alongside `runSetPassword:138`, `setPasswordEpilogue:200` and
  `readNewPassword:214`, which belong to the auth domain.
- **FR-021**: `.gitignore` MUST stop excluding the committed command skills. At `23a167c` line
  48 was a bare `.claude/`, which ignored the entire directory and made the skills
  uncommittable. Now satisfied in the working tree by an allowlist — `!.claude/`,
  `.claude/*`, `!.claude/skills/` (`.gitignore:60-62`) — written with a star rather than a
  trailing slash because Git cannot re-include a path under an excluded directory.
  [present, uncommitted]
- **FR-022**: `.dockerignore` MUST exclude `specs/` and `.specify/` from the build context.
  `Dockerfile:20` is `COPY . .`, and the build context does not honour `.gitignore`; this is
  the same hole that once leaked `configs/config.yaml` into the image (`.dockerignore:1-5`).
  Now satisfied in the working tree: `.dockerignore:16-18` excludes both under the comment
  "Spec-driven development artefacts: documentation, never inputs to the build", beside the
  existing `.git`, `.github`, `.gitignore`, `.claude`, `.serena` block. [present, uncommitted]
- **FR-023**: `CLAUDE.md` MUST point an agent at `specs/README.md` before it edits code, or
  the tree is invisible to the readers it was written for. The pointer sits directly under the
  one-line description, above every other section, and states what `CLAUDE.md` is *not* — the
  traps of editing, never a behavioural specification. (`CLAUDE.md`)

**Coverage**

- **FR-024**: Eight retrospective specs (`001`–`008`) MUST cover the shipped product surface,
  written in risk order with `003` and `006` first. Verified present in the working tree:
  `001-identity-and-credentials`, `002-contact-record-and-catalog`, `003-vcard-representation`,
  `004-carddav-service`, `005-bulk-transfer-and-backup`, `006-sync-engine-and-providers`,
  `007-duplicate-detection-and-merge`, `008-runtime-configuration-and-delivery`.
  [present, uncommitted]
- **FR-025**: A project constitution MUST record the rules that outrank any individual spec.
  Verified present in the working tree with seven principles: forward-only migrations, bulk
  repairs as commands, errors that leak nothing, memory-bounding limits versus policy limits,
  one writer per representation, a spec says where it lies, exactly one spec owns a path
  (`.specify/memory/constitution.md`, ratified 2026-08-07, v1.0.0). [present, uncommitted]

### Key Entities

- **Spec directory** — `specs/NNN-slug/`, containing `spec.md` and, for this spec only,
  `tasks.md`. `NNN` is permanent (FR-005). Attributes: number, slug, title, kind, status,
  code-path claims.
- **Code Paths claim** — a line inside a spec's `## Code Paths` naming one repository path.
  Relationship: many claims per spec; exactly one claim per path across the whole tree
  (FR-008).
- **Unclaimed entry** — a line in `specs/UNCLAIMED.md` naming a path plus a written reason.
  Mutually exclusive with a Code Paths claim for the same path.
- **Constitution** — `.specify/memory/constitution.md`. Rules that apply across all specs and
  outrank any one of them. Carries a version line that every spec header cites.
- **Session state** — `.specify/feature.json`. Per-machine, untracked, names the currently
  selected spec. Not shared state; two machines hold independent values.
- **Template override** — a file in `.specify/templates/overrides/` shadowing an upstream
  template of the same name (FR-017).
- **Ownership checker** — the `internal/speckit` Go package. Runs as a normal Go test so it
  needs no new CI tooling: `.github/workflows/ci.yml:26` already runs
  `go test ./... -count=1 -race` (verified).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of tracked repository paths resolve to exactly one owning spec or to a
  reasoned entry in the unclaimed list. The surface to be covered, measured at `23a167c`:
  **112 non-test Go files**, **75 Go test files**, **106 files under `web/src`**, **25
  migration pairs** (`find`/`ls` counts).
- **SC-002**: Zero paths are claimed by two specs.
- **SC-003**: Zero bare directory claims exist on the dense trees — `internal/handler`
  (19 entries), `internal/service` (35), `internal/repository` (36), `internal/domain` (17),
  `web/src` (106 files).
- **SC-004**: A contributor can name the owning spec for any changed file by reading spec
  headers alone, without opening a source file.
- **SC-005**: A newly added source file that no spec claims causes the ownership check to fail
  on its first run, and the failure message names the file and both remedies.
- **SC-006**: A spec whose status is `draft` never causes a check failure for incomplete
  content; it can still fail for ownership gaps.
- **SC-007**: Re-running or upgrading the toolkit leaves every file in
  `.specify/templates/overrides/` byte-identical.
- **SC-008**: The eight product domains (`001`–`008`) are all present, with `003` and `006`
  written first.
- **SC-009**: The ownership check adds no new CI dependency — it runs inside the existing
  `go test ./...` step.
- **SC-010**: Neither `specs/` nor `.specify/` appears in the built container image.

No performance criterion applies. The only cost is checker runtime, and against a surface of
a few hundred files it is not a number worth committing to.

## Assumptions

- **Single maintainer, single machine per session.** `.specify/feature.json` being per-machine
  and untracked is safe under this assumption and unsafe if two people ever share a checkout
  concurrently — matching the project's existing single-instance stance (`CLAUDE.md`,
  "Startup reconciliation is bounded by process start time … Real leases are the
  multi-instance answer and this project does not support that configuration").
- **The primary reader is an AI agent, not a newcomer.** This is why enforcement is mechanical
  and why `CLAUDE.md` must link the tree (FR-023): an agent that is not told to read
  `specs/README.md` will not.
- **A retrospective spec's value is its traceability, not its completeness.** A spec that
  honestly records an open question is more useful than one that rationalises an accident into
  a requirement. This spec applies that rule to itself; the questions it cannot close are in
  `## Known Divergences`, not smoothed away.
- **The eight product domains partition the shipped surface without gaps.** Not verifiable
  from this spec — `000` does not know the other eight boundaries. If they do not partition
  it, the difference lands in `specs/UNCLAIMED.md`, which is the designed outcome rather than
  a failure.
- **Out of scope: any shipped product behaviour.** Contacts, authentication, CardDAV, sync,
  backup, import/export, duplicates and the web UI are the subject of specs `001`–`008`. This
  spec covers only the specs tree, the toolchain that operates on it, and the gates that keep
  it honest.
- **Depends on Spec Kit `0.15.1.dev0`**, materialised in place (`here: true`) with `sh`
  scripts and `-` as the invoke separator (FR-014). No other new tooling: the checker is a Go
  test package and the Go toolchain is already in CI (`.github/workflows/ci.yml:26`).
- **Ordering the work.** FR-018, FR-019 and FR-020 (the three file splits) block FR-008;
  FR-021 blocked FR-015 and is now satisfied; the constitution (FR-025) landed before the
  eight retrospective specs so they could defer to it instead of restating the same rules
  eight times.

## Status

`shipped`, in `d31c2c9` (scaffold, constitution, nine specs), `79bc642` (the ownership gate),
`356df1e` (the gate's audit surface), `0f4513c` (`specs/README.md` and the `CLAUDE.md` pointer)
and `b8b65d4` (the backlog and the UNCLAIMED.md parser fix), on top of `v0.4.0`.

**Built and enforced:**

- `.specify/` — `memory/constitution.md` (v1.0.0), `templates/`,
  `templates/overrides/spec-template.md`, `scripts/bash/`, `init-options.json`,
  `integration.json` (FR-007, FR-014, FR-017, FR-025)
- `.claude/skills/speckit-*/SKILL.md` — all ten (FR-015)
- `specs/000-speckit-adoption/spec.md` and the eight product specs `001`–`008` (FR-024)
- The `.gitignore` allowlist (FR-021) and the `.dockerignore` exclusions (FR-022)
- `specs/UNCLAIMED.md`
- `specs/BACKLOG.md` (FR-010)
- `internal/speckit/` — the ownership gate (FR-008, FR-009, FR-010, FR-013), plus house-shape,
  English-only and cited-enforcer checks the original requirements did not ask for
- `make specs`, and CI running the same checks inside `go test ./...` (FR-011)
- `specs/README.md` (FR-001, FR-002, FR-003, FR-004, FR-005, FR-006, FR-012)
- The `CLAUDE.md` pointer at `specs/README.md` (FR-023)

- `make specs-use` / `make specs-current` (FR-016), writing the `.specify/feature.json` shape
  `common.sh` reads
- The three file splits (FR-018, FR-019, FR-020): the auth surface to `internal/carddav/auth.go`
  and `set-password` to `cmd/server/set_password.go`, both owned by 001; `pruneSyncRuns` to
  `cmd/server/startup_sync.go`, owned by 006
- `specs/000-speckit-adoption/tasks.md`

**Not built:** nothing. Two questions were deliberately left open rather than answered — who is
the owner of record for a shared registry, and whether the `0.15.1.dev0` pin is authoritative for
this repository — and both live in `specs/BACKLOG.md` rather than here.

**What the gate does not check.** It reads `## Code Paths`, the section set, the header and the
cited test names. It does not and cannot check that this section is true — the failure this spec
already suffered, when it went on claiming `UNCLAIMED.md` and `internal/speckit` were unbuilt for
two days after they shipped. `## Status` is maintained by hand and should be read with that in
mind.

## Code Paths

Owned by this spec:

- `specs/README.md`
- `specs/UNCLAIMED.md`
- `specs/000-speckit-adoption/`
- `.specify/memory/constitution.md`
- `.specify/templates/`
- `.specify/scripts/bash/`
- `.specify/init-options.json`
- `.specify/integration.json`
- `.specify/integrations/claude.manifest.json`
- `.specify/integrations/speckit.manifest.json`
- `.specify/workflows/workflow-registry.json`
- `.specify/workflows/speckit/workflow.yml`
- `.specify/memory/.constitution-template.json`
- `specs/UNCLAIMED.md`
- `.claude/skills/speckit-analyze/SKILL.md`
- `.claude/skills/speckit-checklist/SKILL.md`
- `.claude/skills/speckit-clarify/SKILL.md`
- `.claude/skills/speckit-constitution/SKILL.md`
- `.claude/skills/speckit-converge/SKILL.md`
- `.claude/skills/speckit-implement/SKILL.md`
- `.claude/skills/speckit-plan/SKILL.md`
- `.claude/skills/speckit-specify/SKILL.md`
- `.claude/skills/speckit-tasks/SKILL.md`
- `.claude/skills/speckit-taskstoissues/SKILL.md`
- `internal/speckit/`

## References

Touched by this spec, owned elsewhere:

- `cmd/server/startup.go` — FR-019 requires it split; each half is claimed by its product spec
- `cmd/server/cli.go` — FR-020, same
- `internal/carddav/server.go` — FR-018, same
- `Makefile` — FR-011 and FR-016 add `specs-*` targets
- `.gitignore` — FR-021 adds the `.claude/skills` allowlist
- `.dockerignore` — FR-022 adds `specs` and `.specify`
- `CLAUDE.md` — FR-023 adds the pointer to `specs/README.md`

## Enforced By

The ownership rules are enforced. The documentation-hygiene rules are not, and are listed as
gaps below rather than dressed up as conventions.

- `TestEveryTrackedPathIsClaimed` (`internal/speckit/ownership_test.go`) — FR-008, FR-010.
  Every tracked path under `cmd/`, `internal/`, `migrations/`, `web/src/`, `.specify/` and
  `.claude/` resolves to exactly one spec or to a reasoned entry in `specs/UNCLAIMED.md`.
- `TestNoPathIsClaimedTwice` (`internal/speckit/ownership_test.go`) — FR-008. Checks both
  identical claims and overlapping ones of different shapes, which is the form that hid a
  duplicate claim on `.specify/templates/overrides/`.
- `TestNoBareDenseDirectoryClaim` (`internal/speckit/ownership_test.go`) — FR-009.
- `TestClaimsAreLiteralPaths` (`internal/speckit/ownership_test.go`) — added after seven
  brace-expansion entries (`migrations/013_x.{up,down}.sql`) silently owned nothing.
- `TestSpecHeaderIsWellFormed`, `TestSpecCarriesTheHouseSections`,
  `TestSpecHasNoTemplatePlaceholders` (`internal/speckit/shape_test.go`) — FR-012 and the
  section set the checks above read.
- `TestSpecArtefactsAreEnglish` (`internal/speckit/shape_test.go`) — constitution "Language".
- `TestCitedEnforcersExist` (`internal/speckit/enforcers_test.go`) — every `Test…` named in any
  spec's `## Enforced By` resolves to a real function, so a rename cannot silently downgrade a
  claim to fiction.
- `TestShippedSpecDeclaresItsDivergences` (`internal/speckit/enforcers_test.go`) —
  constitution Principle VI: a shipped spec may not leave `## Known Divergences` blank.
- `make specs` (`Makefile:25-26`) runs the gate locally; CI runs it as part of
  `go test ./... -count=1 -race` (`.github/workflows/ci.yml:26`), so FR-011 holds in both
  directions without a duplicated step.

Per constitution Principle VI, the requirements below are stated as gaps rather than as
review-only conventions, because a gap is what they are:

- SC-004..SC-006, SC-009 — no enforcer. These measure adoption over time (how often the map is
  consulted, whether specs stay current) and no test can observe them.
- FR-001..FR-006, FR-012, FR-023 — no enforcer. Blocked on `specs/README.md` and the
  `CLAUDE.md` edit, neither of which exists.
- FR-007, FR-014, FR-015, FR-017, FR-024, FR-025 — no automated enforcer; true by inspection
  of the files cited in each requirement. Nothing would notice if a file were deleted.
- FR-021 — no enforcer. Nothing asserts that `.claude/skills/speckit-*/SKILL.md` is actually
  tracked; the negation could regress silently and only the next fresh clone would find out.
- FR-022, SC-010 — no enforcer. The Docker job asserts only that `configs/config.yaml` did not
  reach the image (`.github/workflows/ci.yml:149-153`); nothing asserts the absence of
  `specs/` or `.specify/`, even though that assertion would be one more line in the same step.
- FR-018, FR-019, FR-020 — not applicable until the splits happen; nothing prevents the three
  files from growing further in the meantime.
- SC-007 — no enforcer, and none is cheap: it needs an actual toolkit upgrade to observe.

## Known Divergences

- **The entire tree is unenforced.** This is the largest divergence and it makes every
  ownership claim in `000`–`008` a statement of intent rather than a checked fact. Until
  `internal/speckit` exists, Principle VII is a convention, not a gate.
- **Nothing here is committed.** `.specify/`, `specs/` and `.claude/` are untracked and
  `.gitignore` / `.dockerignore` are modified but unstaged. A fresh clone of `23a167c` has
  none of this. Any claim in this spec about "the working tree" is a claim about one machine.
- **FR-009 and SC-003 originally named four dense trees; the constitution names five.** The
  earlier wording listed `internal/handler`, `internal/service`, `internal/repository` and
  `web/src/components`. Constitution Principle VII and the template comment
  (`.specify/templates/overrides/spec-template.md:97-101`) both list five: they add
  `internal/domain` (17 entries) and widen `web/src/components` (36 files) to all of `web/src`
  (106 files). The constitution wins, and FR-009/SC-003 above have been brought into line —
  but no shipped checker enforces either list, so the correction is textual only.
- **Shared files have no owner this model can express.** `Makefile`, `.gitignore`,
  `.dockerignore` and `CLAUDE.md` are each amended by this feature and owned by whichever spec
  owns build and release. Ownership granularity is the path, so the four lines this feature
  contributes to those files are invisible to the check. Still open: which of `001`–`008` owns
  `Makefile`, `Dockerfile*`, `docker-compose.yml`, `.goreleaser.yaml`, `.github/`, `.githooks/`
  and `scripts/`. This spec cannot assign them because it does not know the other eight
  domains' boundaries.
- **`.specify/workflows/` and `.specify/integrations/` are claimed by nobody, including this
  spec.** Verified present: `.specify/workflows/workflow-registry.json`,
  `.specify/workflows/speckit/workflow.yml`, `.specify/integrations/claude.manifest.json`,
  `.specify/integrations/speckit.manifest.json`. They fall inside this spec's subject matter
  but outside its `## Code Paths` list, so the first ownership run will flag them and they will
  need either a claim or an `UNCLAIMED.md` entry.
- **A `## Code Paths` list is a claim, not a proof of correctness.** The check can prove that
  every path is claimed exactly once. It cannot prove that the claiming spec describes what
  that file actually does. Content accuracy stays a human review property, permanently.
- **`specs/UNCLAIMED.md` will be abused if it is free.** The written reason is the only
  friction, and there is no mechanism for expiring entries. Open: should entries carry an owner
  and a review date, and should the check cap their number?
- **The pinned toolkit `0.15.1.dev0` is a development build.** Pinning to a `.dev0` version
  means the pin cannot be re-fetched from a released artefact if the local copy is lost. Open:
  is `0.15.1.dev0` reproducible from a tag or commit, and where is that recorded?
- **Nothing detects a spec left at `draft` indefinitely.** `draft` waiving content assertions
  is a deliberate weakening: it buys the ability to land a spec skeleton without breaking
  master, and it costs the guarantee that a merged spec is complete. Open: should the check
  warn on a spec that has been `draft` across more than one release?
- **The three file splits are deferred, and the deferral is visible in this spec's own
  sections.** `internal/carddav/server.go` (328 lines), `cmd/server/startup.go` (168) and
  `cmd/server/cli.go` (257) each carry two domains' worth of behaviour (verified by `wc -l`
  and the symbol lines cited in FR-018..FR-020). Listing them under `## References` records the
  problem; it does not solve it. Until they are split, three paths have two legitimate
  claimants each and Principle VII cannot hold for them.
- **`CLAUDE.md` still does not mention the tree.** FR-023 is unmet, so the primary reader this
  feature was built for — an agent editing this repo — is not told the specs exist.
- **There is no way to select a spec.** FR-016 requires `make specs-use <slug>` before every
  `/speckit-*` command, and the target does not exist. The prerequisite in User Story 3's
  acceptance scenarios is therefore documentation of an intended workflow, not a description
  of one that runs.
- **`tasks.md` does not exist.** The template reserves a forward-looking `tasks.md` for `meta`
  specs and this spec's own header promises one; `specs/000-speckit-adoption/` currently holds
  `spec.md` alone.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-11 | unreleased | Closed the last three requirements and promoted the spec to `shipped`. FR-016: `make specs-use SPEC=<number-or-slug>` writes the `.specify/feature.json` shape `common.sh` reads, with ambiguity an error rather than a guess, plus `make specs-current` — the wording moved from a positional argument to a variable because a bare positional to `make` is a phony target. FR-018/019/020: the three splits landed, so `internal/carddav/auth.go` and `cmd/server/set_password.go` belong to 001 and `cmd/server/startup_sync.go` to 006; the gate demanded owners for all three the moment they existed, which is that phase's acceptance signal. Added `tasks.md`. | — |
| 2026-08-07 | — | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | — | Rewritten to the house template: header replaced (Kind/Status/Constitution; `Feature Branch` and `Input` removed), `Dependencies` and `Out of Scope` folded into Assumptions, `Status`/`Code Paths`/`References`/`Enforced By`/`Known Divergences`/`Amendments` added in template order. Ownership narrowed to an explicit path list. Every admission moved out of Edge Cases into Known Divergences. Status `Draft` → `partial`. Five open questions closed against files now on disk (FR-014, FR-015, FR-017, FR-024, FR-025); four left open in Known Divergences. FR-009/SC-003 aligned with the constitution's five dense trees. Status vocabulary lowercased to match the template header. | — |
