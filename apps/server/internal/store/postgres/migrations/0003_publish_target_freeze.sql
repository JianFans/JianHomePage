DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'content_versions_status_check'
      AND conrelid = 'content_versions'::regclass
      AND pg_get_constraintdef(oid) LIKE '%publishing%'
  ) THEN
    ALTER TABLE content_versions
      DROP CONSTRAINT IF EXISTS content_versions_status_check;
    ALTER TABLE content_versions
      ADD CONSTRAINT content_versions_status_check
      CHECK (status IN ('draft','in_review','publishing','published','archived'));
  END IF;
END
$$;

ALTER TABLE publish_jobs
  ADD COLUMN IF NOT EXISTS target_revision bigint;

DO $$
DECLARE
  active_job_count bigint;
  unsafe_job_count bigint;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE version = '0003_publish_target_freeze'
  ) THEN
    SELECT count(*) INTO active_job_count
    FROM publish_jobs
    WHERE status IN ('pending', 'building');

    IF active_job_count > 1 THEN
      RAISE EXCEPTION 'cannot upgrade more than one active production publish job';
    END IF;

    SELECT count(*) INTO unsafe_job_count
    FROM publish_jobs AS jobs
    JOIN content_versions AS versions ON versions.id = jobs.version_id
    WHERE jobs.status IN ('pending', 'building')
      AND NOT (
        versions.checksum = jobs.snapshot_checksum
        AND (
          (jobs.operation = 'publish' AND versions.status = 'in_review' AND versions.review_approved)
          OR (jobs.operation = 'rollback' AND versions.status IN ('published', 'archived'))
        )
      );

    IF unsafe_job_count > 0 THEN
      RAISE EXCEPTION 'cannot safely upgrade active publish job; clear it before deployment';
    END IF;

    UPDATE content_versions AS versions
    SET status = 'publishing',
        revision = versions.revision + 1,
        updated_by = 'system:migration:0003',
        updated_at = now()
    FROM publish_jobs AS jobs
    WHERE jobs.version_id = versions.id
      AND jobs.operation = 'publish'
      AND jobs.status IN ('pending', 'building');

    UPDATE publish_jobs AS jobs
    SET target_revision = versions.revision
    FROM content_versions AS versions
    WHERE jobs.version_id = versions.id
      AND jobs.target_revision IS NULL;
  END IF;
END
$$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'publish_jobs'
      AND column_name = 'target_revision'
      AND is_nullable = 'YES'
  ) THEN
    ALTER TABLE publish_jobs
      ALTER COLUMN target_revision SET NOT NULL;
  END IF;
END
$$;

INSERT INTO schema_migrations(version)
VALUES ('0003_publish_target_freeze')
ON CONFLICT (version) DO NOTHING;
