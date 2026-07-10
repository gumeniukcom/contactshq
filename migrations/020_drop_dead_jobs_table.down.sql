CREATE TABLE IF NOT EXISTS jobs (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    payload    TEXT DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending',
    error      TEXT DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
