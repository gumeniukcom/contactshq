-- One incremental-sync cursor per pipeline: the opaque token a provider hands back so the
-- next run fetches only what changed. Keyed by (user, provider_type) to match the key the
-- sync engine already uses everywhere else.
CREATE TABLE IF NOT EXISTS sync_cursors (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type TEXT NOT NULL,
    cursor        TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_cursors_user_provider ON sync_cursors(user_id, provider_type);
