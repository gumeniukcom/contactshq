CREATE TABLE IF NOT EXISTS sync_conflicts (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type    TEXT NOT NULL,
    remote_id        TEXT NOT NULL,
    local_contact_id TEXT NOT NULL DEFAULT '',
    base_vcard       TEXT NOT NULL DEFAULT '',
    local_vcard      TEXT NOT NULL DEFAULT '',
    remote_vcard     TEXT NOT NULL DEFAULT '',
    field_diffs      TEXT NOT NULL DEFAULT '[]',
    status           TEXT NOT NULL DEFAULT 'pending',
    resolution       TEXT NOT NULL DEFAULT '',
    resolved_vcard   TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sync_conflicts_user ON sync_conflicts(user_id, status);
