ALTER TABLE assets
  ADD COLUMN IF NOT EXISTS source_url text;

INSERT INTO schema_migrations(version)
VALUES ('0004_asset_source_url')
ON CONFLICT (version) DO NOTHING;
