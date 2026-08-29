CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS content_versions (
  id text PRIMARY KEY,
  status text NOT NULL CHECK (status IN ('draft','in_review','published','archived')),
  revision bigint NOT NULL CHECK (revision > 0),
  snapshot jsonb NOT NULL,
  checksum text NOT NULL,
  review_approved boolean NOT NULL DEFAULT false,
  created_by text NOT NULL,
  updated_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS assets (
  id text PRIMARY KEY,
  blob_key text NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('pending','ready','deleted')),
  metadata jsonb NOT NULL,
  rights jsonb NOT NULL,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  deleted_at timestamptz
);

CREATE TABLE IF NOT EXISTS publish_jobs (
  id text PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE,
  version_id text NOT NULL REFERENCES content_versions(id),
  snapshot_key text NOT NULL,
  snapshot_checksum text NOT NULL,
  build_id text,
  status text NOT NULL CHECK (status IN ('pending','building','succeeded','failed')),
  error_message text,
  requested_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS publish_pointer (
  slot text PRIMARY KEY,
  version_id text NOT NULL REFERENCES content_versions(id),
  snapshot_key text NOT NULL,
  snapshot_checksum text NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
  id bigserial PRIMARY KEY,
  actor_sub text NOT NULL,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  metadata jsonb NOT NULL,
  created_at timestamptz NOT NULL
);

INSERT INTO schema_migrations(version)
VALUES ('0001_initial')
ON CONFLICT (version) DO NOTHING;
