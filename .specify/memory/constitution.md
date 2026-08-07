# ContactsHQ Constitution

This file holds the rules that outrank convenience. It is deliberately short: a principle
nobody enforces is worse than no principle, because it makes the document look complete.

Facts about how the system works do **not** belong here — see *Where a fact lives* below.

## Core Principles

### I. A migration is forward-only, so it can never need undoing

`.down.sql` files exist and **no code applies them**: `MigrateFS` globs `*.up.sql` only.
Rolling back means restoring a database dump. Therefore a migration may `CREATE TABLE`,
`CREATE INDEX`, or `ADD COLUMN` **with a DEFAULT**, and may not `DROP COLUMN`, rename,
add `NOT NULL` without a default, or use `CREATE INDEX CONCURRENTLY` (each file runs inside
`RunInTx`, and PostgreSQL rejects CONCURRENTLY in a transaction).

**No bulk `UPDATE` in a migration.** It runs on every install, with no dry run and no way to
decline. Data rewrites are explicit commands instead — see Principle II.

Migration numbers are assigned at merge time, not planning time. Two branches that both claim
`023` diverge across installs and leave `schema_migrations` in incompatible states.

### II. A bulk data repair is a command, never a migration

Such a command MUST default to a dry run, batch its writes, refuse to do half the job, and
say plainly that there is no undo. `reencode-vcards` is the worked example: `--apply` requires
`--reconcile-sync-state`, because rewriting cards moves `contacts.etag` while
`sync_states.local_etag` still holds the old value, and the next export would push the entire
address book to the remote provider.

### III. An error leaving the process says nothing the caller should not know

A `*fiber.Error` keeps its message; every other error is logged and answered with a fixed
`"internal server error"`. Driver messages and file paths are not a client's business.

Secrets are never accepted as command-line arguments — argv appears in `ps`,
`/proc/<pid>/cmdline`, shell history and `docker inspect`. Prompt without echo, or read stdin
behind an explicit `--stdin`.

`auth.jwt_secret` has no default and never will. Anyone holding it can mint admin tokens.

### IV. A limit that bounds memory is stated as such; one that does not is called policy

`server.max_body_bytes` is the only limit that bounds memory, because fasthttp reads a whole
body before any handler runs — N concurrent uploads cost N × that value. Per-route limits give
a clear 413 where a large body is meaningless. They are policy, not protection, and must be
documented as policy so nobody sizes capacity from them.

`server.write_timeout` MUST stay 0 and config validation refuses to start otherwise: restore
and import run synchronously inside the request, and a write deadline truncates an operation
that is still changing contacts.

### V. One writer per representation

All vCard encoding goes through `internal/vcard`. `EncodeCard` is the single writer and
`gvcard.NewEncoder` must not be called anywhere else, because escaping has to be chosen per
property type — uniform escaping corrupts URI values, list separators and structured values.

Changing the encoder moves `ContentHash` and `LocalETag` for affected cards, which is a
maintenance-window change, not a refactor.

### VI. A spec records what the code does, and says where it lies

These specs are retrospective: most were written after the software shipped. The standing
hazard is laundering an accident into a stated requirement.

Therefore every spec MUST carry `## Known Divergences`, and every functional requirement MUST
cite the code that implements it. A spec whose `Known Divergences` is empty is to be read with
suspicion, not confidence. `## Enforced By` names the tests that make the claims true; a
requirement with no enforcer is either review-only — say so — or a gap, and then say it louder.

### VII. Exactly one spec owns a path

`## Code Paths` is the authoritative ownership map. Two specs claiming one file means neither
is trusted when they disagree. For the dense trees — `internal/handler`, `internal/service`,
`internal/repository`, `internal/domain`, `web/src` — a bare directory claim is forbidden: it
auto-adopts everything added inside it later and permanently disarms the coverage gate.

## Where a fact lives

Duplication that drifts is how documentation dies. Each fact has exactly one owner:

| Document | Owns | Never contains |
|---|---|---|
| `.specify/memory/constitution.md` | Rules that constrain future work | Descriptions of current behaviour |
| `specs/NNN-*/spec.md` | What a capability must do, and where it diverges | Operator instructions, changelog entries |
| `CLAUDE.md` | Orientation and traps for an agent editing this repo | Full behavioural specification |
| `README.md` | What an operator must know to run and upgrade it | Internal design rationale |
| `docs/*.md` | Deep explanation of one subsystem | Anything a spec already states normatively |
| `CHANGELOG.md` | What changed in a release, and what breaks | Present-tense behaviour |

When these conflict, the code wins, and whichever document was wrong gets fixed in the same
change that discovered it.

## Language

Every artefact under `.specify/` and `specs/` is written in **English**, regardless of the
language of the conversation that produced it.

## Governance

This constitution supersedes convenience and habit, not the code: where a principle describes
behaviour the code does not have, the principle is the bug report.

Amendments are recorded in the table of the spec they affect and in this file's version line.
A principle may be removed — but only explicitly, never by quietly ceasing to follow it.

**Version**: 1.0.0 | **Ratified**: 2026-08-07 | **Last Amended**: 2026-08-07
