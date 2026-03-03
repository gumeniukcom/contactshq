CREATE TABLE IF NOT EXISTS potential_duplicates (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_a_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    contact_b_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    score        REAL NOT NULL,
    match_reasons TEXT NOT NULL DEFAULT '[]',
    status       TEXT NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_potential_dup_user ON potential_duplicates(user_id, status);
