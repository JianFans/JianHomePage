ALTER TABLE content_versions ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE content_versions ADD COLUMN IF NOT EXISTS source_version_id TEXT REFERENCES content_versions(id);

CREATE INDEX IF NOT EXISTS content_versions_created_id_idx
  ON content_versions (created_at DESC, id DESC);

INSERT INTO schema_migrations(version) VALUES ('0007_version_library')
ON CONFLICT (version) DO NOTHING;
