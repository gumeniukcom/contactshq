# Tasks: Spec Kit Adoption

**Input**: `specs/000-speckit-adoption/spec.md`

**Prerequisites**: none. This spec has no `plan.md`: the design was settled in the spec itself
and the work was small enough that a separate planning artefact would have been ceremony.

**Organization**: grouped by the user story each task serves, so a story can be finished and
checked on its own.

## Format: `[ID] [P?] [Story] Description`

`[P]` marks tasks that touch disjoint files and may run in parallel.

## Phase 1: Ownership (User Story 1, 2 — P1)

**Goal**: every tracked path resolves to exactly one spec, and the build fails when that stops
being true.

- [x] T001 [US1] Install the spec-kit scaffold by copying the sibling tree — `.specify/`,
      `.claude/skills/speckit-*/` (`d31c2c9`)
- [x] T002 [US1] Write `.specify/memory/constitution.md` (`d31c2c9`)
- [x] T003 [US1] Write the eight retrospective product specs `001`–`008` (`d31c2c9`)
- [x] T004 [US1] `.gitignore` allowlist so the skills are committable; `.dockerignore`
      exclusions (`d31c2c9`)
- [x] T005 [US2] `internal/speckit` — ownership, house shape, cited-enforcer and English checks
      (`79bc642`)
- [x] T006 [US2] `make specs`, and confirm CI runs it inside `go test ./...` (`79bc642`)
- [x] T007 [US2] `specs/UNCLAIMED.md` — the argued exemption list (`79bc642`)
- [x] T008 [US2] Audit untracked-but-unignored files too, so the gate is useful before a
      commit (`356df1e`)
- [x] T009 [US2] Read exemptions from UNCLAIMED.md's table column only — prose granting an
      exemption had silently excused all 106 files under `web/src` (`b8b65d4`)

**Checkpoint**: 375 paths, one owner each, enforced.

## Phase 2: Readability (User Story 4 — P2)

**Goal**: a newcomer can use the tree without reading the tooling, and knows which document
owns which kind of statement.

- [x] T010 [US4] `specs/README.md` — artefact vocabulary, status meanings, numbering rules
      (`0f4513c`)
- [x] T011 [US4] `CLAUDE.md` points at it, above every other section (`0f4513c`)
- [x] T012 [US4] `specs/BACKLOG.md` — the triage of all 184 Known Divergences (`b8b65d4`)

## Phase 3: Session selection (User Story 3 — P2)

**Goal**: a `/speckit-*` command acts on the spec you meant.

- [x] T013 [US3] `make specs-use SPEC=<number-or-slug>` writes `.specify/feature.json` in the
      shape `common.sh` reads; ambiguity is an error, never a guess
- [x] T014 [US3] `make specs-current` prints the selection — state you can set and cannot see
      is state you will get wrong

## Phase 4: Splitting shared files (User Story 1 — P1)

**Goal**: no file needs two owners.

- [x] T015 [P] [US1] Split `internal/carddav/server.go` — auth surface to
      `internal/carddav/auth.go`, owned by 001; transport stays with 004 (FR-018)
- [x] T016 [P] [US1] Split `cmd/server/cli.go` — `set-password` to `cmd/server/set_password.go`,
      owned by 001; dispatch stays with 008 (FR-020)
- [x] T017 [P] [US1] Split `cmd/server/startup.go` — `pruneSyncRuns` to
      `cmd/server/startup_sync.go`, owned by 006; backup halves stay with 008 (FR-019)

## Remaining

Nothing in this spec is unbuilt. Two decisions were deliberately left open rather than
implemented, and both belong to `specs/BACKLOG.md` rather than here:

- Whether a shared registry such as `web/src/router/index.ts` gets one owner of record or an
  explicit exception to Principle VII. Today it has an owner and a cross-reference, which works;
  the question is whether that is the rule or the workaround.
- Whether the toolkit pin `0.15.1.dev0` — a development build with no recorded provenance — is
  the authoritative fork for this repository.

## Dependencies

Phase 1 blocks everything: without ownership there is nothing for the other phases to describe.
Phase 4 depends on T005, because the split is only worth doing once something checks the result —
and indeed the gate demanded owners for all three new files the moment they existed, which is the
acceptance signal for that phase.

Phases 2 and 3 are independent of each other and of Phase 4.
