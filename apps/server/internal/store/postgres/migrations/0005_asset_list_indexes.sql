CREATE INDEX IF NOT EXISTS assets_status_created_id_idx
  ON assets (status, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS assets_active_created_id_idx
  ON assets (created_at DESC, id DESC)
  WHERE status IN ('pending', 'ready');

INSERT INTO schema_migrations(version)
VALUES ('0005_asset_list_indexes')
ON CONFLICT (version) DO NOTHING;
