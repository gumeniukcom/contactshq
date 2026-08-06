-- Backups have no representation in the database at all: List builds its answer from
-- os.ReadDir, the id is a filename and the date is an mtime. That leaves "did last night's
-- backup run?" unanswerable, and the obvious mtime heuristic breaks outright at retention=1,
-- where the previous file is deleted the moment a new one succeeds.
--
-- filename is a plain column rather than a reference to anything: retention deletes the file,
-- and the record of the run has to outlive it.
CREATE TABLE IF NOT EXISTS backup_runs (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    trigger       TEXT NOT NULL DEFAULT 'manual',
    status        TEXT NOT NULL DEFAULT 'running',
    filename      TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    contact_count INTEGER NOT NULL DEFAULT 0,
    compressed    BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at   TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_backup_runs_user ON backup_runs(user_id, started_at DESC);
