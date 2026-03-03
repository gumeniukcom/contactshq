-- Allow multiple credentials of the same provider type per user.
DROP INDEX IF EXISTS idx_provider_connections_user_type;
CREATE INDEX IF NOT EXISTS idx_provider_connections_user ON provider_connections(user_id, created_at);
