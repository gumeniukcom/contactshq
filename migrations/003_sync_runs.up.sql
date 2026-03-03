CREATE TABLE IF NOT EXISTS sync_runs (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'running',
    created_count INTEGER NOT NULL DEFAULT 0,
    updated_count INTEGER NOT NULL DEFAULT 0,
    deleted_count INTEGER NOT NULL DEFAULT 0,
    error_count   INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at   TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sync_runs_user ON sync_runs(user_id, started_at DESC);
