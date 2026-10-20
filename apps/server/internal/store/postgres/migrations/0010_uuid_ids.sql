DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM content_versions LIMIT 1)
    OR EXISTS (SELECT 1 FROM assets LIMIT 1)
    OR EXISTS (SELECT 1 FROM publish_jobs LIMIT 1)
    OR EXISTS (SELECT 1 FROM publish_pointer LIMIT 1)
    OR EXISTS (SELECT 1 FROM publisher_builds LIMIT 1)
    OR EXISTS (SELECT 1 FROM publish_snapshots LIMIT 1) THEN
    RAISE EXCEPTION '0010_uuid_ids requires empty content, asset, publish, and publisher tables; no records were removed';
  END IF;
END $$;

ALTER TABLE publish_snapshots DROP CONSTRAINT IF EXISTS publish_snapshots_release_id_check;
ALTER TABLE publish_snapshots ADD CONSTRAINT publish_snapshots_release_id_check
CHECK (release_id ~ '^rel_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$');

ALTER TABLE audit_log ALTER COLUMN id DROP DEFAULT;
ALTER SEQUENCE IF EXISTS audit_log_id_seq OWNED BY NONE;
DROP SEQUENCE IF EXISTS audit_log_id_seq;
ALTER TABLE audit_log ALTER COLUMN id TYPE uuid USING gen_random_uuid();
ALTER TABLE audit_log ALTER COLUMN id SET DEFAULT gen_random_uuid();

INSERT INTO schema_migrations(version) VALUES ('0010_uuid_ids')
ON CONFLICT (version) DO NOTHING;
