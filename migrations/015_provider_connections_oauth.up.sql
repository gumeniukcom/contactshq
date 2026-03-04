ALTER TABLE provider_connections ADD COLUMN access_token TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_connections ADD COLUMN refresh_token TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_connections ADD COLUMN token_expiry TIMESTAMP;
ALTER TABLE provider_connections ADD COLUMN client_id TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_connections ADD COLUMN client_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE provider_connections ADD COLUMN scopes TEXT NOT NULL DEFAULT '';
