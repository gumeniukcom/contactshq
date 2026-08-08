# Feature Specification: Runtime, Configuration & Delivery

Kind: journey
Status: shipped
Constitution: v1.0.0

Reconstructed from the implementation at commit `23a167c` (`v0.4.0-3-g23a167c`). Every requirement
below was read out of the code at the cited path before it was written down. Where the
implementation has a deliberate limitation, a stale string or a genuine gap, it is recorded under
`## Known Divergences` rather than dressed up as a requirement that is met.

This spec owns the machinery every feature runs on — configuration, schema, the request pipeline,
background execution, observability, the CLI dispatch contract, the build and release chain, and
the SPA shell that belongs to no one feature. It is **not** a residual owner: a source path this
spec does not name in `## Code Paths` is a coverage failure, resolved by claiming it in the right
spec or listing it in `specs/UNCLAIMED.md` with a reason — never by widening this one.

## User Scenarios & Testing *(mandatory)*

The people served by this domain are the **operator** who installs, configures and upgrades an
instance, and the **contributor** who has to change it without breaking the deployments already
running. Every ordinary user is served indirectly: the error envelope they read, the limits they
hit, the screens they navigate and the theme they picked all live here.

### User Story 1 - Bring an instance up, and be told plainly when it must not start (Priority: P1)

An operator copies the example config or sets a handful of `CHQ_` environment variables, starts
the process, and either gets a running server or a single line saying exactly which setting is
wrong. A configuration that would be unsafe — no signing secret, a placeholder secret, a write
deadline that would truncate a restore, an import limit larger than the global body limit —
stops the process at boot rather than becoming an incident later.

**Why this priority**: This is the first thing every deployment does and the only place where a
mistake is still cheap. A weak `auth.jwt_secret` lets anyone forge an administrator token; the
project treats refusing to start as the correct response, and it is load-bearing enough that
`Validate` is never relaxed, not even for the CLI (`internal/config/config.go:213-228`).

**Independent Test**: Start the binary with no `CHQ_AUTH_JWT_SECRET` and observe the refusal;
set a 40-character random secret and observe it start; set `server.max_import_bytes` above
`server.max_body_bytes` and observe a second, different refusal.

**Acceptance Scenarios**:

1. **Given** no `auth.jwt_secret` anywhere, **When** the server starts, **Then** it exits with a
   message naming the setting and the command that generates one.
2. **Given** `auth.jwt_secret` set to `changeme` (any case), **When** the server starts, **Then**
   it refuses, saying the value is a well-known placeholder.
3. **Given** a secret shorter than 32 characters, **When** the server starts, **Then** it refuses
   and states both the required and the supplied length.
4. **Given** a valid config file **and** `CHQ_DATABASE_DSN` in the environment, **When** the
   server starts, **Then** the environment value wins.
5. **Given** `server.trusted_proxies` containing something that is neither an IP nor a CIDR,
   **When** the server starts, **Then** it refuses rather than quietly trusting nothing.
6. **Given** a non-zero `server.write_timeout`, **When** the server starts, **Then** it refuses,
   because restore and import mutate contacts synchronously inside the request.
7. **Given** no config file at all, **When** the server starts, **Then** defaults plus the
   environment are enough — a missing file is not an error.

---

### User Story 2 - Upgrade without losing the database (Priority: P1)

An operator pulls a new image and restarts. The new binary applies whatever schema changes it
carries, inside transactions, records what it applied, and reports the applied schema version on
its health endpoint. A half-applied migration never happens: either a file went in whole or it
did not go in at all.

**Why this priority**: The database is the only thing in the system that cannot be rebuilt. A
migration runner that can wedge a schema turns every upgrade into a gamble.

**Independent Test**: Start against an empty database, confirm the tables exist and
`schema_migrations` holds one row per file; restart and confirm nothing is re-applied; point a
migration at deliberately invalid SQL and confirm neither its statements nor its version row
survive.

**Acceptance Scenarios**:

1. **Given** an empty database, **When** the server starts, **Then** every embedded migration is
   applied in filename order and recorded, and the server begins serving.
2. **Given** an already-migrated database, **When** the server restarts, **Then** no migration is
   re-applied.
3. **Given** a migration whose second statement fails, **When** it is applied, **Then** the first
   statement is rolled back with it and no `schema_migrations` row is written.
4. **Given** a binary started from any working directory, **When** it migrates, **Then** it finds
   its migrations — they are compiled into it, not read from disk.
5. **Given** a build that somehow carries no migrations, **When** it starts, **Then** that is an
   error, not a silent "already up to date".
6. **Given** a running instance, **When** `GET /health` is called, **Then** it reports the
   filename stem of the newest applied migration.

---

### User Story 3 - A failure tells the user something useful and the operator something precise (Priority: P1)

A request fails. The user sees a short message in a predictable shape; they never see a database
error, a file path or a hostname. The operator finds one log line carrying the same request id
the user can quote, with the real cause attached.

**Why this priority**: The error envelope is a contract in two directions — the SPA parses it,
and the security posture depends on internal text never crossing it.

**Independent Test**: Make a handler return a raw driver error and confirm the body is exactly
`{"error":"internal server error"}` while the log line holds the driver text and the request id.

**Acceptance Scenarios**:

1. **Given** a handler returning an arbitrary error, **When** the response is written, **Then** it
   is `500` with `{"error":"internal server error"}` and nothing of the original text.
2. **Given** a handler returning a deliberate `404` with a chosen message, **When** the response is
   written, **Then** the status and the message survive and nothing is logged at error level.
3. **Given** a request carrying `X-Request-Id`, **When** it is handled, **Then** that id is echoed
   back, is available to handlers, and appears in the log line and in any error log.
4. **Given** a request carrying no `X-Request-Id`, **When** it is handled, **Then** one is minted
   and used the same way.
5. **Given** a successful health check, **When** it completes, **Then** nothing is logged; a
   failing one is always logged.
6. **Given** an unrouted path, **When** it is requested, **Then** it stays a `404` rather than
   collapsing into the generic `500`.

---

### User Story 4 - One client cannot spend the whole instance's capacity (Priority: P2)

Sign-in attempts are throttled because each one costs a 64 MiB password hash. The six operations
that read or rewrite an entire address book — duplicate detection, both imports, a pipeline
trigger, a backup and a restore — share a *single* budget of five per minute per client, so a
client cannot open five doors at once into the same expensive room. Bodies are capped, with a
larger allowance on the import routes and a clear `413` everywhere else.

**Why this priority**: These are the paths where one client can make the server allocate or
compute far more than the request cost them. It is P2 rather than P1 because the failure is
degradation, not data loss.

**Independent Test**: Call `POST /api/v1/backup/create` five times in a minute, then call
`POST /api/v1/import/vcard` and observe `429` — a different endpoint, the same budget. Post a
3 MiB JSON body to an ordinary endpoint and observe `413`; post the same to `/import/vcard` and
observe it accepted.

**Acceptance Scenarios**:

1. **Given** ten sign-in attempts in a minute from one address, **When** an eleventh arrives,
   **Then** it is refused with `429` and `{"error":"too many attempts, try again later"}`.
   *(The credential tiers are spec 001 FR-041/FR-042; this scenario is context, not a rule
   this spec states.)*
2. **Given** register and login, **When** either is called, **Then** they draw on the same bucket;
   token refresh draws on its own, more generous one. *(Spec 001 FR-041, FR-042.)*
3. **Given** any five of the six expensive routes in a minute, **When** a sixth call to any of them
   arrives, **Then** it is refused — the budget is shared across all six. *(FR-034, this spec.)*
4. **Given** no trusted proxies configured, **When** a request arrives carrying
   `X-Forwarded-For`, **Then** the header is ignored and the direct peer is the client.
   *(Spec 001 FR-044; this spec owns only the setting and its validation, FR-010.)*
5. **Given** trusted proxies configured, **When** requests arrive through one, **Then** each
   forwarded client gets its own bucket. *(Spec 001 FR-044.)*
6. **Given** a body larger than the route's limit, **When** it is posted, **Then** the response is
   `413` naming the limit in bytes. *(FR-030, this spec.)*

---

### User Story 5 - See whether the instance is alive, current and keeping up (Priority: P2)

One unauthenticated endpoint answers: is the process up, which version is it, when was it built,
can it reach its database, which schema is applied, and how many background jobs are waiting. The
container runtime polls the same endpoint and takes a container that cannot reach its database
out of service.

**Why this priority**: A health check that only proves the process is running is worse than none:
it reported everything fine while every request failed against an unreachable database.

**Independent Test**: Call `GET /health` and check the fields; stop the database and confirm the
response becomes `503` with `"database":"unreachable"`; queue work and confirm `queue_depth` rises.

**Acceptance Scenarios**:

1. **Given** a healthy instance, **When** `/health` is called, **Then** it returns `200` with
   status, version, build time, database `ok`, the applied schema version and the queue depth.
2. **Given** an unreachable database, **When** `/health` is called, **Then** it returns `503` with
   status `degraded`.
3. **Given** a stalled database, **When** `/health` is called, **Then** it answers within seconds
   rather than hanging with the rest of the process.
4. **Given** a deep job backlog, **When** `/health` is called, **Then** the depth is reported and
   the status stays `ok` — a restart is exactly what would lose those jobs.
5. **Given** a container, **When** it cannot reach its database for three consecutive polls,
   **Then** the runtime marks it unhealthy.

---

### User Story 6 - Scheduled work happens without anyone asking, and a hard kill leaves no lies behind (Priority: P2)

Pipelines, backups and duplicate detection run on their own cron schedules. Saving a schedule
takes effect immediately, with no restart. If the process is killed mid-run, the next boot closes
the history rows it left open instead of leaving them saying "still running" forever.

**Why this priority**: Unattended execution is what makes the product a service rather than a
tool. The reconciliation is what keeps its history honest.

**Independent Test**: Change a pipeline schedule and confirm the next firing moves without a
restart; `docker kill` the container mid-backup and confirm the run reads `interrupted` after the
next start rather than `running`.

**Acceptance Scenarios**:

1. **Given** enabled pipelines, per-user backup schedules and per-user dedup schedules, **When**
   the server starts, **Then** a cron job is registered for each.
2. **Given** a pipeline created, edited or deleted through the API, **When** the change is saved,
   **Then** its scheduled job is registered, replaced or removed in the running scheduler.
3. **Given** a schedule that is not a valid five-field cron expression, **When** it is saved,
   **Then** it is rejected.
4. **Given** runs left at `running` by a process that died, **When** a new process starts,
   **Then** they are closed as interrupted — but only those that began before this process did.
5. **Given** shutdown, **When** the signal arrives, **Then** HTTP drains, the scheduler stops, the
   job in flight is allowed to finish and the buffer is drained, all inside one bounded deadline.
6. **Given** a full job queue, **When** the scheduler tries to enqueue, **Then** it is told so
   immediately rather than blocking its own goroutine forever.

---

### User Story 7 - Operate the instance from a shell (Priority: P3)

The same binary is the admin tool. `contactshq set-password you@example.com` recovers a locked-out
account; `contactshq reencode-vcards` repairs stored cards; `contactshq version` and `help`
explain themselves. A subcommand never needs the signing secret, never takes a password as an
argument, and never starts a server by accident.

**Why this priority**: Rare but unforgiving. The one moment this is used is a moment when nobody
can sign in, and a typo that silently started a server instead would be actively harmful.

**Independent Test**: Run `set-password` with a typo'd verb and confirm exit code 2 and a usage
message rather than a running server; run it against an empty database and confirm exit code 5;
check `docker inspect` and the logs afterwards for the password.

**Acceptance Scenarios**:

1. **Given** no arguments, **When** the binary runs, **Then** it starts the server.
2. **Given** an unrecognised first argument, **When** the binary runs, **Then** it prints usage and
   exits `2` — never falls through to serving.
3. **Given** a leading dash, **When** the binary runs, **Then** it is treated as a server flag, not
   a subcommand.
4. **Given** `set-password <email> --stdin`, **When** it runs, **Then** the flag written *after*
   the positional is honoured.
5. **Given** a terminal, **When** `set-password` runs without `--stdin`, **Then** the password is
   prompted twice without echo and compared.
6. **Given** a pipe and no `--stdin`, **When** `set-password` runs, **Then** it refuses and says
   which flag to pass.
7. **Given** an unknown email, **When** `set-password` runs, **Then** it exits `3`.
8. **Given** a database with no schema, **When** any subcommand runs, **Then** it exits `5` and
   tells the operator to start the server once — it does not migrate.

---

### User Story 8 - Install and upgrade from a published artefact (Priority: P3)

An operator runs `docker compose up -d` against a published multi-architecture image, or downloads
a release archive. The image runs unprivileged, carries no local developer configuration, contains
its own migrations and its own web UI, and reports its version.

**Why this priority**: Delivery is what turns the repository into something somebody else can run.
It is P3 only because a failure here is discovered before anything is at stake.

**Independent Test**: Build the image from a checkout containing a real `configs/config.yaml`, and
confirm the file is not in the image while `config.example.yaml` is; run the container and confirm
it becomes healthy and that `id -u` is not 0. Both are CI steps.

**Acceptance Scenarios**:

1. **Given** compose with no `CHQ_AUTH_JWT_SECRET`, **When** `docker compose up` runs, **Then** it
   refuses with a message naming the variable and how to generate one.
2. **Given** a build context holding a developer's `configs/config.yaml`, **When** the image is
   built, **Then** the file is not in the image.
3. **Given** a running container, **When** its process is inspected, **Then** it is not root.
4. **Given** a tag `vX.Y.Z`, **When** the release workflow runs, **Then** linux amd64 and arm64
   binaries, archives and a multi-architecture `ghcr.io` image are published, and the release body
   is the hand-written changelog section.
5. **Given** a prerelease tag, **When** the release runs, **Then** `:latest` is not moved onto it.
6. **Given** any pull request, **When** CI runs, **Then** Go tests with the race detector, `go vet`,
   a pinned linter, the PostgreSQL migration suite, the frontend lint/format/test suite, a full
   build and an end-to-end container exercise all run.

---

### User Story 9 - Use the whole product through one coherent shell (Priority: P2)

Every screen sits in the same frame: a sidebar with live badges, a header with the account and a
theme toggle, a progress bar on navigation, toasts for outcomes, modals that trap focus, and one
confirm dialog that can demand a typed word before a destructive action. Signing out, or a session
that cannot be refreshed, returns to the login screen.

**Why this priority**: It is the surface through which almost every other requirement in the
product is actually reached. It is P2 rather than P1 because the API remains fully usable without
it.

**Independent Test**: Sign in, navigate every sidebar entry, toggle the theme and reload, let an
access token expire and confirm the session refreshes silently, then clear the refresh token and
confirm the next call lands on the login screen.

**Acceptance Scenarios**:

1. **Given** an unauthenticated visitor, **When** they open any application route, **Then** they
   are redirected to the login screen.
2. **Given** a non-administrator, **When** they open an admin route, **Then** they are redirected
   to the dashboard, and the admin section is not rendered in the sidebar.
3. **Given** an expired access token and a valid refresh token, **When** a request returns `401`,
   **Then** the client refreshes once, retries the request, and the user notices nothing.
4. **Given** several requests failing with `401` at once, **When** the refresh is in flight,
   **Then** they queue behind it rather than each starting their own.
5. **Given** a failed refresh or no refresh token at all, **When** a `401` arrives, **Then** stored
   tokens are cleared and the browser goes to the login screen.
6. **Given** an unknown URL under the application base, **When** it is opened, **Then** a
   not-found screen renders rather than an empty layout.
7. **Given** a theme choice, **When** the page is reloaded, **Then** it is remembered; `system`
   follows the operating system live.
8. **Given** an open modal, **When** the user presses Tab repeatedly, **Then** focus stays inside;
   Escape closes it and focus returns to where it was.

---

### Edge Cases

These are boundary conditions the code handles deliberately. Behaviour that is *wrong*, stale,
silently ignored or unenforced is recorded under Known Divergences instead, not here.

**Configuration**

- **What happens when three settings are wrong at once?** `Validate` returns the first failure
  only, so an operator fixes them one restart at a time
  (`internal/config/config.go:293-304`). The order is not arbitrary: authentication is checked
  first because a forgeable token is the misconfiguration with the worst consequence.

**Schema and persistence**

- **What happens when a schema change has to be undone?** Nothing in the application can do it.
  Migrations are forward-only: the `.down.sql` files are embedded (`migrations/embed.go:9-10`)
  and `MigrateFS` globs `*.up.sql` alone (`internal/repository/db.go:75`). Rolling back means
  restoring a database dump.
- **What happens when two writers hit SQLite at once?** They queue. `SetMaxOpenConns(1)`
  (`internal/repository/db.go:37`) is what makes the file safe under concurrent writes, and it
  is also why one long operation blocks every other request that touches the database. The
  PostgreSQL path sets no pool limit at all, so it inherits the driver default.
- **What happens when a migration is numbered without zero padding?** The reported schema
  version becomes wrong, not merely untidy. `schema_migrations.version` is a filename stem and
  the ordering guarantee is lexicographic; zero padding is what makes the lexicographic maximum
  also the numeric one (`internal/repository/db.go:121-134`).

**Request pipeline**

- **What happens when body limits are nested?** Both run, parent first, and the parent's `413`
  wins — which is why there is exactly one middleware resolving the limit per path
  (`internal/handler/middleware/bodylimit.go:37-42`, `internal/handler/handler.go:75-80`). A
  narrow limit on a parent group and a wider one on a child does not do what it looks like.
- **What happens when a client alternates between the six expensive endpoints?** It still gets
  five calls a minute in total, not five each (`internal/handler/handler.go:105`, `:134`,
  `:151-152`, `:225`, `:234`, `:242`). A user who has just imported a file may find "create
  backup" refused. This is deliberate — the point is to bound the total, not each door into it
  (`internal/handler/middleware/ratelimit.go:20-26`).
- **What happens to a private or loopback address?** Nothing filters it anywhere. LAN CardDAV is
  a supported setup; the rule is owned by 006.
- **What does an unauthenticated caller learn from `/health`?** The version, the build time and
  the applied schema version (`internal/handler/handler.go:258-292`). That is deliberate — it is
  what a container runtime and an operator need — and it does disclose the build to anyone who
  can reach the port.

**Background execution**

- **What happens when the queue is full?** `Enqueue` returns `ErrQueueFull` immediately and is
  never waited out, because the scheduler calls it from its own goroutine with a background
  context and blocking there would stall every later scheduled job
  (`internal/worker/goroutine_worker.go:70-97`). The scheduler logs the failure; the job is
  simply not run.
- **What happens between removing and re-adding a scheduled job?** There is a moment with no job
  registered. The window is inside a mutex and is not observable from outside the process
  (`internal/worker/scheduler.go:100-106`, `:152-159`, `:196-203`).
- **What happens when a second process starts against the same database?** The boot-time
  reconciliation closes only runs that began before *this* process started, so a second
  instance's live runs are never marked interrupted (`cmd/server/startup.go:16-49`,
  `cmd/server/startup_test.go:105-116`). That makes a second instance harmless rather than
  destructive; it does not make multi-instance a supported configuration.
- **What happens to a cron expression `humanizeCron` does not recognise?** It is displayed
  verbatim rather than wrongly; the helper covers roughly fifteen common shapes and falls back
  to the raw string (`web/src/utils/cron.ts:29-33`).

**The web shell**

- **What happens to a request that was queued behind a token refresh?** It is re-sent through the
  same axios instance, so a request whose body was a consumed stream would not survive. In
  practice every call sends a plain object or a `FormData` (`web/src/api/client.ts:46-50`).
- **What does a navigation cost?** Two extra API calls: the sidebar refetches both badge counts
  on every navigation (`web/src/components/layout/Sidebar.vue:100-108`). That is what keeps the
  badges from going stale while the layout stays mounted.

## Requirements *(mandatory)*

> **FR numbers are stable identifiers.** Three requirements were settled against sibling specs
> rather than deleted: FR-033 and FR-035 are withdrawn in favour of spec 001 and retained as
> labelled cross-references, and FR-068 is narrowed to the half this spec owns. Numbers are never
> reused, so an external citation cannot silently point at a different rule.

### Functional Requirements

**Configuration loading**

- **FR-001**: The system MUST read configuration from `./configs/config.yaml` or `./config.yaml`
  if present, and MUST treat a missing file as normal rather than an error
  (`internal/config/config.go:233-258`).
- **FR-002**: Every configuration key MUST be overridable by an environment variable named by
  upper-casing the key, replacing dots with underscores and prefixing `CHQ_`, and the environment
  MUST outrank the config file (`internal/config/config.go:240-242`; verified by loading a config
  file and an environment variable for the same key).
- **FR-003**: Every environment-overridable key MUST be bound explicitly by name, because viper's
  automatic environment lookup only reaches keys it already knows about — the defect that once made
  `CHQ_AUTH_JWT_SECRET` and every `CHQ_GOOGLE_*` variable dead on env-only deployments
  (`internal/config/config.go:32-59`, `:244-252`; 26 keys).
- **FR-004**: A key that has a default MUST also have a binding entry, and a test MUST fail the
  build otherwise, naming the variable that would be ignored
  (`internal/config/env_binding_test.go:121-136`). Every bound key MUST additionally be proven to
  arrive in the loaded struct (`internal/config/env_binding_test.go:94-116`).
- **FR-005**: A list-valued setting MUST accept both a YAML list and a single comma-separated
  string, with blanks dropped (`internal/config/config.go:265-284`).
- **FR-006**: `Load` MUST apply validation and the server MUST refuse to start when it fails,
  reporting the reason (`internal/config/config.go:200-211`, `cmd/server/main.go:60-63`).

**Fail-closed validation**

- **FR-007**: `auth.jwt_secret` MUST be present, MUST NOT be one of the five known placeholder
  values (compared case-insensitively), and MUST be at least 32 characters; each refusal MUST name
  the command that generates a good one (`internal/config/config.go:13-24`, `:362-381`,
  `internal/config/config_test.go:10-54`).
- **FR-008**: Validation MUST report the authentication failure first, because a forgeable token is
  the misconfiguration with the worst consequence (`internal/config/config.go:289-304`).
- **FR-009**: `database.driver` MUST be `sqlite` or `postgres` and `database.dsn` MUST be non-empty
  (`internal/config/config.go:309-320`).
- **FR-010**: Every `server.trusted_proxies` entry MUST parse as an IP address or a CIDR range;
  a typo MUST stop the server rather than leave the operator believing rate limiting keys on the
  real client (`internal/config/config.go:322-333`, `internal/config/config_test.go:55-93`).
- **FR-011**: `server.max_body_bytes` and `server.max_import_bytes` MUST both be positive, and the
  import limit MUST NOT exceed the global one — a per-route limit above the global one is a promise
  fasthttp rejects before the route's middleware runs (`internal/config/config.go:335-346`,
  `internal/config/config_test.go:124-140`).
- **FR-012**: `server.write_timeout` MUST be zero, and a non-zero value MUST stop the server with a
  message explaining that restore and import mutate contacts synchronously inside the request
  (`internal/config/config.go:348-351`, `internal/config/config_test.go:145-152`).
- **FR-013**: `carddav.max_resource_bytes` MUST be positive (`internal/config/config.go:355-360`).
- **FR-014**: A CLI subcommand MUST be able to load configuration without a signing secret, and
  that relaxation MUST be a separate entry point that validates the database section only — the
  full validation MUST NOT be weakened (`internal/config/config.go:213-228`,
  `internal/config/load_for_cli_test.go`).
- **FR-015**: Defaults MUST be registered in one place, separate from loading, so a test can assert
  that the set of known keys stays a subset of the bound keys
  (`internal/config/config.go:169-198`).

**Persistence and schema**

- **FR-016**: The system MUST support SQLite and PostgreSQL through one ORM layer; SQLite MUST be
  opened with foreign keys and write-ahead logging enabled and MUST be limited to a single
  connection; both drivers MUST be pinged before the process proceeds
  (`internal/repository/db.go:26-53`).
- **FR-017**: Migrations MUST be compiled into the binary, so a binary started from any working
  directory carries its own schema (`migrations/embed.go`,
  `internal/repository/migrate_runner_test.go:19-36`).
- **FR-018**: Migrations MUST be applied at startup before anything else touches the schema, and a
  failure MUST stop the process (`cmd/server/main.go:79-82`).
- **FR-019**: Each migration file MUST run inside its own transaction together with the row that
  records it, so a failure part-way through leaves nothing behind
  (`internal/repository/db.go:109-119`, `internal/repository/migrate_runner_test.go:49-81`).
- **FR-020**: Only `*.up.sql` files MUST be applied, in filename order, and an empty migration set
  MUST be an error rather than a silent "already up to date"
  (`internal/repository/db.go:75-82`, `internal/repository/migrate_runner_test.go:38-47`).
- **FR-021**: A migration already recorded MUST be skipped, so repeated starts apply nothing
  (`internal/repository/db.go:87-94`, `internal/repository/migrate_runner_test.go:108`).
- **FR-022**: The recorded version MUST be the migration's filename stem, and the accessor MUST
  return it as a string; names MUST be zero-padded so the lexicographic maximum is the newest
  (`internal/repository/db.go:85`, `:121-134`).
- **FR-023**: The full expected table set MUST be listed in one place and compared against the live
  PostgreSQL schema **in both directions**, so a new table added without a corresponding line fails
  the build and a table that exists but is unlisted does too
  (`internal/repository/migrate_postgres_test.go:49-110`; 25 tables).
- **FR-024**: Migrations MUST be exercised against PostgreSQL in CI, since every migration is
  written in SQLite-flavoured SQL and the documented install runs PostgreSQL
  (`.github/workflows/ci.yml:40-74`).
- **FR-025**: A new migration MUST restrict itself to what a forward-only, transactional runner can
  carry: creating tables and indexes, and adding columns with a default. Dropping or renaming a
  column, adding a `NOT NULL` column without a default, creating an index concurrently (rejected
  inside a transaction by PostgreSQL) and bulk data rewrites are all excluded by that runner's
  shape (`internal/repository/db.go:65-119`; migration `002_contacts_title_note.up.sql` is the
  worked example of the allowed form).

**Request pipeline**

- **FR-026**: Every error response MUST have the body shape `{"error": "..."}`. An error this
  application chose MUST keep its status and message; any other error MUST be logged with its cause
  and answered with a fixed `"internal server error"` (`cmd/server/main.go:362-395`,
  `cmd/server/main_test.go:17-80`).
- **FR-027**: Every request MUST carry an id — reused from `X-Request-Id` when a proxy supplied one,
  minted otherwise — echoed in the response header, available to handlers, and present in both the
  access log line and any error log line for that request
  (`internal/handler/middleware/logger.go:30-66`, `cmd/server/main.go:380-389`).
- **FR-028**: A successful health check MUST NOT be logged; a failing one MUST be
  (`internal/handler/middleware/logger.go:44-46`,
  `internal/handler/middleware/logger_test.go:84-107`).
- **FR-029**: The global request body ceiling MUST come from `server.max_body_bytes` and MUST be
  understood as the only real memory bound (`cmd/server/main.go:205-211`).
- **FR-030**: Per-route body limits MUST be resolved by a single middleware from the request path,
  never by nesting one limit inside another, and an oversized body MUST be answered `413` with a
  message naming the limit (`internal/handler/middleware/bodylimit.go:43-81`,
  `internal/handler/handler.go:75-80`, `internal/handler/middleware/bodylimit_test.go`).
- **FR-031**: Ordinary API routes MUST be capped well below the global ceiling (2 MiB, or the
  global ceiling when that is smaller), and the import routes MUST receive
  `server.max_import_bytes` (`cmd/server/main.go:272-275`, `:397-407`).
- **FR-032**: Authentication and expensive operations MUST be rate limited per client address over
  a one-minute window, answering `429` with `{"error":"too many attempts, try again later"}`
  (`internal/handler/middleware/ratelimit.go:29-50`).
- **FR-033**: *Withdrawn — cross-reference only.* The credential-cost tiers (register and login
  sharing one bucket of 10 per minute, token refresh its own bucket of 60) are **spec 001 FR-041
  and FR-042**. Both constants and the `RateLimiter` constructor live in
  `internal/handler/middleware/ratelimit.go:11-37`, a path this spec lists under References as
  owned by 001; ownership of the file and of the rule must not diverge. The shared
  expensive-operation budget stays here as FR-034, because the sharing is achieved by passing one
  middleware instance in `internal/handler/handler.go`, which this spec owns.
- **FR-034**: The six operations that read or rewrite a whole address book — duplicate detection,
  vCard import, CSV import, pipeline trigger, backup creation and restore — MUST share **one**
  budget of 5 per minute. Sharing is achieved by passing one middleware instance to all six routes
  (`internal/handler/handler.go:102-105`, `:134`, `:151-152`, `:225`, `:234`, `:242`,
  `internal/handler/middleware/ratelimit_test.go:42-65`).
- **FR-035**: *Withdrawn — cross-reference only.* "`X-Forwarded-For` is believed only when the
  request arrived through a configured trusted proxy, otherwise the direct peer is the rate-limit
  key" is **spec 001 FR-044**. Both enforcement sites are 001's —
  `internal/handler/middleware/ratelimit.go:41` for the API limiter and
  `internal/carddav/throttle.go:166-196` for the DAV failure bucket (001 FR-038) — and it is 001's
  per-client budget claims that become false if the keying rule changes. This spec keeps the
  `server.trusted_proxies` configuration surface and its validation (FR-010) and the wiring that
  hands the same list to the CardDAV adapter (FR-036).
- **FR-036**: The same trusted-proxy list MUST be passed to the CardDAV server, which is mounted
  through an adapter and sees a `net/http` request that the framework's own proxy handling never
  reaches (`cmd/server/main.go:286-291`).
- **FR-037**: A read timeout and an idle timeout MUST be applied so a slow or idle connection cannot
  hold a worker indefinitely, and no write timeout MUST be applied
  (`cmd/server/main.go:212-221`).
- **FR-038**: The WebDAV verbs (`PROPFIND`, `PROPPATCH`, `REPORT`, `MKCOL`, `COPY`, `MOVE`) MUST be
  added to the framework's routable method set, or the `/dav` mount is unreachable to every client
  (`cmd/server/main.go:41-50`, `:223`).
- **FR-039**: Panic recovery, CORS and the request logger MUST run ahead of every route
  (`cmd/server/main.go:238-240`).
- **FR-040**: Shutdown on `SIGINT`/`SIGTERM` MUST drain HTTP, stop the scheduler and stop the
  worker, all bounded by one 30-second deadline (`cmd/server/main.go:303-339`, `:38-39`).
- **FR-041**: At boot the system MUST close backup and sync history rows left `running` by a dead
  process, **and MUST close only those that began before this process started**, so a second
  instance's live runs are never marked interrupted (`cmd/server/main.go:84-86`, `:191-192`,
  `cmd/server/startup.go:16-49`, `cmd/server/startup_test.go:88-136`).
- **FR-042**: At boot the system MUST prune pipeline-run history past its retention, because that
  table gains a row per execution rather than roughly one a day
  (`cmd/server/startup.go:51-66`, `cmd/server/main.go:193`).

**Background execution**

- **FR-043**: Background work MUST run on an in-process pool of goroutines fed by a bounded queue
  (4 workers, 100 slots) (`internal/worker/goroutine_worker.go:39-50`, `cmd/server/main.go:136`).
- **FR-044**: Enqueueing MUST never block: a full queue MUST return an error immediately, and an
  enqueue after shutdown began MUST be refused rather than accepted and dropped
  (`internal/worker/goroutine_worker.go:70-97`, `internal/worker/worker.go:8-17`,
  `internal/worker/goroutine_worker_semantics_test.go:80-164`).
- **FR-045**: A panicking job MUST be contained and logged with its stack; the worker MUST keep
  running. Job handlers parse data from outside the system and the HTTP recover middleware does not
  cover them (`internal/worker/goroutine_worker.go:131-155`,
  `internal/worker/goroutine_worker_test.go:33-62`).
- **FR-046**: Stopping MUST stop accepting work, let the job in flight finish, drain what is
  buffered, and interrupt handlers only once the caller's deadline has run out
  (`internal/worker/goroutine_worker.go:157-203`,
  `internal/worker/goroutine_worker_semantics_test.go:19-79`, `:165-195`).
- **FR-047**: The queue MUST carry the four job types the product schedules — pipeline, backup,
  sync and dedup — registered at composition (`cmd/server/main.go:136-141`). The job handlers MUST
  depend on interfaces declared in the consuming package rather than on concrete services, so a
  handler's failure paths are testable without a database or a filesystem
  (`internal/worker/jobs/deps.go`).
- **FR-048**: At startup the system MUST register a scheduled job for every enabled pipeline, every
  user with a backup schedule and every user with an enabled dedup schedule, and MUST survive any
  of those lookups failing (`cmd/server/main.go:149-189`).
- **FR-049**: Creating, updating or deleting a schedule MUST take effect in the running scheduler
  immediately; jobs MUST be addressable by tag so they can be replaced or removed without a restart
  (`internal/worker/scheduler.go:100-106`, `:135-159`, `:188-203`).
- **FR-050**: A schedule MUST be validated as a five-field cron expression before it is accepted
  (`internal/worker/scheduler.go:32-40`).
- **FR-051**: "When does this run next" MUST be answered from the registered job rather than by
  re-parsing the expression somewhere else (`internal/worker/scheduler.go:215-235`).

**Observability**

- **FR-052**: `GET /health` MUST report status, version and build time; MUST report the queue depth
  when a worker is wired; MUST probe database connectivity under a 2-second bound and answer `503`
  with status `degraded` when it fails; and MUST report the applied schema version
  (`internal/handler/handler.go:17-19`, `:254-292`).
- **FR-053**: A deep queue MUST NOT make the health check fail — answering `503` would make the
  container runtime restart the process, and a restart is exactly what loses the queued jobs
  (`internal/handler/handler.go:265-271`).
- **FR-054**: Logging MUST be structured, with level and format taken from configuration, and MUST
  fall back to `info` rather than failing on an unparsable level (`internal/logger/logger.go`).
- **FR-055**: On SQLite the resolved absolute database path MUST be logged at startup, with a note
  when the configured path was relative, so an operator can see which file is actually in use
  (`cmd/server/main.go:342-360`).

**CLI dispatch contract**

- **FR-056**: Subcommands MUST be dispatched before the server's configuration is read, so a
  recovery command works on a deployment whose signing secret the operator does not have
  (`cmd/server/main.go:52-58`).
- **FR-057**: The first argument MUST be matched against a whitelist; an unrecognised one MUST print
  usage and exit `2` and MUST NEVER fall through to starting the server
  (`cmd/server/cli.go:31-62`, `cmd/server/cli_test.go:32-39`).
- **FR-058**: An argument beginning with a dash MUST be treated as a server flag, not a subcommand
  (`cmd/server/cli.go:41-45`, `cmd/server/cli_test.go:26-31`).
- **FR-059**: Exit codes MUST be stable and distinct, declared once as package constants:
  `0` success, `1` generic failure, `2` usage, `3` no such user, `4` database unreachable,
  `5` database not migrated (`cmd/server/cli.go:21-29`).
- **FR-060**: A secret MUST NEVER be accepted as a command-line argument — argv is visible in the
  process list, `/proc/<pid>/cmdline`, shell history and container inspection. A password MUST be
  prompted twice without echo and the two entries compared, or read from standard input behind an
  explicit `--stdin` (`cmd/server/cli.go:140-151`, `:240-284`, `cmd/server/cli_test.go:63-90`,
  `.github/workflows/ci.yml:215-219`).
- **FR-061**: Flag parsing MUST accept flags written after positional arguments — `parseInterleaved`
  rather than `fs.Parse` — because the standard parser stops at the first positional and would
  otherwise ignore `--stdin` in the form the documentation itself uses
  (`cmd/server/cli.go:64-82`, `cmd/server/cli_test.go:80-102`).
- **FR-062**: A subcommand MUST NOT run migrations, and MUST refuse to work on a database with no
  schema, exiting `5` and telling the operator to start the server once. The refusal MUST come
  before any prompt for a secret, so a missing schema does not cost the operator two password
  entries. The shared helper also returns the configuration it loaded, so a subcommand reporting on
  runtime behaviour quotes what this process read rather than a literal that can drift
  (`cmd/server/cli.go:109-138`, `:167-177`, `cmd/server/cli_test.go:207`).
- **FR-063**: `version` and `help` MUST work without a database and MUST report the version injected
  at build time (`cmd/server/cli.go:84-106`, `cmd/server/cli_test.go:40-53`).

**Build and delivery**

- **FR-064**: The build MUST produce a single static binary with the frontend already inside it:
  the frontend is built first, then the Go binary with cgo disabled
  (`Makefile:8-15`, `Dockerfile:1-24`).
- **FR-065**: Version and build time MUST be injected at link time and surfaced by both `/health`
  and the `version` subcommand (`Makefile:4-6`, `cmd/server/main.go:32-36`,
  `.goreleaser.yaml:23-24`).
- **FR-066**: SQLite MUST work without cgo, through a pure-Go driver, so one static binary runs on
  a minimal base image (`go.mod` → `modernc.org/sqlite`, `internal/repository/db.go:19`,
  `Dockerfile:29-31`).
- **FR-067**: The compiled SPA MUST be embedded and served under `/app`, with unknown sub-paths
  falling back to the application shell so client-side routing works on a page reload. The embed
  directory MUST contain a committed placeholder, or the package does not compile
  (`internal/web/embed.go`, `internal/web/handler.go:35-45`,
  `internal/web/static/spa/.gitkeep`, `.gitignore:42-43`).
- **FR-068**: A landing page MUST be served at `/`, from a template embedded in the binary and
  parsed at startup; a template that fails to parse MUST stop the process rather than serve a
  broken page (`internal/web/handler.go:12-33`, `internal/web/templates/landing.html`). The
  public CardDAV setup guide at `/setup` shares that mechanism but is **spec 004 FR-033**: 004
  owns `internal/web/templates/setup-guide.html` and the guide's content, and lists
  `internal/web/handler.go` under its References for the route registration.
- **FR-069**: Web routes MUST be registered after the API and the CardDAV mount, so a catch-all
  never shadows them (`cmd/server/main.go:242-301`).
- **FR-070**: The container image MUST run as an unprivileged user, MUST contain only the
  configuration *template* and never a real config file, MUST need no database client libraries,
  and MUST declare a health check that exercises the application's own health endpoint
  (`Dockerfile:26-54`, `Dockerfile.goreleaser`, `.dockerignore`).
- **FR-071**: Compose MUST refuse to start without `CHQ_AUTH_JWT_SECRET`, MUST run the documented
  PostgreSQL setup, MUST wait for the database to be healthy, and MUST keep backups on a named
  volume (`docker-compose.yml`, `.env.example`).
- **FR-072**: A tagged release MUST publish linux `amd64` and `arm64` binaries, archives carrying
  the config template, migrations, compose file, README and licence, and a multi-architecture
  container image; a prerelease MUST NOT move the `latest` tag; the release body MUST be the
  hand-written changelog section rather than a commit list
  (`.goreleaser.yaml`, `.github/workflows/release.yml`).
- **FR-073**: CI MUST run, on every pull request: Go tests with the race detector, `go vet`, a
  pinned linter version, the PostgreSQL migration and repository suite, frontend lint, format check
  and unit tests, and a full build (`.github/workflows/ci.yml:9-129`).
- **FR-074**: CI MUST additionally build the container image and exercise it end to end: prove a
  planted local config file does not reach the image, wait for the container to report healthy,
  register a user (which proves migrations ran inside the image), assert the process is not root,
  change a password through the CLI and prove the old one stops working, prove the password reaches
  neither the process inspection output nor the logs, and prove the vCard encoder and the
  re-encode command behave over a real round trip
  (`.github/workflows/ci.yml:131-301`).

**Web application shell**

- **FR-075**: The SPA MUST be served under the base path `/app/`, MUST redirect an unauthenticated
  visitor to the login screen, MUST redirect a non-administrator away from admin routes, MUST load
  the current user once per session, and MUST render a not-found screen for an unknown URL
  (`web/src/router/index.ts:4-5`, `:150-173`, `web/src/views/NotFoundView.vue`).
- **FR-076**: The API client MUST attach the stored access token to every request, and on a `401`
  MUST attempt exactly one token refresh, queue concurrent failures behind it, retry the original
  request on success, and clear the session and return to the login screen on failure or when there
  is no refresh token (`web/src/api/client.ts:9-82`). *(The session mechanism itself is spec 001
  FR-051/FR-052 and `web/src/api/client.ts` is 001's path; it is restated here because the shell's
  behaviour depends on it — see Known Divergences.)*
- **FR-077**: The client MUST extract a human-readable message from the `{"error": "..."}` envelope
  and fall back to the transport error — the SPA's dependence on that shape is what makes FR-026 a
  contract rather than a convention (`web/src/api/client.ts:84-93`).
- **FR-078**: Every authenticated screen MUST render inside one layout — sidebar, header, scrollable
  main region — with a spinner while a lazily loaded screen resolves; the sidebar MUST collapse
  behind an overlay on small screens and MUST show live counts of pending duplicates and conflicts,
  refreshed on navigation (`web/src/components/layout/AppLayout.vue`, `Sidebar.vue`, `Header.vue`,
  `NavLink.vue`).
- **FR-079**: The interface MUST be built on semantic colour tokens with a full dark theme, MUST
  offer light, dark and system modes, MUST persist the choice, and MUST follow the operating system
  live while in system mode (`web/src/style.css`, `web/src/composables/useTheme.ts`).
- **FR-080**: Transient outcomes MUST be reported through one shared toast queue, rendered once and
  auto-dismissed after four seconds, with a manual dismiss and a polite live region
  (`web/src/composables/useToast.ts`, `web/src/components/ui/AppToast.vue`).
- **FR-081**: A modal MUST be marked as a dialog, MUST keep keyboard focus inside itself, MUST close
  on Escape, and MUST return focus where it was (`web/src/components/ui/AppModal.vue`,
  `web/src/composables/useFocusTrap.ts`).
- **FR-082**: Navigation MUST show a progress indicator, delayed enough not to flash on a fast
  transition (`web/src/components/ui/RouteProgress.vue`).
- **FR-083**: Schedules MUST be edited through one shared control offering presets plus a custom
  cron expression, with a plain-language rendering of the current value, used by pipelines, backups
  and duplicate detection alike (`web/src/components/ui/ScheduleInput.vue`,
  `web/src/utils/cron.ts`).
- **FR-084**: One shared confirmation dialog MUST support requiring the user to type a specific word
  before the destructive action becomes available (`web/src/components/ui/ConfirmDialog.vue`).
- **FR-085**: Dates MUST be rendered in one place and one format (`YYYY.MM.DD`, optionally with
  `HH:MM`) so screens do not drift apart (`web/src/utils/date.ts`). Sizes, durations and relative
  times MUST likewise have one implementation each (`web/src/utils/format.ts`).
- **FR-086**: A test MUST fail the build when a screen exists under the views directory with no
  route pointing at it — a whole feature was once built, shipped and unreachable
  (`web/src/router/routes.spec.ts`).

### Key Entities

- **Config** — the whole settings surface, in nine sections (server, database, auth, google,
  carddav, backup, merge, sync, log). Loaded once at startup and never reloaded; changing anything
  means restarting (`internal/config/config.go:61-167`).
- **Bound-key list** — the enumeration of which settings the environment can reach. Its existence is
  the requirement: a key missing from it is a variable that is silently ignored
  (`internal/config/config.go:32-59`).
- **Migration** — one `NNN_name.up.sql` file, embedded, applied once, inside a transaction. Its
  identity is its filename stem. A paired `.down.sql` exists and is never applied
  (`migrations/`, `internal/repository/db.go:65-119`).
- **`schema_migrations` row** — version (filename stem) plus applied timestamp. The only record of
  what a database has had done to it (`internal/repository/db.go:66-71`).
- **Expected-table list** — the schema's declared shape, compared against the live PostgreSQL
  schema in both directions (`internal/repository/migrate_postgres_test.go:57-83`).
- **Repository interface set** — the persistence contracts every service depends on, declared in
  one place so no service imports a concrete Bun type (`internal/repository/interfaces.go`).
- **Request id** — a per-request string, taken from `X-Request-Id` or minted, echoed to the client
  and carried into every log line for that request. The only thing tying a user's report to a log
  entry (`internal/handler/middleware/logger.go:11-39`).
- **Error envelope** — `{"error": "..."}`. The single response shape for every failure, produced by
  the framework error handler and consumed by the SPA (`cmd/server/main.go:362-395`,
  `web/src/api/client.ts:84-93`).
- **Body-limit policy** — a default plus path-prefix overrides, resolved per request. Policy, not a
  memory bound (`internal/handler/middleware/bodylimit.go:10-17`).
- **Rate-limit bucket** — a counter owned by one middleware instance. Passing the same instance to
  several routes is what makes them share a budget
  (`internal/handler/middleware/ratelimit.go:29-50`).
- **Job** — a type string plus a JSON payload, held in a bounded in-memory channel. It has no
  database row and no identity that survives the process
  (`internal/worker/goroutine_worker.go:34-37`).
- **Scheduled job** — a cron expression plus a tag of the form `pipeline:<id>`, `backup:<userID>`
  or `dedup:<userID>`. The tag is what makes immediate re-registration possible
  (`internal/worker/scheduler.go:75-203`).
- **Health report** — status, version, build time, queue depth, database reachability, applied
  schema version (`internal/handler/handler.go:258-292`).
- **Subcommand** — a name in a whitelist mapped to a function taking argv, stdin and two writers,
  returning an exit code (`cmd/server/cli.go:31-39`).
- **Release artefact** — a linux binary per architecture, an archive, and a multi-architecture
  container image tagged by version and conditionally by `latest` (`.goreleaser.yaml`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An instance never starts with a forgeable signing secret. Three distinct refusals
  cover absent, placeholder and too-short, and the minimum is **32 characters**.
- **SC-002**: **100%** of environment-overridable settings are proven to arrive from their variable,
  and **zero** settings can exist with a default but no binding — both directions are enforced,
  which is what closed the class of bug where `CHQ_AUTH_JWT_SECRET` was silently ignored.
- **SC-003**: A failed migration leaves **zero** partially applied statements and **zero**
  version rows: the file and its record share one transaction.
- **SC-004**: A restart of an up-to-date instance applies **zero** migrations.
- **SC-005**: The live schema and the declared table list agree exactly — **25 tables**, compared
  in both directions, so neither an unlisted table nor a missing one can ship.
- **SC-006**: **Zero** internal error text reaches a client. Every non-deliberate failure is
  answered with one fixed string while the cause and the request id go to the log.
- **SC-007**: A user can quote **one** identifier that locates the exact log line for their failed
  request, whether the id came from their proxy or was minted by the server.
- **SC-008**: An idle instance logs **nothing** for its health checks, which arrive every
  **30 seconds** — so a real log line is not buried among thousands.
- **SC-009**: The whole-address-book operations are bounded at **5 per minute per client in
  total**, not 5 each — a client cannot spend five times the intended budget by alternating
  endpoints.
- **SC-010**: Worst-case request-body memory is a single number an operator can compute:
  concurrent uploads × `server.max_body_bytes` (default **32 MiB**), with ordinary API routes
  capped at **2 MiB** so only the import routes can approach it.
- **SC-011**: A spoofed forwarded header creates **zero** additional rate-limit buckets when no
  proxy is trusted, and behind a trusted proxy each forwarded client gets its own. *(The keying
  rule this measures is spec 001 FR-044; the outcome is listed here because this spec owns the
  setting that switches it on.)*
- **SC-012**: "Is this instance healthy" is answerable in **one unauthenticated request**, and the
  answer distinguishes a live process from a live process that cannot reach its database — the
  distinction that used to be invisible.
- **SC-013**: **No** history row remains at `running` after the process that opened it has exited,
  and **zero** rows belonging to another process are ever closed by that reconciliation.
- **SC-014**: A panicking background job costs **one job**, not the process.
- **SC-015**: A schedule change takes effect in **zero restarts**.
- **SC-016**: A mistyped subcommand results in **zero** servers started.
- **SC-017**: A password set through the CLI appears in **zero** of: argv, the process inspection
  output, the logs.
- **SC-018**: A locked-out operator can regain access with **one command** and without knowing the
  instance's signing secret.
- **SC-019**: The published image contains **zero** developer configuration files and runs as a
  **non-zero** uid, both asserted in CI on every pull request.
- **SC-020**: A deployment needs **one artefact** — the container image or the single binary — with
  no separate migration step, no database client libraries and no separate web asset bundle.
- **SC-021**: **Zero** screens exist in the web application with no route pointing at them.
- **SC-022**: An expired access token costs the user **zero** interruptions: the session is
  refreshed once, concurrent requests queue behind that one refresh, and the original request is
  retried.

## Out of Scope

- **Any feature's own endpoints, entities or views.** Contacts, authentication, vCard handling,
  CardDAV, import/export/backup, sync and duplicates are specs 001–007. They appear here only to be
  disclaimed, or where a rule of this spec is visible through them.
- **The authentication middleware, the JWT lifecycle and the CardDAV credential path.** Spec 001,
  which also owns the credential-cost rate-limit tiers and the trusted-proxy keying rule (this
  spec's withdrawn FR-033 and FR-035).
- **What a sync run or a backup run *means*.** This spec closes rows left open by a dead process
  and prunes old ones; it does not define either lifecycle.
- **Endpoint validation for outbound provider requests.** Spec 006, even though the setting that
  relaxes it lives in this spec's configuration surface.
- **The vCard encoder and its documented escaping gap.** Spec 003.
- **The public CardDAV setup guide's content and its `/setup` route as a user-facing promise.**
  Spec 004 FR-033.
- **The Vue stores, the per-feature API modules and every screen under `web/src/views/`** except
  `NotFoundView.vue`. Owned by the feature specs; this spec owns only the shell they render inside.

## Assumptions

These are conditions the implementation takes as given.

- **Single instance.** The job queue, the scheduler, the rate-limit counters and the CardDAV
  authentication cache are all per process, and the backup directory is a local path. Two
  processes against one database would each run every schedule and each hand out a full rate-limit
  budget. The one place where a second instance could be *destructive* — the boot-time
  reconciliation — is deliberately bounded so it is merely useless instead. Multi-instance is not a
  supported configuration and the stated condition for revisiting is a second replica.
- **Configuration is read once.** Changing anything, including a log level, requires a restart.
  There is no reload signal and no runtime settings endpoint for instance-level configuration.
- **Migrations are forward-only.** The `.down.sql` files are embedded for reference and nothing
  applies them. The recovery path for a bad schema change is a database dump.
- **SQLite is the single-user default and PostgreSQL is the documented install.** SQLite runs on one
  connection, so a long operation serialises everything else against that database; the compose file
  and the release archive point at PostgreSQL.
- **Restore and import run synchronously inside the HTTP request.** That is the whole reason
  `server.write_timeout` must stay zero. It can be reconsidered only once every long operation runs
  on the queue — which requires the queue to be durable first.
- **Backwards, the error envelope is a contract with the SPA.** Changing the body shape of a failure
  breaks `getApiError` and every screen that reports a message. The Go test that asserts the exact
  JSON exists to say so out loud.
- **The trusted-proxy list is trusted absolutely.** Anything reaching the process from a listed
  address may state who the client is. Nothing validates that the deployment actually has such a
  proxy in front of it.
- **The server-rendered HTML pages assume outbound internet access for styling.** The application
  itself does not; the SPA and its styles are embedded. Both server-rendered templates load a
  third-party stylesheet — see Known Divergences.
- **The `.gitkeep` under the SPA embed directory must stay committed.** Without it the package does
  not compile in a clean checkout.

## Status

Shipped. Every requirement above is serving at `23a167c` (`v0.4.0-3-g23a167c`). Nothing in this
spec is aspirational: where the shipped behaviour is silent, stale, untested or contradicts the
project's own rules, it is recorded under Known Divergences rather than softened here. Two
requirement numbers (FR-033, FR-035) are withdrawn cross-references to spec 001 and one (FR-068)
is narrowed against spec 004; the behaviour they described is still shipped, it is simply
specified elsewhere.

## Code Paths

Owned by this spec:

- `cmd/server/main.go`
- `cmd/server/main_test.go`
- `cmd/server/startup.go`
- `cmd/server/startup_test.go`
- `cmd/server/cli.go`
- `cmd/server/cli_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/config/env_binding_test.go`
- `internal/config/load_for_cli_test.go`
- `internal/logger/logger.go`
- `internal/handler/handler.go`
- `internal/handler/health_test.go`
- `internal/handler/middleware/logger.go`
- `internal/handler/middleware/logger_test.go`
- `internal/handler/middleware/bodylimit.go`
- `internal/handler/middleware/bodylimit_test.go`
- `internal/repository/db.go`
- `internal/repository/interfaces.go`
- `internal/repository/migrate_test.go`
- `internal/repository/migrate_runner_test.go`
- `internal/repository/migrate_postgres_test.go`
- `internal/worker/worker.go`
- `internal/worker/goroutine_worker.go`
- `internal/worker/goroutine_worker_test.go`
- `internal/worker/goroutine_worker_semantics_test.go`
- `internal/worker/scheduler.go`
- `internal/worker/scheduler_test.go`
- `internal/worker/jobs/deps.go`
- `internal/web/embed.go`
- `internal/web/handler.go`
- `internal/web/templates/landing.html`
- `internal/web/static/spa/.gitkeep`
- `migrations/embed.go`
- `migrations/001_init.up.sql`, `migrations/001_init.down.sql`
- `migrations/001_init.down.sql`
- `migrations/002_contacts_title_note.up.sql`, `migrations/002_contacts_title_note.down.sql`
- `migrations/002_contacts_title_note.down.sql`
- `migrations/020_drop_dead_jobs_table.up.sql`, `migrations/020_drop_dead_jobs_table.down.sql`
- `migrations/020_drop_dead_jobs_table.down.sql`
- `Makefile`
- `Dockerfile`
- `Dockerfile.goreleaser`
- `docker-compose.yml`
- `.goreleaser.yaml`
- `.github/workflows/ci.yml`
- `web/src/App.vue`
- `web/src/main.ts`
- `web/src/env.d.ts`
- `web/src/style.css`
- `web/src/types/index.ts`
- `web/src/test/setup.ts`
- `web/src/router/`
- `web/src/composables/`
- `web/src/components/layout/`
- `web/src/components/ui/`
- `web/src/utils/cron.ts`
- `web/src/utils/cron.spec.ts`
- `web/src/utils/date.ts`
- `web/src/utils/format.ts`
- `web/src/utils/format.spec.ts`
- `web/src/views/NotFoundView.vue`

`web/src/router/`, `web/src/composables/`, `web/src/components/layout/` and
`web/src/components/ui/` are subpackages of `web/src`, not the dense tree itself — every file
inside each is shell machinery with no feature of its own. Migrations `003`–`019` and `021`–`025`
belong to the feature specs that introduced them; `011` and `019` are cited under Known
Divergences only.

## References

Paths this spec touches but does **not** own:

- `internal/handler/middleware/ratelimit.go` — the limiter, its constants and its client-address
  keying. Spec 001 (FR-041, FR-042, FR-044). This spec passes one instance of it to six routes
  (FR-034) but states no rule about the file's contents.
- `internal/handler/middleware/ratelimit_test.go` — same owner; cited below for the one assertion
  that proves FR-034.
- `web/src/api/client.ts` — the SPA token store and the `401` refresh flow. Spec 001
  (FR-051, FR-052). Restated here as FR-076 because the shell depends on it; see Known Divergences.
- `web/src/api/client.spec.ts` — same owner.
- `internal/carddav/server.go`, `internal/carddav/backend.go` — spec 004. This spec owns only the
  `/dav` mount, the WebDAV verb registration and the trusted-proxy list handed to the adapter
  (FR-036, FR-038).
- `internal/web/templates/setup-guide.html` — spec 004 (FR-033). This spec owns the template-parsing
  mechanism the page shares with the landing page, not the page.
- `cmd/server/reencode.go`, `cmd/server/reencode_test.go` — the command's *dispatch contract* is
  specified here (FR-056..FR-063); what it does to stored cards is spec 003.
- `internal/sync/endpoint.go` — provider endpoint validation, spec 006. Cited because the setting
  that relaxes it, `sync.allow_insecure_endpoints` (`internal/config/config.go:159-161`), is part
  of this spec's configuration surface, as is `sync.runs_retention_days` behind FR-042.
- `internal/service/backup.go` — spec 005. Cited because the boot-time reconciliation (FR-041)
  closes rows this service opens.
- `internal/repository/bun_sync_run.go` — spec 006. Cited for the same reason, and for the
  pipeline-run pruning of FR-042.

Cross-domain couplings:

- **The web framework's routable method set** must include the WebDAV verbs, or spec 004's mount is
  unreachable (FR-038). This is the clearest coupling from this domain into a feature domain.
- **The error envelope (FR-026) and the API client (FR-077)** are two halves of one contract across
  the Go/TypeScript boundary.
- **The trusted-proxy configuration (FR-010, FR-036)** is consumed by spec 001's authentication
  rate limiting (FR-044) and by its CardDAV throttle (FR-038). This spec owns the setting, its
  validation and the wiring; 001 owns what is done with the resulting client address.
- **The shared expensive-operation budget (FR-034)** is consumed by specs 005 (import, backup,
  restore), 006 (pipeline trigger) and 007 (duplicate detection). The routes are theirs; the single
  bucket is specified here.
- **The boot-time reconciliation (FR-041)** closes rows owned by specs 005 (`backup_runs`) and 006
  (`sync_runs`). The mechanism and its process-start-time boundary are specified here; the meaning
  of the statuses is theirs.
- **`internal/config`** is imported by `internal/repository` and `internal/logger`, so a
  configuration change ripples into both.
- **External components**: Fiber and fasthttp (whose whole-body-before-handler behaviour is the
  reason FR-029 and FR-030 read as they do), Bun, `modernc.org/sqlite`, `pgdriver`, viper, zap,
  gocron, `robfig/cron` and `adhocore/gronx` (two cron parsers, for validation and for period
  estimation respectively), Vue 3, Vue Router, Pinia, axios, Tailwind, GoReleaser and GitHub
  Actions.

## Enforced By

**Configuration** (`internal/config/`)

- `TestAuthConfigValidate`, `TestConfigValidateRejectsDefaultSecret` (`config_test.go:10`, `:46`) —
  FR-007, FR-008, SC-001.
- `TestServerConfigValidate_TrustedProxies` (`config_test.go:55`) — FR-010.
- `TestServerConfigValidate_Limits` (`config_test.go:124`) — FR-011 and FR-012; the non-zero
  `write_timeout` refusal is asserted at `:145-152` inside the same test.
- `TestCardDAVConfigValidate` (`config_test.go:154`) — FR-013.
- `TestSplitList` (`config_test.go:94`) — FR-005.
- `TestEnvBinding_EveryBoundKeyReachesConfig` (`env_binding_test.go:94`),
  `TestEnvBinding_NoDefaultedKeyIsLeftUnbound` (`env_binding_test.go:121`) — FR-002, FR-003,
  FR-004, FR-015, SC-002. The second walks the defaults registry, which is why FR-015 requires
  defaults to be registered separately from loading.
- `TestLoadForCLI_DoesNotRequireTheSigningSecret`, `TestLoadForCLI_StillValidatesTheDatabase`,
  `TestLoadForCLI_MatchesLoadOnEveryOtherField` (`load_for_cli_test.go:12`, `:25`, `:35`) — FR-014.

**Schema and the migration runner** (`internal/repository/`)

- `TestMigrate_WorksFromAnyWorkingDirectory` (`migrate_runner_test.go:19`) — FR-017.
- `TestMigrateFS_EmptyFilesystemIsAnError` (`migrate_runner_test.go:38`) — FR-020's second half.
- `TestMigrateFS_FailedMigrationRollsBackEntirely` (`migrate_runner_test.go:49`) — FR-019, SC-003.
- `TestMigrateFS_AppliesInFilenameOrderAndRecordsEach` (`migrate_runner_test.go:83`) — FR-020,
  FR-022.
- `TestMigrateFS_IsIdempotent` (`migrate_runner_test.go:108`) — FR-021, SC-004.
- `TestMigrate_CreatesSchemaMigrationsTable`, `TestMigrate_Idempotent`,
  `TestMigrate_ExpectedTablesExist` (`migrate_test.go:51`, `:64`, `:78`) — the same guarantees
  against the real embedded migration set on SQLite.
- `TestPostgres_MigrateAppliesEverySchemaObject`, `TestPostgres_MigrateIsIdempotent`
  (`migrate_postgres_test.go:85`, `:112`) — FR-023, FR-024, SC-005. These are the tests the
  PostgreSQL CI job runs; the `TestPostgres…` prefix is load-bearing (see Known Divergences).
- `TestPostgres_Migration019SwapsInvertedRows` (`migrate_postgres_test.go:126`) and
  `internal/repository/migrate_019_test.go` — cover the day-one divergence recorded below.

**Request pipeline**

- `TestErrorHandler_InternalErrorIsNotDisclosed`,
  `TestErrorHandler_FiberErrorKeepsItsMessageAndStatus`, `TestErrorHandler_UnroutedPathStays404`
  (`cmd/server/main_test.go:17`, `:48`, `:69`) — FR-026, SC-006.
- `TestRequestLogger_EchoesASuppliedRequestID`, `TestRequestLogger_MintsAnIDWhenNoneWasSupplied`,
  `TestRequestLogger_MakesTheIDAvailableToHandlers`, `TestRequestLogger_LogsOrdinaryRequests`
  (`internal/handler/middleware/logger_test.go:47`, `:60`, `:72`, `:108`) — FR-027, SC-007.
- `TestRequestLogger_SkipsSuccessfulHealthChecks`, `TestRequestLogger_LogsAFailingHealthCheck`
  (`logger_test.go:84`, `:97`) — FR-028, SC-008.
- `TestBodyLimit_AllowsABodyWithinTheLimit`, `TestBodyLimit_RejectsOnDeclaredContentLength`,
  `TestBodyLimit_RejectsABodyThatDeclaredNoLength`, `TestBodyLimit_AllowsAnEmptyBody`,
  `TestBodyLimit_AnOverrideRaisesTheLimitForItsPath`
  (`internal/handler/middleware/bodylimit_test.go:26`, `:39`, `:61`, `:80`, `:92`) — FR-030,
  FR-031.
- `TestRateLimiterInstanceSharesBucket` (`internal/handler/middleware/ratelimit_test.go:42`) —
  FR-034, SC-009. *(Test file owned by spec 001.)*
- `TestRateLimiter_KeysPerForwardedClientWhenProxyTrusted`,
  `TestRateLimiter_IgnoresForwardedHeaderFromUntrustedPeer` (`ratelimit_test.go:91`, `:123`) —
  SC-011. The rule they enforce is spec 001 FR-044. *(Test file owned by spec 001.)*
- `TestReconcileInterruptedRuns_ClosesBothHistories`,
  `TestReconcileInterruptedRuns_LeavesRunsStartedAfterThisProcess`,
  `TestReconcileInterruptedRuns_SurvivesAFailure`,
  `TestReconcileInterruptedRuns_ToleratesMissingRepositories`
  (`cmd/server/startup_test.go:88`, `:105`, `:118`, `:132`) — FR-041, SC-013.
- `TestPruneSyncRuns_SkippedWhenRetentionIsOff` (`cmd/server/startup_test.go:138`) — FR-042, the
  retention-off branch only.

**Background execution** (`internal/worker/`)

- `TestWorker_PanicInHandlerDoesNotKillWorker` (`goroutine_worker_test.go:33`) — FR-045, SC-014.
- `TestWorker_HandlerErrorDoesNotStopWorker`, `TestWorker_UnknownJobTypeIsIgnored`
  (`goroutine_worker_test.go:63`, `:143`) — FR-045's "keeps running" half, FR-047.
- `TestWorker_StopDrainsQueuedJobs`, `TestWorker_StopRespectsContextDeadline`
  (`goroutine_worker_test.go:91`, `:129`), `TestWorker_StopWaitsForInFlightJob`,
  `TestWorker_StopInterruptsAfterDeadline`, `TestWorker_StopDrainsBufferedJobs`
  (`goroutine_worker_semantics_test.go:19`, `:53`, `:165`) — FR-046.
- `TestWorker_EnqueueDoesNotBlockForeverWhenFull`,
  `TestWorker_EnqueueReturnsPromptlyWithBackgroundContext`, `TestWorker_EnqueueAfterStopIsRefused`
  (`goroutine_worker_semantics_test.go:80`, `:110`, `:150`) — FR-044.
- `TestWorker_QueueDepthReportsBacklog` (`goroutine_worker_semantics_test.go:196`) — the depth
  FR-052 reports.
- `TestWorker_RegisterDuringRunIsRaceFree` (`goroutine_worker_semantics_test.go:224`) — FR-043,
  FR-047 under the race detector.
- `TestNewScheduler_Valid`, `TestStop_NoError` (`scheduler_test.go:40`, `:86`) — FR-048's
  construction and FR-040's scheduler stop.
- `TestRegisterPipelines_SkipsDisabled`, `TestRegisterPipelines_SkipsEmptySchedule`,
  `TestRegisterPipelines_AddsValidJob`, `TestRegisterBackupForUser`, `TestRegisterDedupForUser`
  (`scheduler_test.go:46`, `:57`, `:67`, `:78`, `:122`) — FR-048.
- `TestReregisterPipelineJob`, `TestReregisterPipelineJob_DisabledRemoves`,
  `TestReregisterBackupForUser`, `TestReregisterBackupForUser_EmptyRemoves`,
  `TestReregisterDedupForUser`, `TestReregisterDedupForUser_EmptyRemoves`, `TestRemovePipelineJob`,
  `TestRemoveDedupForUser` (`scheduler_test.go:157`, `:168`, `:208`, `:217`, `:139`, `:148`,
  `:94`, `:130`) — FR-049, SC-015.
- `TestValidateCron_Valid`, `TestValidateCron_Invalid` (`scheduler_test.go:179`, `:194`) — FR-050.
- `TestBackupPayload_Serializable`, `TestDedupPayload_Serializable` (`scheduler_test.go:105`,
  `:112`) — the payload half of FR-047.

**Observability** (`internal/handler/health_test.go`)

- `TestHealth_ReportsDatabaseReachable`, `TestHealth_ReportsDatabaseUnreachable`,
  `TestHealth_WithoutDatabaseStillReportsVersion`, `TestHealth_ReportsSchemaVersion` (`:49`, `:64`,
  `:75`, `:84`) — FR-052, SC-012.
- `TestHealth_QueueDepthDoesNotFailTheCheck` (`:103`) — FR-053.

**CLI dispatch** (`cmd/server/cli_test.go`)

- `TestRunCLI_NoArgumentsFallsThroughToTheServer` (`:29`) — FR-056.
- `TestRunCLI_FlagsFallThroughToTheServer` (`:35`) — FR-058.
- `TestRunCLI_UnknownSubcommandIsAUsageError` (`:41`) — FR-057, FR-059, SC-016.
- `TestSetPassword_RequiresAnEmail` (`:68`) — FR-057's usage path.
- `TestRunCLI_VersionAndHelp` (`:49`) — FR-063.
- `TestSetPassword_RefusesAPasswordArgument`,
  `TestSetPassword_NonTerminalWithoutStdinFlagIsRefused`, `TestSetPassword_EmptyStdinIsRefused`,
  `TestSetPassword_WarningsAreSpelledOut` (`:63`, `:76`, `:84`, `:118`) — FR-060.
- `TestParseInterleaved_AcceptsFlagsAfterPositionals`,
  `TestParseInterleaved_AcceptsFlagsBeforePositionals` (`:94`, `:105`) — FR-061.
- `TestSetPassword_RefusesAnUnmigratedDatabaseBeforePrompting` (`:207`) — FR-062. It asserts both
  exit `5` and that nothing was read from stdin.
- `TestSetPassword_EpilogueQuotesTheRunningConfiguration` (`:179`) and `TestHumanTTL` (`:129`) —
  the "returns the configuration it loaded" half of FR-062; the requirement they serve is
  **001 FR-050**.

**Delivery** (`.github/workflows/ci.yml`, by job and step name)

- Job **Test**: steps `Run tests` (race detector), `go vet`, `golangci-lint` — FR-073.
- Job **PostgreSQL**: step `Migration and repository tests against PostgreSQL` — FR-024.
- Job **Frontend**: steps `Lint`, `Check formatting`, `Unit tests` — FR-073.
- Job **Build**: steps `Build frontend`, `Build binary` — FR-064, FR-073.
- Job **Docker image**: steps `Plant a local config.yaml in the build context`,
  `Local config.yaml did not reach the image`, `Build image`, `Start container`, `Wait for health`,
  `Migrations ran inside the image`, `Runs unprivileged`, `set-password changes the password`,
  `set-password reports a missing user`,
  `The password never reaches the process list or the logs`,
  `vCard list separators survive a round trip`, `A manual backup is recorded in the history`,
  `reencode-vcards is a dry run by default and refuses a half job` — FR-070, FR-074, SC-017,
  SC-019.

**Web shell**

- `web/src/router/routes.spec.ts` — `has a route for every screen under views/` and
  `routes the sync providers screen` — FR-086, SC-021.
- `web/src/utils/cron.spec.ts` — `humanizeCron` and `schedule presets` — the humanising half of
  FR-083.
- `web/src/utils/format.spec.ts` — `formatSize`, `formatDuration`, `formatAgo` — the second
  sentence of FR-085.
- `web/src/api/client.spec.ts` — `getApiError` and `401 refresh flow` — FR-076, FR-077, SC-022.
  *(File owned by spec 001.)*

Requirements with **no** enforcer are named in Known Divergences below rather than padded out with
tests that do not exist.

## Known Divergences

**Silently ignored configuration**

- **`server.write_timeout` cannot be set from the environment.** It has no default and no entry in
  `envBoundKeys`, so `CHQ_SERVER_WRITE_TIMEOUT` is silently ignored; only a YAML entry reaches it
  (`internal/config/config.go:32-59`, `:100-103`). The guard test only walks keys that have
  defaults, so it does not catch this (`internal/config/env_binding_test.go:121-136`). The effect
  is benign — the only accepted value is the zero this produces — but it is the exact class of
  silent-ignore that FR-003 and FR-004 exist to prevent. Open: add it to `envBoundKeys` so an
  operator gets the documented refusal instead of silence.
- **`log.format` accepts exactly one special value.** `text` selects the development encoder;
  anything else, including a typo, silently becomes production JSON. An unparsable `log.level`
  likewise falls back to `info` without complaint (`internal/logger/logger.go:15-26`). FR-054
  states the fallback as intended behaviour; the *silence* around a typo is not intended, merely
  shipped.

**Stale or wrong strings**

- **The `.dockerignore` rationale overstates the risk in one direction.** Its comment says a baked
  `configs/config.yaml` "silently overrides every value passed through the environment"
  (`.dockerignore:1-5`). Verified against the real loader: an environment variable wins over the
  config file, because every key is explicitly bound (FR-002). The *leak* concern the file exists
  for is entirely real; the override claim is not. Open: correct the comment, or narrow it to the
  one key where it is true (`server.write_timeout`, above).
- **Cron expressions are interpreted in the scheduler's own timezone**, while the UI labels its
  custom cron field "(UTC)" (`web/src/components/ui/ScheduleInput.vue:27`). Nothing in the server
  pins a timezone, so the label is true only when the process runs in UTC — which the containers
  do, and a bare binary need not. Open: pin UTC server-side, or make the label reflect the process
  timezone.
- **Both server-rendered pages load Tailwind from `https://cdn.tailwindcss.com`** —
  `internal/web/templates/landing.html:7` (owned here) and
  `internal/web/templates/setup-guide.html:7` (owned by spec 004). An air-gapped or offline
  instance serves both unstyled, and each page makes a third-party request on behalf of every
  visitor. This spec previously claimed the landing page was the only such page and that
  "everything else in the product is embedded and self-contained"; that was false in the same way
  spec 004's mirror-image claim was. What *is* true is that the SPA and its styles are embedded.
  Open: vendor the stylesheet, drop it, or accept the outbound request — for both templates, not
  one.

**Behaviour that contradicts the project's own rules**

- **Three shipped migrations predate the migration rules this spec states, contradict them, and are
  applied on every existing install.** They are recorded rather than silently excused:
  - `019_normalize_pipeline_direction.up.sql` is three bulk `UPDATE`s and a `DELETE` — the exact
    shape constitution Principle I forbids and FR-025 excludes, with no dry run and no way to
    decline. It carries a long header comment explaining why the rewrite was necessary and is
    covered by dialect-specific tests (`internal/repository/migrate_postgres_test.go:126-175`,
    `internal/repository/migrate_019_test.go`).
  - `020_drop_dead_jobs_table.up.sql` is a `DROP TABLE`, which the forward-only rule forbids. The
    reasoning is stated in the file: the `jobs` table was never read or written, and leaving it in
    the schema implied a durability guarantee the system does not offer.
  - `011_drop_provider_unique.up.sql` is a `DROP INDEX`, likewise irreversible by the application.
  - Consequently `jobs` is deliberately absent from `expectedTables`, with a comment saying so
    (`internal/repository/migrate_postgres_test.go:56`).
- **Override matching in the body-limit middleware is `strings.Contains`, not a prefix match**
  (`internal/handler/middleware/bodylimit.go:70`). Any path containing `/import/` gets the import
  allowance. Harmless with today's single override; it is not the general path matcher FR-030 reads
  as promising.
- **The job queue is in memory and lossy.** A buffered channel of 100 in one process
  (`internal/worker/goroutine_worker.go:45`); a hard kill drops everything queued. FR-043 states
  the mechanism, not a durability guarantee, and none is offered. The `jobs` table was dropped in
  migration 020 precisely so the schema stops implying otherwise. Only backups have a compensating
  boot-time catch-up (owned by 005).
- **CORS is enabled with the default configuration** (`cmd/server/main.go:239`), i.e. any origin.
  The API is bearer-token authenticated rather than cookie authenticated, so this is not the same
  exposure it would otherwise be, but it is a framework default rather than a decision recorded in
  code — and FR-039 currently launders it into a requirement by naming CORS among the middleware
  that "MUST run ahead of every route".
- **`Makefile clean` deletes `contactshq.db`** (`Makefile:29`) — convenient in development, and
  destructive if the default SQLite DSN is ever a real deployment's database.
- **`formatDate` renders in the browser's local timezone with no timezone shown**
  (`web/src/utils/date.ts`). Two users in different zones can read different dates for the same
  record, which is the opposite of what FR-085's "so screens do not drift apart" claims to buy.

**Unenforced requirements — no test exists**

- **FR-001, FR-006, FR-009** have no direct test. Nothing asserts that a missing config file is
  normal, that `Load` refuses on a validation failure, or that a bad `database.driver` is rejected;
  `internal/config/config_test.go` contains no case for the driver or the DSN.
- **FR-016, FR-018, FR-029, FR-037, FR-038, FR-039, FR-040, FR-055, FR-065, FR-066, FR-069** are
  review-only. They are composition-root wiring in `cmd/server/main.go` with no test harness: the
  SQLite pragmas and single connection, migrate-before-anything-else, the global body ceiling, the
  read/idle timeouts, the WebDAV verb registration, middleware order, the bounded shutdown, the
  database-path log line, ldflags injection, the cgo-free driver and web-route ordering. The
  container exercise in CI covers some of them indirectly (a healthy container proves migrations
  ran and the binary is static) but asserts none of them by name.
- **FR-042 is half enforced.** `TestPruneSyncRuns_SkippedWhenRetentionIsOff` proves only that
  pruning is skipped when retention is off; nothing asserts that rows past retention are actually
  removed.
- **FR-051 has no test.** `NextBackupRun` (`internal/worker/scheduler.go:215-235`) is not covered
  by `scheduler_test.go`. `TestSchedulePeriod` (`cmd/server/startup_test.go:172`) tests
  `schedulePeriod` in `cmd/server/startup.go`, a different function belonging to spec 005's backup
  catch-up.
- **FR-054 has no test.** `internal/logger/` contains no test file at all; the level fallback and
  the format switch are asserted nowhere.
- **FR-071, FR-072 have no test.** Compose's refusal without `CHQ_AUTH_JWT_SECRET` and every
  release-workflow claim (architectures, archive contents, the `latest` guard, the hand-written
  release body) are verified by reading `docker-compose.yml`, `.goreleaser.yaml` and
  `.github/workflows/release.yml`, never by running them on a pull request. SC-018 and SC-020 rest
  on the same reading.
- **FR-078 through FR-082 and FR-084 have no tests.** There is no test file anywhere under
  `web/src/components/layout/`, `web/src/components/ui/` or `web/src/composables/`. The layout,
  the sidebar badges, the theme system, the toast queue, the focus trap, the route progress bar and
  the typed-word confirmation are all review-only. Only the `humanizeCron` half of FR-083 and the
  `format.ts` half of FR-085 are covered.
- **Trusted-proxy handling has to be applied twice and nothing enforces that the two stay in step.**
  Fiber's configuration does not reach the `/dav` mount, which is adapted from a `net/http` handler
  and does its own client attribution (`cmd/server/main.go:288-291`,
  `internal/carddav/throttle.go:166-190`). FR-036 states the wiring; no test compares the two call
  sites.

**Untested paths and CI gaps**

- **Nothing verifies that a migration is SQLite-and-PostgreSQL clean before it ships** except the
  PostgreSQL CI job, and that job runs only tests named `TestPostgres…` inside package `repository`
  (`.github/workflows/ci.yml:71-74`). A test written anywhere else never runs against PostgreSQL,
  and the contributor gets no warning.
- **The Docker CI job does not depend on the PostgreSQL job** (`needs: [test, frontend]`,
  `.github/workflows/ci.yml:134`), so a container image can be built and exercised while the
  PostgreSQL suite is still running or failing. The `build` job does depend on it. Open: is that
  deliberate — faster feedback on the image — or an oversight?
- **CI still creates the SPA stub directory by hand** (`.github/workflows/ci.yml:22-23`, `:68-69`)
  even though `internal/web/static/spa/.gitkeep` is committed and un-ignored (`.gitignore:42-43`).
  The step is redundant, not wrong; it also masks, for CI alone, what a clean checkout would show
  if the placeholder were ever deleted.
- **Rate-limit counters are in-process and in-memory.** A restart resets every bucket, and two
  instances would each hand out a full budget. SC-009 and SC-011 hold per process, not per
  deployment.

**Ownership still to settle**

- **FR-076 restates a rule spec 001 owns.** `web/src/api/client.ts` is 001's path and 001 FR-051 /
  FR-052 state the refresh flow. FR-076 is kept here because User Story 9 is meaningless without
  it, but two normative statements of one rule is what constitution Principle VII forbids. The
  clean resolution is to withdraw FR-076 the way FR-033 and FR-035 were withdrawn; it has not been
  done because 001's own repair is already published against the current numbering.
- **`.github/workflows/release.yml` and `scripts/release-notes.sh` are named by FR-072 but are not
  in this spec's ownership list.** They are the other half of the delivery chain and no other
  domain plausibly owns them. They must be claimed here or listed in `specs/UNCLAIMED.md` — which
  does not yet exist. This spec does not silently absorb them.
- **The same applies to `.dockerignore`, `.gitignore`, `.env.example`,
  `configs/config.example.yaml`, `.golangci.yml`, `.githooks/`, `.gitleaks.toml` and the frontend
  build configuration under `web/` (`vite.config.ts`, `package.json`, `tsconfig*.json`,
  `eslint.config.*`).** Each is cited by a requirement above and none is in the ownership list.
- **The SPA's tokens live in `localStorage`** (`web/src/api/client.ts:10`, `:52`), readable by any
  script that gets into the page. The choice is spec 001's to defend; it is recorded here because
  the shell is where it is visible.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-07 | v0.4.0-3-g23a167c | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0-3-g23a167c | Conformed to the house template: header block replaced (Kind `journey`, Status `shipped`, Constitution v1.0.0), `**Feature Branch**` and `**Input**` removed; Dependencies folded into References, Open Questions folded into Known Divergences; ownership list restated file by file with the frontend shell subpackages, `internal/repository/interfaces.go`, `internal/worker/jobs/deps.go` and the shared test/type files added, and `web/src/api/client.ts` moved to References. Withdrew FR-033 (credential rate-limit tiers → 001 FR-041/FR-042) and FR-035 (trusted-proxy keying → 001 FR-044) as labelled cross-references; narrowed FR-068 to the landing page, leaving `/setup` to 004 FR-033. Corrected the claim that the landing page was the only server-rendered page loading a CDN asset (`setup-guide.html:7` does too) in both Edge Cases and Assumptions. Moved every admission out of Edge Cases and Assumptions into Known Divergences and added the unenforced-requirement inventory. | — |
| 2026-08-07 | unreleased | FR-062 extended: `openCLIDatabase` now returns the loaded `*config.Config` alongside the connection, and the unmigrated-schema refusal is required to happen before any password prompt (`set-password` opens the database first). Enforcer citations for `cmd/server/cli_test.go` re-anchored after the file grew a database fixture. | D3 |
