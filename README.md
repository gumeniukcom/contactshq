# ContactsHQ

A self-hosted contact management hub with a CardDAV server, multi-provider sync engine, and a modern web UI. Designed to be the single source of truth for all your contacts — no matter where they originally live.

## What it does

- **Centralized address book** — store and manage all contacts in one place with full vCard 4.0 support (names, emails, phones, addresses, IMs, URLs, categories, dates, and more)
- **CardDAV server** — expose your contacts as a standard CardDAV endpoint, compatible with macOS Contacts, iOS, Thunderbird, and any CalDAV/CardDAV client
- **Sync pipelines** — pull contacts from external CardDAV servers (Fastmail, iCloud, Nextcloud, etc.) on a schedule or on demand; supports pull, push, and bidirectional modes
- **Three-way merge** — when a contact is modified both locally and on a remote source, the engine merges changes field-by-field automatically; unresolvable conflicts are queued for manual review
- **Conflict resolution UI** — inspect field-level diffs between base/local/remote versions and resolve each field individually
- **Duplicate detection** — score-based detection (email, phone, name similarity) surfaces potential duplicates for review and merging
- **Contact merge** — merge two contacts with field-by-field resolution; sync state is transferred to the winner automatically
- **Import / Export** — import vCard (.vcf) and CSV files; export to vCard, CSV, or JSON
- **Backup & restore** — scheduled or on-demand backups with optional gzip compression, configurable retention, and merge/replace restore modes
- **QR codes** — generate a QR code for any contact (vCard payload, scannable by phones)
- **Multi-user** — each user has an isolated address book; admin role for user management
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
| Migrations | Custom sequential SQL runner |

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
docker compose up -d
```

The app will be available at `http://localhost:8080`. The first registered user becomes an admin.

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
  jwt_secret: change-me-in-production
  jwt_expiry: 15m
  refresh_expiry: 7d
```

| Env variable | Description |
|---|---|
| `CHQ_DATABASE_DRIVER` | `sqlite` or `postgres` |
| `CHQ_DATABASE_DSN` | SQLite file path or PostgreSQL connection string |
| `CHQ_AUTH_JWT_SECRET` | JWT signing secret — **change in production** |
| `CHQ_SERVER_PORT` | HTTP port (default `8080`) |

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
DELETE /api/v1/contacts          (delete all)
GET    /api/v1/contacts/:id/vcard
GET    /api/v1/contacts/:id/qrcode

POST   /api/v1/import/vcard
POST   /api/v1/import/csv
GET    /api/v1/export/vcard
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

CardDAV: /dav/{email}/contacts/
.well-known/carddav → /dav/ (RFC 6764)
```

## Development

```bash
make build    # build binary (includes frontend embed)
make run      # build and run
make test     # run all tests
make clean    # remove build artifacts
```

Tests use in-memory SQLite and cover the repository, service, sync engine, and worker layers.

## License

[Elastic License 2.0](LICENSE) — free to use, modify, and self-host (including commercial internal use); selling access to the service as a SaaS product is not permitted.

© 2026 Stanislav Gumeniuk <i@gumeniuk.com>
