CREATE TABLE IF NOT EXISTS admin_sessions (
  token_hash TEXT PRIMARY KEY,
  principal JSONB NOT NULL,
  csrf_token TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS admin_sessions_expiry_idx ON admin_sessions (expires_at);

CREATE TABLE IF NOT EXISTS admin_login_flows (
  state_hash TEXT PRIMARY KEY,
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS admin_login_flows_expiry_idx ON admin_login_flows (expires_at);

INSERT INTO schema_migrations(version) VALUES ('0006_admin_sessions')
ON CONFLICT (version) DO NOTHING;
