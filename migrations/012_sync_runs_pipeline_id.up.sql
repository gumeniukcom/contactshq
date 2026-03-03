ALTER TABLE sync_runs ADD COLUMN pipeline_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sync_runs_pipeline ON sync_runs(pipeline_id);
