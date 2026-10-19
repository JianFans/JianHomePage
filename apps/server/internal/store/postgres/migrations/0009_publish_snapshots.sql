CREATE TABLE IF NOT EXISTS publish_snapshots (
  snapshot_key TEXT PRIMARY KEY,
  release_id TEXT NOT NULL,
  checksum TEXT NOT NULL CHECK (checksum ~ '^sha256:[a-f0-9]{64}$'),
  content BYTEA NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 8388608),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (release_id ~ '^rel_[a-z0-9_]+$'),
  CHECK (snapshot_key = 'snapshots/' || release_id || '/' || checksum || '.json')
);

-- Old builds still reference COS snapshots; finish them with the old binary.
-- Check only on first application so normal restarts can resume new builds.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE version = '0009_publish_snapshots'
  ) AND EXISTS (
    SELECT 1 FROM publisher_builds
    WHERE phase IN ('queued', 'building', 'submitting', 'deployed')
  ) THEN
    RAISE EXCEPTION 'cannot upgrade publish snapshot storage with active publisher builds; finish or recover them with the old binary first';
  END IF;
END
$$;

INSERT INTO schema_migrations(version) VALUES ('0009_publish_snapshots')
ON CONFLICT (version) DO NOTHING;
