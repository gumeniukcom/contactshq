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
internal/carddav/            - CardDAV server (go-webdav backend)
internal/sync/               - Sync engine, providers (internal, CardDAV, Google)
internal/worker/             - Task queue (goroutine worker)
migrations/                  - SQL migrations
configs/                     - YAML config files
```

## Config

Config via `configs/config.yaml` or env vars with `CHQ_` prefix:
- `CHQ_DATABASE_DRIVER` = sqlite | postgres
- `CHQ_DATABASE_DSN` = connection string
- `CHQ_AUTH_JWT_SECRET` = JWT signing secret
- `CHQ_SERVER_PORT` = HTTP port (default 8080)

## API

All endpoints under `/api/v1/`:
- `POST /auth/register`, `/auth/login`, `/auth/refresh`
- `GET/PUT/DELETE /users/me`
- `GET/POST /contacts`, `GET/PUT/DELETE /contacts/:id`
- `POST /import/vcard`, `/import/csv`
- `GET /export/vcard`, `/export/csv`, `/export/json`
- `GET/POST/PUT/DELETE /pipelines`
- `POST /backup/create`, `GET /backup/list`
- CardDAV: `/dav/{email}/contacts/`

## Docker

```bash
docker compose up -d   # Runs with PostgreSQL
```
