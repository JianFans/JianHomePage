DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM content_versions LIMIT 1)
    OR EXISTS (SELECT 1 FROM assets LIMIT 1)
    OR EXISTS (SELECT 1 FROM publish_jobs LIMIT 1)
    OR EXISTS (SELECT 1 FROM publish_pointer LIMIT 1)
    OR EXISTS (SELECT 1 FROM publisher_builds LIMIT 1) THEN
    RAISE EXCEPTION '0009_uuid_ids requires empty content, asset, publish, and publisher tables; no records were removed';
  END IF;
END $$;

ALTER TABLE audit_log ALTER COLUMN id DROP DEFAULT;
ALTER SEQUENCE IF EXISTS audit_log_id_seq OWNED BY NONE;
DROP SEQUENCE IF EXISTS audit_log_id_seq;
ALTER TABLE audit_log ALTER COLUMN id TYPE uuid USING gen_random_uuid();
ALTER TABLE audit_log ALTER COLUMN id SET DEFAULT gen_random_uuid();

INSERT INTO schema_migrations(version) VALUES ('0009_uuid_ids')
ON CONFLICT (version) DO NOTHING;
