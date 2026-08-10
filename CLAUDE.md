# ContactsHQ

Go API service for centralized contact management with CardDAV server, sync engine, and pipelines.

**Before changing behaviour, find the spec that owns the file.** `specs/` states what each
capability is supposed to do; `## Code Paths` in each spec is a path→spec lookup, and exactly one
spec owns any given path. Start at [specs/README.md](specs/README.md), and read
[.specify/memory/constitution.md](.specify/memory/constitution.md) for the rules that outrank
convenience. `make specs` fails the build when the tree stops being true. This file stays what it
has always been: the traps an agent hits while editing, not a behavioural specification.

## Build & Run

```bash
make build    # Build binary
make run      # Build and run
make test     # Run tests
make clean    # Clean artifacts
```

## Tech Stack

- **Go 1.25+**, **Fiber v2** (HTTP), **Bun** (ORM), **SQLite/PostgreSQL**
- **go-webdav** (CardDAV server), **go-vcard** (vCard parsing)
- **JWT** auth with argon2id password hashing
- **Viper** config (YAML + env vars)

## Project Structure

```
cmd/server/main.go          - Composition root
internal/config/             - Viper config loading
internal/domain/             - Domain entities (User, Contact, AddressBook, etc.)
internal/repository/         - DB init + Bun repository implementations
internal/service/            - Business logic (auth, contacts, import/export, backup)
internal/handler/            - Fiber HTTP handlers + middleware
internal/carddav/            - CardDAV server (go-webdav backend + CTag/sync-collection)
internal/sync/               - Sync engine, providers (internal, CardDAV, Google), incremental
internal/worker/             - Task queue (goroutine worker)
migrations/                  - SQL migrations, embedded via go:embed (migrations/embed.go)
configs/                     - YAML config files
```

## Config

Config via `configs/config.yaml` or env vars with `CHQ_` prefix:
- `CHQ_DATABASE_DRIVER` = sqlite | postgres
- `CHQ_DATABASE_DSN` = connection string
- `CHQ_AUTH_JWT_SECRET` = **required**, min 32 chars, no default — the server refuses to
  start without it or with a known placeholder. Generate with `openssl rand -hex 32`.
- `CHQ_SERVER_PORT` = HTTP port (default 8080)
- `CHQ_AUTH_ALLOW_REGISTRATION` = open public sign-up (default `false`)
- `CHQ_BACKUP_MAX_RESTORE_BYTES` = decompressed restore cap (default 128 MiB)
- `CHQ_SERVER_MAX_BODY_BYTES` = 32 MiB; the only limit that bounds memory (see body limits below)
- `CHQ_SERVER_MAX_IMPORT_BYTES` = 32 MiB; validation refuses a value above max_body_bytes
- `CHQ_SERVER_READ_TIMEOUT` / `CHQ_SERVER_IDLE_TIMEOUT` = 30s / 120s. There is deliberately **no**
  write timeout and config validation refuses a non-zero one
- `CHQ_CARDDAV_MAX_RESOURCE_BYTES` = 1 MiB, enforced on `/dav` PUT only
- `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS` = permit `http://` provider endpoints (default `false`)
- `CHQ_SYNC_RUNS_RETENTION_DAYS` / `CHQ_MERGE_LOG_RETENTION_DAYS` = 90 / 30
- `CHQ_AUTH_TOKEN_TTL` / `CHQ_AUTH_REFRESH_TTL` = 1h / 168h since v0.4.0 (were 24h / 720h). There
  is no token denylist, so these lifetimes **are** the revocation window; rotating the JWT secret
  is the only way to cut every session at once

Every key needs an explicit entry in `envBoundKeys` (`internal/config/config.go`) or its
`CHQ_` variable is silently ignored — `TestEnvBinding_NoDefaultedKeyIsLeftUnbound` enforces this.

## API

This is a partial map, not a reference — `internal/handler/handler.go` registers 78 routes
(`grep -cE '\.(Get|Post|Put|Delete|Patch)\("' internal/handler/handler.go`),
plus the `/dav` mount and the web routes in `internal/web/handler.go` and `cmd/server/main.go`.
**Read `handler.go` before assuming a route's shape.** There is no generated API spec.

All API endpoints live under `/api/v1/`:
- **Auth** (public, rate-limited): `POST /auth/register`, `/auth/login`, `/auth/refresh`
- `GET/PUT/DELETE /users/me`, `PUT /users/me/password`
- **Contacts**: `GET/POST /contacts`, `GET/PUT/DELETE /contacts/:id`,
  `POST /contacts/bulk-delete`, `DELETE /contacts/` (all), `GET /contacts/facets`,
  `GET /contacts/:id/vcard`, `GET /contacts/:id/qrcode`
- **Duplicates**: `GET /contacts/duplicates`, `/duplicates/count`, `/duplicates/settings`,
  `POST /contacts/duplicates/detect`, `/duplicates/:id/dismiss`, `POST /contacts/merge`
- `POST /import/vcard`, `/import/csv`
- `GET /export/vcard` (optional `?ids=`), `/export/csv`, `/export/json`
- `GET/POST/PUT/DELETE /pipelines`, `POST /pipelines/:id/trigger`, `GET /pipelines/:id/runs`
- **Backup**: `POST /backup/create`, `GET /backup/list`, `GET /backup/runs`, `GET /backup/status`,
  `GET/PUT /backup/settings`,
  `GET /backup/download/:id`, `DELETE /backup/:id`, `POST /backup/restore/:id` (`?mode=`)
- **Sync**: `GET /sync/providers`, `/status`, `/history`; `POST /sync/carddav/connect`
  and `/sync/carddav/trigger`; `DELETE /sync/providers/:id`. Google is connected through
  `/auth/google/*`, not here — the `/sync/google/*` pair were 501 stubs nothing called and
  are gone
- **Sync conflicts**: `GET /sync/conflicts`, `/conflicts/count`, `/conflicts/:id`,
  `POST /sync/conflicts/:id/resolve`, `/conflicts/:id/dismiss`
- **Credentials**: `GET/POST /credentials`, `GET/PUT/DELETE /credentials/:id`
- **App passwords**: `GET/POST /app-passwords`, `DELETE /app-passwords/:id`
- **Admin** (`AdminOnly`): `GET/POST /admin/users`, `PUT /admin/users/:id/role`,
  `DELETE /admin/users/:id`
- `GET /setup/ios-profile`, `GET/POST/DELETE /auth/google/*`

Outside `/api/v1/`:
- CardDAV: `/dav/{email}/addressbooks/contacts/`
- `GET /health` — reports DB connectivity (503 when unreachable)

**Error bodies** are always `{"error": "..."}`. A `*fiber.Error` keeps its message; any other
error is logged and answered with a fixed `"internal server error"` — never return an internal
error's text to the client (`newErrorHandler` in `cmd/server/main.go`).

See [docs/sync.md](docs/sync.md) for how the sync engine works.

## CLI subcommands

The server binary doubles as an admin CLI (`cmd/server/cli.go`, no cobra):

```bash
contactshq                        # no arguments → start the server
contactshq set-password <email>   # recover a forgotten password
contactshq reencode-vcards        # rewrite stored vCards with the current encoder
contactshq version | help
```

- Dispatch happens in `main()` **before** `config.Load()`, via `config.LoadForCLI()` — a
  subcommand touches only the database and must not require `CHQ_AUTH_JWT_SECRET`. Never
  weaken `Config.Validate()` itself: refusing to serve without a real secret is load-bearing.
- The first argument is matched against a whitelist. An unrecognised one is a usage error
  (exit 2), never a silent fallthrough to starting the server.
- **Never accept a secret as a command-line argument** — argv shows up in `ps`,
  `/proc/<pid>/cmdline`, shell history and `docker inspect`. Prompt without echo, or read
  stdin behind an explicit `--stdin`.
- Use `parseInterleaved`, not `fs.Parse`: `flag` stops at the first positional, so a flag
  written after the email would be silently ignored.
- Subcommands do **not** run migrations — they refuse to work on an empty schema (exit 5).
  The server owns migration, and a second process racing it has no upside.
  That check runs **before** the password prompt: `set-password` used to ask twice and only then
  exit 5. One consequence to expect — against an unmigrated database a run that would previously
  have reported a usage error (exit 2, a missing `--stdin` say) now reports 5, because the
  database is reached first.
- `set-password`'s closing message quotes `auth.token_ttl` and `auth.refresh_ttl` **from the
  config the command itself loaded**, not a hardcoded number — the previous text still said
  24h/720h after v0.4.0 changed them, and telling an operator how long the old session outlives
  the reset is that message's only job. A subcommand cannot see the environment of a server it is
  not running in, so it must be run where the server's config is (`docker compose exec`).

### Bulk data repairs are commands, never migrations

`reencode-vcards` is the worked example. A mass `UPDATE` in a migration file is forbidden
(also noted under Migrations above) because `applyMigration` runs the file inside `RunInTx`
and SQLite's pool is one connection wide: the rewrite holds that connection, the container
health check (10s start period) reports unhealthy, and compose restarts the process in the
middle of the transaction. A migration also offers no dry run and no way to decline.

Such a command must: default to a dry run, batch its writes, refuse to do half the job (here
`--apply` requires `--reconcile-sync-state`), and state plainly that there is no undo.

**Why the reconcile half is mandatory:** rewriting cards changes `contacts.etag` while
`sync_states.local_etag` still holds the old value, so the engine reads the entire address
book as locally modified and the next `export`/`two_way` run pushes all of it to Google or
CardDAV. `base_vcard` is re-encoded and `content_hash` recomputed from it; `remote_etag` is
left alone because nothing here touched the remote side.

## Conventions & gotchas

- **Migrations** are embedded and applied transactionally at startup; a binary can run
  from any working directory. Add a numbered `NNN_name.up.sql` + `.down.sql` pair.
- **Migrations are forward-only.** `.down.sql` files are embedded but **no code ever applies
  them** — `MigrateFS` globs `*.up.sql` only (`internal/repository/db.go`). Rolling back means
  restoring a database dump. Consequences for every new migration:
  - Allowed: `CREATE TABLE`, `CREATE INDEX`, `ADD COLUMN` **with a DEFAULT**.
  - Forbidden: `DROP COLUMN`, renames, `NOT NULL` without a DEFAULT, and
    `CREATE INDEX CONCURRENTLY` — `applyMigration` runs each file inside `RunInTx`, and
    PostgreSQL rejects CONCURRENTLY in a transaction.
  - No bulk `UPDATE` in a migration: it runs on every install with no way back and no dry run.
    Do data rewrites as an explicit, resumable command instead.
  - **Numbers are assigned at merge time, not planning time.** Two branches that both claim
    `023` will diverge across installs and `schema_migrations` will record incompatible
    states. Renumber on rebase.
  - `schema_migrations.version` holds the migration's **filename stem** (`025_backup_runs`),
    not a number. `SchemaVersion` returns a string for that reason; the names are zero-padded
    so the lexicographic maximum is also the numeric one. **25 migrations** as of v0.3.0.
- **A new table means a new line in `expectedTables`** (`internal/repository/migrate_postgres_test.go`)
  **in the same PR** — that test compares the live schema against the list in both directions.
- **PostgreSQL coverage naming**: CI runs only `go test ./internal/repository/ -run TestPostgres`.
  A test that must pass on PostgreSQL has to live in package `repository` *and* be named
  `TestPostgres…`; anywhere else it simply never runs against PostgreSQL.
- **First registered user is the admin.** No other code assigns the admin role.
- **Backup history lives in `backup_runs`, written inside `BackupService.CreateWithTrigger`**
  — not in the scheduled job, because the manual `POST /backup/create` runs synchronously in
  the handler and would otherwise never be recorded. Finalisation uses
  `context.WithoutCancel`: on the caller's context a graceful shutdown leaves the row stuck at
  `running` forever.
- **Startup reconciliation is bounded by process start time.** `MarkStaleInterrupted` only
  closes runs that began before this process did; an unbounded UPDATE would mark a second
  instance's live runs as interrupted. Real leases are the multi-instance answer and this
  project does not support that configuration.
- **Public registration is closed by default** (`auth.allow_registration`, default `false`).
  Creating the *first* account always works so an instance can be bootstrapped; after that
  `POST /auth/register` returns 403. `POST /admin/users` uses `RegisterBypassPolicy` and is
  deliberately exempt — do not point it back at `authHandler.Register`.
- **CardDAV path layout** is `/dav/{email}/addressbooks/contacts/{uid}.vcf`; the segment
  depths matter to go-webdav's resource routing (principal/home-set/book/object). Build
  paths with the exported helpers in `internal/carddav`, never by hand.
- **Fiber `RequestMethods`** must include the WebDAV verbs (PROPFIND, REPORT, …) or the
  `/dav` mount is unreachable — set in `cmd/server/main.go`.
- **Pipeline steps**: source is always an external provider, dest is always `internal`;
  `direction` is `import` / `export` / `two_way` (legacy `pull`/`push`/`bidirectional`
  still parse). Provider-to-provider steps are rejected.
- **vCard encoding goes through `internal/vcard` only.** `EncodeCard` is the single writer;
  `gvcard.NewEncoder` must not be called anywhere else. go-vcard escapes every value the same
  way, which corrupts URI values (`PHOTO:…base64\,…` — this broke photos on iOS), list
  separators (`CATEGORIES:work\,friends` becomes one category) and structured values, and it
  passes a bare CR through. Escaping is chosen per property type; see the comment at the top
  of `encoder.go`.
  - **Known, deliberate gap:** a `;` inside a single-valued TEXT property is *not* escaped,
    although RFC 6350 §3.4 requires it. go-vcard's decoder has no case for `\;`, so emitting
    it would make this application misread its own notes, and undoing it after decoding is
    ambiguous. Fixing it means owning the decode path.
  - Changing the encoder moves `ContentHash` and `LocalETag` for affected cards. Anything
    that touches it needs the maintenance-window treatment described under `reencode-vcards`.
- **Body limits**: `server.max_body_bytes` is the only one that bounds memory — fasthttp reads
  a whole body before any handler runs, so N concurrent uploads cost N × that. The per-route
  `middleware.BodyLimit` is POLICY (a clear 413), not protection. It resolves the limit per
  path in ONE middleware: mounting a narrow limit on a parent group and a wider one on a child
  does not work, because both run and the parent's 413 wins.
- **`server.write_timeout` must stay 0** and config validation refuses to start otherwise:
  `POST /backup/restore/:id` and `POST /import/*` run synchronously inside the request, so a
  write deadline truncates an operation that is still changing contacts.
- **`ctx.Err()` in long loops, but never between `DeleteAll` and the inserts** in restore —
  cancelling there leaves an empty address book, turning "more cancellable" into data loss.
  With fasthttp, `c.Context()` is only cancelled at server shutdown and `c.UserContext()`
  returns `context.Background()` when unset; swapping them changes nothing.
- **Provider endpoints are validated at all four entry points** (`ValidateProviderEndpoint`):
  CardDAV connect, stored credentials, the endpoint inside a pipeline step's JSON, and the
  trigger endpoint posted in a request body. The client also refuses cross-host redirects —
  validating the string alone would not stop `302 → 169.254.169.254`. Private addresses are
  deliberately NOT filtered: LAN CardDAV is a supported setup.
  **Entry points are not enough, and this is the trap:** a pipeline step naming a
  `credential_id` carries no endpoint of its own, so step validation had nothing to look at and
  a credential stored before the check existed was used as it stood. `createProvider`
  (`internal/sync/pipeline.go`) therefore resolves the credential first and validates the value
  it will actually build the provider from — before any network call, including the OAuth token
  exchange. Anything new that turns stored configuration into a connection owes the same check.
  `skip_tls_verify` is copied from the credential and is still never validated.
- **Sync**: `SyncProvider.Put` returns the id the provider assigned (Google mints its
  own). Incremental providers implement `IncrementalProvider` (delta + cursor);
  conditional writers implement `ConditionalWriter` (If-Match / ETag). The engine falls
  back to a full listing when neither is available.
- **A slice that reaches the browser must be `[]`, never nil.** `json.Marshal` writes a nil slice
  as the four bytes `null`, and `JSON.parse("null")` returns null **without throwing** — so a
  `try`/`catch` on the client does not catch it and the next `.forEach` dies. This blanked both
  sync-conflict screens for the *ordinary* manual conflict (two sides editing different
  properties, hence no field diffs). `recordConflict` now starts from `[]FieldConflict{}`, and
  `parseFieldDiffs` (`web/src/utils/sync-conflicts.ts`) coerces a non-array to `[]` — that guard
  is permanent, because nothing rewrites rows already stored as `null`.
- **`carddav.max_resource_bytes` is enforced in `Backend.PutAddressObject`**, after go-webdav has
  already decoded the body, so it is POLICY not protection (see the body-limits rule above) — it
  keeps an oversized card out of the database and gives the device a specific refusal. It binds
  the `/dav` write path only: the API, imports and inbound sync can still store a larger card, and
  `GET` still serves it, so an oversized contact becomes read-only *from devices*. Note `/dav` does
  not pass through `newErrorHandler` — `internal/server.go` echoes the error text — so the usual
  "never return an internal error's text" rule does not apply there and messages must be written
  with that in mind.
- **Duplicate detection reads child tables through `ListDedupValues`**, two narrow
  `(contact_id, value)` projections — never a relation load and never a join. The detector was
  rewritten for scale and the numbers are load-bearing: 10k contacts run at ~11 ms / 7.3 MB
  (18.8 ms / 16.8 MB with secondary values) against 39 s / 13.6 GB before. Neither benchmark runs
  in CI and both use an in-memory mock, so the query cost is argued from shape, not measured.
- **`fields` is a full replacement of the properties it covers.** A `PUT` whose `fields` object
  omits `geo` clears `GEO`, exactly as omitting `note` clears the note. The web form has no GEO
  input and therefore passes the stored value through untouched rather than submitting an empty
  one; any new caller that builds `fields` from a partial form owes the same care.

## Docker

```bash
cp .env.example .env
echo "CHQ_AUTH_JWT_SECRET=$(openssl rand -hex 32)" > .env
docker compose up -d   # Runs with PostgreSQL; compose refuses to start without the secret
```
