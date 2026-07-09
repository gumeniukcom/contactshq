-- Records the remote ETag observed when the conflict was detected. Resolving a conflict
-- needs it to advance sync_states, otherwise the next run re-detects the same divergence.
ALTER TABLE sync_conflicts ADD COLUMN remote_etag TEXT NOT NULL DEFAULT '';
