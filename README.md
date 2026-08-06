# ContactsHQ

A self-hosted contact management hub with a CardDAV server, multi-provider sync engine, and a modern web UI. Designed to be the single source of truth for all your contacts — no matter where they originally live.

## What it does

- **Centralized address book** — store and manage all contacts in one place with full vCard 4.0 support (names, emails, phones, addresses, IMs, URLs, categories, dates, and more)
- **CardDAV server** — expose your contacts as a standard CardDAV endpoint, compatible with macOS Contacts, iOS, Thunderbird, and any CalDAV/CardDAV client; supports CTag and RFC 6578 collection sync, so clients fetch only what changed
- **Sync pipelines** — move contacts between external providers (Fastmail, iCloud, Nextcloud, Google) and your address book on a schedule or on demand; each step is an import, an export, or a two-way sync ([how it works](docs/sync.md))
- **Incremental sync** — providers that support it (Google via a sync token, CardDAV via RFC 6578) send only what changed since the last run; exports write conditionally so a concurrent edit becomes a conflict rather than being overwritten
- **Three-way merge** — when a contact is modified both locally and on a remote source, the engine merges changes field-by-field automatically; unresolvable conflicts are queued for manual review
- **Conflict resolution UI** — inspect field-level diffs between base/local/remote versions and resolve each field individually
- **Duplicate detection** — score-based detection (email, phone, name similarity) surfaces potential duplicates for review and merging
- **Contact merge** — merge two contacts with field-by-field resolution; sync state is transferred to the winner automatically
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
  token_ttl: 24h
  refresh_ttl: 720h
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
  (default 24h), refresh tokens for theirs (default 720h). Rotate `CHQ_AUTH_JWT_SECRET` and
  restart to sign everyone out.
- **A running server keeps its cached CardDAV verdicts for up to 5 minutes**, because the
  subcommand runs in its own process. Restart the server to drop the cache at once. A
  password changed through the web UI takes effect for CardDAV immediately.

Exit codes: `0` success, `2` usage error, `3` no such user, `4` database unreachable,
`5` database has no schema yet (start the server once so it can migrate).

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

## Connect your devices

ContactsHQ includes a built-in CardDAV server. Connect your iPhone, iPad, Mac, or Thunderbird to sync contacts automatically.

- Visit `/setup` on your instance for step-by-step instructions
- In the app, go to **Settings → Connect Devices** for one-tap iOS profile download
- Use **App Passwords** (Settings → App Passwords) instead of your main password for CardDAV clients
- HTTPS is required for mobile clients — see [reverse proxy examples](docs/reverse-proxy.md)

## API

All endpoints are under `/api/v1/`. Authentication uses Bearer JWT tokens.

```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh

GET    /api/v1/contacts
POST   /api/v1/contacts
GET    /api/v1/contacts/:id
PUT    /api/v1/contacts/:id
DELETE /api/v1/contacts/:id
POST   /api/v1/contacts/bulk-delete   (body: {"ids": [...]}, max 500)
DELETE /api/v1/contacts          (delete all)
GET    /api/v1/contacts/:id/vcard
GET    /api/v1/contacts/:id/qrcode

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
GET    /api/v1/sync/conflicts/:id
POST   /api/v1/sync/conflicts/:id/resolve
POST   /api/v1/sync/conflicts/:id/dismiss

GET    /api/v1/contacts/duplicates
POST   /api/v1/contacts/duplicates/detect
POST   /api/v1/contacts/merge

GET    /api/v1/backup/list
POST   /api/v1/backup/create
POST   /api/v1/backup/restore/:id
DELETE /api/v1/backup/:id
GET    /api/v1/backup/settings
PUT    /api/v1/backup/settings

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

`GET /health` reports the version and, when configured with a database, its
connectivity — it returns `503` with `"status":"degraded"` when the database is
unreachable, which is what the container's `HEALTHCHECK` and a monitoring probe should
watch.

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
