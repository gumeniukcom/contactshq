# ContactsHQ

A self-hosted contact management hub with a CardDAV server, multi-provider sync engine, and a modern web UI. Designed to be the single source of truth for all your contacts — no matter where they originally live.

Current release: **v0.5.0**. [CHANGELOG.md](CHANGELOG.md) lists what changed and what has to
be checked *before* upgrading — 0.5.0 starts enforcing the CardDAV card-size limit it had only
been advertising, which can make an oversized contact read-only from every device, and refuses
`http://` sync credentials stored before 0.4.0.

## What it does

- **Centralized address book** — store and manage all contacts in one place with full vCard 4.0 support (names, emails, phones, addresses, IMs, URLs, categories, dates, and more)
- **CardDAV server** — expose your contacts as a standard CardDAV endpoint, compatible with macOS Contacts, iOS, Thunderbird, and any CalDAV/CardDAV client; supports CTag and RFC 6578 collection sync, so clients fetch only what changed
- **Sync pipelines** — move contacts between external providers (Fastmail, iCloud, Nextcloud, Google) and your address book on a schedule or on demand; each step is an import, an export, or a two-way sync ([how it works](docs/sync.md))
- **Incremental sync** — providers that support it (Google via a sync token, CardDAV via RFC 6578) send only what changed since the last run; exports write conditionally so a concurrent edit becomes a conflict rather than being overwritten
- **Three-way merge** — when a contact is modified both locally and on a remote source, the engine merges changes field-by-field automatically; unresolvable conflicts are queued for manual review
- **Conflict resolution UI** — inspect field-level diffs between base/local/remote versions and resolve each field individually; when a conflict has no diff attributable to a single property, whole-card actions are offered instead. "Apply remote" replaces the contact outright, discarding anything that exists only locally, so it asks first and there is no undo in the app
- **Duplicate detection** — pairs are found by a shared email address or a normalised phone number, counting *every* address and number a contact holds rather than only its first, and each pair says which one it was ("Same email: a@b.c") rather than showing a percentage. A value shared by more than 500 contacts is treated as saying nothing about identity and its whole group is skipped, with a log warning and nothing in the UI
- **Contact merge** — resolve the result value by value, so the work address from one record and the home address from the other is expressible; the merge is one transaction that transfers sync state, tombstones the discarded card, and keeps a 30-day history you can undo from by hand
- **Import / Export** — import vCard (.vcf) and CSV files; export to vCard, CSV, or JSON
- **Backup & restore** — scheduled or on-demand backups with optional gzip compression, configurable retention, and merge/replace restore modes
- **QR codes** — generate a QR code for any contact (vCard payload, scannable by phones)
- **Multi-user** — each user has an isolated address book; the first registered account is the administrator
- **Self-hosted** — runs as a single binary or via Docker Compose with PostgreSQL

## Tech stack

### Backend
| Component | Technology |
|---|---|
| Language | Go 1.25 |
| HTTP framework | [Fiber v2](https://github.com/gofiber/fiber) |
| ORM | [Bun](https://bun.uptrace.dev) |
| Database | SQLite (development) / PostgreSQL 16 (production) |
| CardDAV server | [go-webdav](https://github.com/emersion/go-webdav) |
| vCard parsing | [go-vcard](https://github.com/emersion/go-vcard) |
| Authentication | JWT (golang-jwt/jwt v5) + Argon2id password hashing |
| Configuration | [Viper](https://github.com/spf13/viper) (YAML + env vars) |
| Scheduler | [gocron v2](https://github.com/go-co-op/gocron) |
| Logging | [zap](https://github.com/uber-go/zap) |
| Migrations | Embedded SQL, applied transactionally at startup |

### Frontend
| Component | Technology |
|---|---|
| Framework | Vue 3 (Composition API) |
| Build tool | Vite |
| Styling | Tailwind CSS v4 |
| State management | Pinia |
| Routing | Vue Router |
| HTTP client | Axios |
| Language | TypeScript |

The SPA is embedded into the Go binary via `go:embed` and served directly from the server — no separate frontend deployment needed.

## Getting started

### Run with Docker Compose (recommended)

```bash
git clone https://github.com/gumeniukcom/contactshq
cd contactshq

# Required: generate a signing secret. Compose refuses to start without it.
cp .env.example .env
echo "CHQ_AUTH_JWT_SECRET=$(openssl rand -hex 32)" > .env

docker compose up -d
```

The app will be available at `http://localhost:8080`. The first account you register becomes the administrator.

### Run locally

```bash
# Build frontend + backend
make build

# Or just run in development mode
make run
```

Default config uses SQLite. The database file `contactshq.db` is created automatically.

### Configuration

Copy `configs/config.example.yaml` to `configs/config.yaml` and adjust as needed, or use environment variables with the `CHQ_` prefix:

```yaml
server:
  port: 8080

database:
  driver: sqlite          # sqlite | postgres
  dsn: contactshq.db     # file path for SQLite, or postgres DSN

auth:
  jwt_secret: ""          # required, min 32 chars — openssl rand -hex 32
  token_ttl: 1h
  refresh_ttl: 168h
```

`auth.jwt_secret` has no default: the server refuses to start when it is missing, shorter
than 32 characters, or set to a known placeholder such as `change-me-in-production`. Anyone
who knows the signing secret can mint tokens for any account, including admins.

| Env variable | Description |
|---|---|
| `CHQ_DATABASE_DRIVER` | `sqlite` or `postgres` |
| `CHQ_DATABASE_DSN` | SQLite file path or PostgreSQL connection string |
| `CHQ_AUTH_JWT_SECRET` | **Required.** JWT signing secret, min 32 chars |
| `CHQ_AUTH_ALLOW_REGISTRATION` | Open public sign-up (default `false` — see below) |
| `CHQ_SERVER_PORT` | HTTP port (default `8080`) |
| `CHQ_SERVER_TRUSTED_PROXIES` | Comma-separated proxy IPs/CIDRs whose `X-Forwarded-For` to trust (set behind a reverse proxy) |
| `CHQ_BACKUP_MAX_RESTORE_BYTES` | Cap on decompressed restore size (default `134217728`, 128 MiB) |
| `CHQ_MERGE_LOG_RETENTION_DAYS` | How long merge history is kept (default `30`) |
| `CHQ_SYNC_RUNS_RETENTION_DAYS` | How long pipeline run history is kept (default `90`) |
| `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS` | Permit plain `http` provider URLs (default `false`) |
| `CHQ_SERVER_MAX_BODY_BYTES` | Largest request body (default `33554432`, 32 MiB) |
| `CHQ_SERVER_MAX_IMPORT_BYTES` | Largest import upload; must not exceed the above |
| `CHQ_SERVER_READ_TIMEOUT` | How long a client may take to send a request (default `30s`) |
| `CHQ_SERVER_IDLE_TIMEOUT` | How long an idle keep-alive connection is held (default `120s`) |
| `CHQ_CARDDAV_MAX_RESOURCE_BYTES` | Largest single vCard a device may upload over CardDAV (default `1048576`, 1 MiB) |

There is deliberately no write timeout, and the server refuses to start if you set one:
restore and import run synchronously inside the request, so a write deadline would truncate
an operation that is still changing contacts.

> **On body limits:** `CHQ_SERVER_MAX_BODY_BYTES` is the only one that actually bounds memory —
> fasthttp reads a whole body before any handler runs, so N concurrent uploads cost
> N × that value. Set it to what import genuinely needs, not "with room to spare". The
> per-route limits give a clear 413 where a large body is meaningless; they are policy, not
> protection.

### Who can create an account

`POST /auth/register` is **closed by default**. Creating the *first* account is always
allowed — that is how a new instance is bootstrapped, and the first account becomes the
administrator. After that, public sign-up returns `403` unless
`CHQ_AUTH_ALLOW_REGISTRATION=true`.

An administrator can add users at any time from **Admin → Users**, which goes through
`POST /admin/users` and is unaffected by this setting. `GET /auth/config` reports
`{"registration_open": bool}` without authentication, so you can check what an instance
currently accepts.

> **Upgrading:** instances that relied on open registration must set
> `CHQ_AUTH_ALLOW_REGISTRATION=true` to keep it. Everyone else gains a closed endpoint that
> previously handed an account to anyone who could reach the port.

### Where sync is allowed to connect

A sync request carries the provider's username and password, so the endpoint is validated
before anything is fetched. Only `https` is accepted by default; `file://`, `gopher://` and a
bare hostname are refused outright, as is a URL carrying credentials in its userinfo. The
check runs at all four places an endpoint can enter the system: CardDAV connect, a stored
credential, the endpoint inside a pipeline step's config, and one posted to a manual trigger.

Private addresses are **not** filtered — a CardDAV server on your LAN is a supported setup.
What would make that dangerous is handled directly instead: the client follows at most three
redirects and refuses to cross hosts, so a permitted host answering `302` toward a cloud
metadata address goes nowhere.

If your CardDAV server is reachable only over plain http, opt in explicitly:

```bash
CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true
```

> **Upgrading:** a step will fail every run until this is set, whether its endpoint is written
> inline or comes from a credential stored before 0.4.0 — a stored endpoint is validated when
> the credential is resolved, immediately before the connection is made. Nothing rewrites rows
> that already exist; the credential is still there and this variable brings it back.
> The failure is recorded against that step alone, so other steps in the same pipeline still run.
> Prefer fixing the transport where you can: the password travels in every request. Note also
> that this variable and the per-credential `skip_tls_verify` box are different things — the
> latter keeps `https://` in the URL while removing the protection it stands for.

### Forgotten password

There is no email-based reset — the server sends no mail. Recover access from the machine
that runs it, using a subcommand of the same binary:

```bash
# Prompts twice, without echoing.
./contactshq set-password you@example.com

# Or read it from a pipe, for scripted use.
printf '%s\n' "$NEW_PASSWORD" | ./contactshq set-password you@example.com --stdin
```

Under Docker, run it inside the same container: the SQLite file belongs to uid 10001, and a
second process outside the container would be pointing at a different filesystem.

```bash
printf '%s\n' "$NEW_PASSWORD" | docker compose exec -T app ./contactshq set-password you@example.com --stdin
```

The password is never taken as a command-line argument — argv is visible in `ps`, shell
history and `docker inspect`.

Two things the command deliberately does not do, and says so when it runs:

- **Existing sessions stay signed in.** Access tokens keep working for their full lifetime
  (`auth.token_ttl`, 1h by default), refresh tokens for theirs (`auth.refresh_ttl`, 168h).
  The command prints the values *it* read, so run it where the server's configuration is —
  inside the container, not beside it. Rotate `CHQ_AUTH_JWT_SECRET` and restart to sign
  everyone out.
- **A running server keeps its cached CardDAV verdicts for up to 5 minutes**, because the
  subcommand runs in its own process. Restart the server to drop the cache at once. A
  password changed through the web UI takes effect for CardDAV immediately.

Exit codes: `0` success, `2` usage error, `3` no such user, `4` database unreachable,
`5` database has no schema yet (start the server once so it can migrate). The database is
checked before the password is asked for, so an unmigrated one fails with `5` rather than
after you have typed a password twice.

### Repairing vCards written by an older version

Earlier releases escaped every vCard value the same way, which corrupted three kinds of
value: photo and URL data (`PHOTO:data:image/jpeg;base64\,…`, which stops photos rendering on
iOS), category separators (`CATEGORIES:work\,friends` — one category, not two), and values
containing a carriage return. New and edited contacts are written correctly from this release
on; cards already stored are not touched until you ask.

```bash
./contactshq reencode-vcards                                   # dry run: reports, changes nothing
./contactshq reencode-vcards --apply --reconcile-sync-state    # rewrites
```

**Before running it with `--apply`:**

1. Take a database dump. There is no undo.
2. Disable your sync pipelines, and re-enable them afterwards.
3. Expect every CardDAV client to re-download the address book — rewritten cards get new
   ETags.

`--reconcile-sync-state` is required rather than optional on purpose. Rewriting cards moves
`contacts.etag` while `sync_states.local_etag` still holds the old value, so the sync engine
would read the whole address book as locally modified and push all of it to Google or CardDAV
on the next run. The flag brings the sync state back in line in the same pass.

Running it twice is safe: the second pass reports nothing to do.

### Revoking every session at once

There is no token denylist — checking one would mean a database read on every authenticated
request, and this deployment runs with a single connection. The token lifetime is therefore
the revocation window: an access token is valid for an hour, a refresh token for a week.

To cut every session immediately, change the signing secret and restart:

```bash
CHQ_AUTH_JWT_SECRET=$(openssl rand -hex 32)   # then restart the server
```

Every issued token stops verifying at once and everyone signs in again. This is the supported
answer to "a token leaked" and to "I want everyone out now".

### Knowing whether backups work

Backups are recorded in the database, not inferred from the files on disk — retention deletes
those, and at `retention: 1` the only file present is always the newest one, so "when did this
last succeed?" had no answer.

- The **Backup** screen opens with one line: healthy, failing, overdue, never run, or off,
  with the error from the last failed attempt when there is one.
- `GET /backup/runs` returns the history; `GET /backup/status` returns last success, last
  attempt and the next scheduled run. Both are per user and behind authentication — `/health`
  is public and deliberately carries none of this.
- Every attempt is recorded, including a manual one from the UI, so the status cannot quietly
  describe only the scheduled runs.

Two things happen at startup:

- **Interrupted runs are closed.** A container killed mid-backup would otherwise leave rows
  reading "running" forever. Only runs that began before the process started are touched, so
  the reconciliation is safe on the supported single-instance deployment and harmless rather
  than destructive if a second instance is running.
- **A missed backup is caught up.** If scheduled backups are on and the last success is older
  than the schedule allows, one backup is queued — recorded with `trigger: catchup`. A cron
  schedule alone does not cover this: if the machine was off overnight, the next firing is
  tomorrow and the day's backup is simply gone.

### Finding a request in the log

Every request is logged with an id, taken from `X-Request-Id` when a reverse proxy supplies
one and minted otherwise, and echoed back in the response header. That is the handle for
"it failed at 14:32": ask for the id from the response, then grep the log for it.

A *successful* `/health` check is not logged. The container health check polls it every 30
seconds, and an idle instance's log would otherwise consist of nothing else. A failing one is
always logged.

## Connect your devices

ContactsHQ includes a built-in CardDAV server. Connect your iPhone, iPad, Mac, or Thunderbird to sync contacts automatically.

- Visit `/setup` on your instance for step-by-step instructions
- In the app, go to **Settings → Connect Devices** for one-tap iOS profile download
- Use **App Passwords** (Settings → App Passwords) instead of your main password for CardDAV clients
- HTTPS is required for mobile clients — see [reverse proxy examples](docs/reverse-proxy.md)

> **A card a device uploads must fit `CHQ_CARDDAV_MAX_RESOURCE_BYTES`** (1 MiB by default) or the
> `PUT` is refused with `413`. The limit binds the CardDAV write path only: a bigger contact can
> still arrive through the API, an import or an inbound sync, and it will still sync *down* to the
> phone — but the phone can then never save an edit to it, and most CardDAV clients retry silently
> rather than say so. An embedded photo is the usual way a contact gets that large. If you have
> such contacts, raise the limit rather than leaving them read-only on every device.

## API

All endpoints are under `/api/v1/`. Authentication uses Bearer JWT tokens. This is a
selection, not a reference — the router registers rather more than fits here, and
`internal/handler/handler.go` is the authoritative list. Errors are always
`{"error": "..."}`.

```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
GET    /api/v1/auth/config      (public: {"registration_open": bool})

GET    /api/v1/users/me
PUT    /api/v1/users/me
PUT    /api/v1/users/me/password
DELETE /api/v1/users/me

GET    /api/v1/contacts
POST   /api/v1/contacts
GET    /api/v1/contacts/:id
PUT    /api/v1/contacts/:id
DELETE /api/v1/contacts/:id
POST   /api/v1/contacts/bulk-delete   (body: {"ids": [...]}, max 500)
DELETE /api/v1/contacts          (delete all)
GET    /api/v1/contacts/:id/vcard
GET    /api/v1/contacts/:id/qrcode
GET    /api/v1/contacts/facets   (categories, organisations and counts for the filter bar)

POST   /api/v1/import/vcard
POST   /api/v1/import/csv
GET    /api/v1/export/vcard      (optional ?ids=id1,id2 to export a selection)
GET    /api/v1/export/csv
GET    /api/v1/export/json

GET    /api/v1/pipelines
POST   /api/v1/pipelines
GET    /api/v1/pipelines/:id
PUT    /api/v1/pipelines/:id
DELETE /api/v1/pipelines/:id
POST   /api/v1/pipelines/:id/trigger
GET    /api/v1/pipelines/:id/runs

GET    /api/v1/sync/conflicts
GET    /api/v1/sync/conflicts/count
GET    /api/v1/sync/conflicts/:id
POST   /api/v1/sync/conflicts/:id/resolve
POST   /api/v1/sync/conflicts/:id/dismiss

GET    /api/v1/contacts/duplicates
GET    /api/v1/contacts/duplicates/count
GET    /api/v1/contacts/duplicates/:id      (one pair, every value of both contacts)
POST   /api/v1/contacts/duplicates/detect
POST   /api/v1/contacts/duplicates/:id/dismiss
GET    /api/v1/contacts/duplicates/settings
PUT    /api/v1/contacts/duplicates/settings
POST   /api/v1/contacts/merge
GET    /api/v1/contacts/merge-log           (with a snapshot of the discarded card)

GET    /api/v1/sync/providers
DELETE /api/v1/sync/providers/:id
POST   /api/v1/sync/google/connect
POST   /api/v1/sync/google/trigger
POST   /api/v1/sync/carddav/connect
POST   /api/v1/sync/carddav/trigger
GET    /api/v1/sync/status
GET    /api/v1/sync/history

GET    /api/v1/credentials
POST   /api/v1/credentials
GET    /api/v1/credentials/:id
PUT    /api/v1/credentials/:id
DELETE /api/v1/credentials/:id

GET    /api/v1/backup/list
POST   /api/v1/backup/create
POST   /api/v1/backup/restore/:id      (?mode=merge|replace)
GET    /api/v1/backup/download/:id
DELETE /api/v1/backup/:id
GET    /api/v1/backup/settings
PUT    /api/v1/backup/settings
GET    /api/v1/backup/runs             (history, including manual and catch-up runs)
GET    /api/v1/backup/status           (last success, last attempt, next scheduled run)

GET    /api/v1/admin/users             (administrators only)
POST   /api/v1/admin/users
PUT    /api/v1/admin/users/:id/role
DELETE /api/v1/admin/users/:id

POST   /api/v1/app-passwords
GET    /api/v1/app-passwords
DELETE /api/v1/app-passwords/:id

GET    /api/v1/setup/ios-profile

CardDAV principal:    /dav/{email}/
CardDAV address book: /dav/{email}/addressbooks/contacts/
.well-known/carddav → /dav/ (RFC 6764)
```

The CardDAV server supports the CalendarServer CTag extension and RFC 6578
`sync-collection`, so clients fetch only what changed rather than the whole address book
on every poll.

`GET /health` returns `503` with `"status":"degraded"` when the database is unreachable,
which is what the container's `HEALTHCHECK` and a monitoring probe should watch. It also
reports the build version, the applied `schema_version` (`025_backup_runs`) — the first
question after an upgrade — and `queue_depth`, so a backlog is distinguishable from an idle
system. A deep queue is reported, never fatal: answering `503` for it would make the health
check restart the process, and a restart is exactly what loses the queued jobs.

`/health` is public and deliberately carries nothing per-user: backup health lives behind
authentication at `GET /backup/status`.

## Development

```bash
make build    # build frontend + binary (frontend is embedded via go:embed)
make run      # build and run
make test     # run the Go test suite
make lint     # golangci-lint
make clean    # remove build artifacts
```

### Testing

The Go suite runs against in-memory SQLite and covers the repository, service, sync
engine, CardDAV server, HTTP handlers, and worker. Run it with the race detector the way
CI does:

```bash
go test ./... -race
```

PostgreSQL-specific tests (migrations, the change journal, cursor upserts) are skipped
unless a database is pointed at them — CI runs them against a real PostgreSQL:

```bash
TEST_POSTGRES_DSN='postgres://user:pass@localhost:5432/chqtest?sslmode=disable' \
  go test ./internal/repository/ -run TestPostgres
```

Note the `-run TestPostgres` filter: only tests in package `repository` whose name starts
with `TestPostgres` ever execute against PostgreSQL. A test that needs PostgreSQL coverage
has to be named and placed accordingly, or it silently runs on SQLite alone.

### Schema changes are forward-only

`.down.sql` files exist and are embedded, but **nothing applies them** — the migration runner
reads `*.up.sql` only. There is no rollback command; reverting a schema change means restoring
a database dump, so take one before upgrading a deployment you care about.

New migrations must therefore stick to additive statements: `CREATE TABLE`, `CREATE INDEX`,
and `ADD COLUMN` with a DEFAULT. Dropping or renaming a column, adding `NOT NULL` without a
default, or using `CREATE INDEX CONCURRENTLY` (each migration file runs inside a transaction)
will strand installations with no way back.

The frontend has its own gates:

```bash
cd web
npm run lint          # ESLint
npm run format:check  # Prettier
npm run test          # Vitest
```

CI (`.github/workflows/ci.yml`) runs all of the above — Go tests with `-race`,
`golangci-lint`, the PostgreSQL suite, the frontend gates, and a Docker image build that
must come up healthy — on every push and pull request.

## License

[Elastic License 2.0](LICENSE) — free to use, modify, and self-host (including commercial internal use); selling access to the service as a SaaS product is not permitted.

© 2026 Stanislav Gumeniuk <i@gumeniuk.com>
