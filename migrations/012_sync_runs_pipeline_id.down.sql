DROP INDEX IF EXISTS idx_sync_runs_pipeline;
ALTER TABLE sync_runs DROP COLUMN pipeline_id;
