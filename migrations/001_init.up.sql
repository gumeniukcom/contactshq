CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    display_name  TEXT DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'user',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS address_books (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Contacts',
    description TEXT DEFAULT '',
    sync_token  TEXT DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS contacts (
    id              TEXT PRIMARY KEY,
    address_book_id TEXT NOT NULL REFERENCES address_books(id) ON DELETE CASCADE,
    uid             TEXT NOT NULL,
    etag            TEXT NOT NULL,
    vcard_data      TEXT NOT NULL,
    first_name      TEXT DEFAULT '',
    last_name       TEXT DEFAULT '',
    email           TEXT DEFAULT '',
    phone           TEXT DEFAULT '',
    org             TEXT DEFAULT '',
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(address_book_id, uid)
);

CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(last_name, first_name);

CREATE TABLE IF NOT EXISTS sync_states (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type  TEXT NOT NULL,
    provider_uri   TEXT DEFAULT '',
    remote_id      TEXT DEFAULT '',
    local_id       TEXT DEFAULT '',
    remote_etag    TEXT DEFAULT '',
    local_etag     TEXT DEFAULT '',
    content_hash   TEXT DEFAULT '',
    last_synced_at TIMESTAMP,
    sync_token     TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sync_states_user ON sync_states(user_id, provider_type);

CREATE TABLE IF NOT EXISTS pipelines (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    schedule   TEXT DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pipeline_steps (
    id            TEXT PRIMARY KEY,
    pipeline_id   TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    step_order    INTEGER NOT NULL,
    source_type   TEXT NOT NULL,
    source_config TEXT DEFAULT '{}',
    dest_type     TEXT NOT NULL,
    dest_config   TEXT DEFAULT '{}',
    conflict_mode TEXT NOT NULL DEFAULT 'source_wins'
);

CREATE TABLE IF NOT EXISTS jobs (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    payload    TEXT DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending',
    error      TEXT DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
