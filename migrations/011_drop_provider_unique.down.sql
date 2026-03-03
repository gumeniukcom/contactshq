-- Restore unique constraint (only valid if there is at most one row per user+type).
DROP INDEX IF EXISTS idx_provider_connections_user;
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_connections_user_type ON provider_connections(user_id, provider_type);
