# ContactsHQ

Go API service for centralized contact management with CardDAV server, sync engine, and pipelines.

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

## API

All endpoints under `/api/v1/`:
- `POST /auth/register`, `/auth/login`, `/auth/refresh`
- `GET/PUT/DELETE /users/me`
- `GET/POST /contacts`, `GET/PUT/DELETE /contacts/:id`, `POST /contacts/bulk-delete`
- `POST /import/vcard`, `/import/csv`
- `GET /export/vcard` (optional `?ids=`), `/export/csv`, `/export/json`
- `GET/POST/PUT/DELETE /pipelines`, `POST /pipelines/:id/trigger`
- `POST /backup/create`, `GET /backup/list`, `POST /backup/restore/:id`
- `GET/POST/PUT DELETE /sync/conflicts`
- CardDAV: `/dav/{email}/addressbooks/contacts/`
- `GET /health` — reports DB connectivity (503 when unreachable)

See [docs/sync.md](docs/sync.md) for how the sync engine works.

## Conventions & gotchas

- **Migrations** are embedded and applied transactionally at startup; a binary can run
  from any working directory. Add a numbered `NNN_name.up.sql` + `.down.sql` pair.
- **First registered user is the admin.** No other code assigns the admin role.
- **CardDAV path layout** is `/dav/{email}/addressbooks/contacts/{uid}.vcf`; the segment
  depths matter to go-webdav's resource routing (principal/home-set/book/object). Build
  paths with the exported helpers in `internal/carddav`, never by hand.
- **Fiber `RequestMethods`** must include the WebDAV verbs (PROPFIND, REPORT, …) or the
  `/dav` mount is unreachable — set in `cmd/server/main.go`.
- **Pipeline steps**: source is always an external provider, dest is always `internal`;
  `direction` is `import` / `export` / `two_way` (legacy `pull`/`push`/`bidirectional`
  still parse). Provider-to-provider steps are rejected.
- **Sync**: `SyncProvider.Put` returns the id the provider assigned (Google mints its
  own). Incremental providers implement `IncrementalProvider` (delta + cursor);
  conditional writers implement `ConditionalWriter` (If-Match / ETag). The engine falls
  back to a full listing when neither is available.

## Docker

```bash
cp .env.example .env
echo "CHQ_AUTH_JWT_SECRET=$(openssl rand -hex 32)" > .env
docker compose up -d   # Runs with PostgreSQL; compose refuses to start without the secret
```
