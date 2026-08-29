ALTER TABLE publish_jobs ADD COLUMN IF NOT EXISTS operation text;
ALTER TABLE publish_jobs ADD COLUMN IF NOT EXISTS release_id text;

UPDATE publish_jobs SET operation = 'publish' WHERE operation IS NULL;
UPDATE publish_jobs SET release_id = id WHERE release_id IS NULL;

ALTER TABLE publish_jobs ALTER COLUMN operation SET NOT NULL;
ALTER TABLE publish_jobs ALTER COLUMN release_id SET NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'publish_jobs_operation_check'
  ) THEN
    ALTER TABLE publish_jobs
      ADD CONSTRAINT publish_jobs_operation_check CHECK (operation IN ('publish','rollback'));
  END IF;
END
$$;

INSERT INTO schema_migrations(version)
VALUES ('0002_publish_job_identity')
ON CONFLICT (version) DO NOTHING;
