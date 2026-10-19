CREATE TABLE IF NOT EXISTS publish_snapshots (
  snapshot_key TEXT PRIMARY KEY,
  release_id TEXT NOT NULL,
  checksum TEXT NOT NULL CHECK (checksum ~ '^sha256:[a-f0-9]{64}$'),
  content BYTEA NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 8388608),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (release_id ~ '^rel_[a-z0-9_]+$'),
  CHECK (snapshot_key = 'snapshots/' || release_id || '/' || checksum || '.json')
);

INSERT INTO schema_migrations(version) VALUES ('0009_publish_snapshots')
ON CONFLICT (version) DO NOTHING;
