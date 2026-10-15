CREATE TABLE IF NOT EXISTS publisher_builds (
  id TEXT PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  request JSONB NOT NULL,
  target JSONB NOT NULL,
  phase TEXT NOT NULL CHECK (phase IN ('queued', 'building', 'submitting', 'deployed', 'succeeded', 'failed')),
  deployment_id TEXT UNIQUE,
  preview_url TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  reconciliation JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((phase IN ('deployed', 'succeeded') AND deployment_id IS NOT NULL) OR phase NOT IN ('deployed', 'succeeded'))
);

-- Keep an uncertain cloud write active until an operator resolves its outcome.
CREATE UNIQUE INDEX IF NOT EXISTS publisher_builds_one_active
  ON publisher_builds ((true)) WHERE phase IN ('queued', 'building', 'submitting', 'deployed');

INSERT INTO schema_migrations(version) VALUES ('0008_publisher_builds')
ON CONFLICT (version) DO NOTHING;
