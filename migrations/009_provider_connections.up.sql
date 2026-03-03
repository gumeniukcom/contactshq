CREATE TABLE IF NOT EXISTS provider_connections (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    endpoint      TEXT NOT NULL DEFAULT '',
    username      TEXT NOT NULL DEFAULT '',
    password      TEXT NOT NULL DEFAULT '',
    connected     INTEGER NOT NULL DEFAULT 1,
    last_sync_at  TIMESTAMP,
    last_error    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_provider_connections_user ON provider_connections(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_connections_user_type ON provider_connections(user_id, provider_type);
