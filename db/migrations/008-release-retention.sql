-- Release retention: per-channel delete support + auto-prune settings.
--
-- releases.channel is the source of truth for the unique constraint that
-- the upload API enforces anyway (no re-uploading a versionCode); making
-- it a real UNIQUE index turns accidental duplicates into a clean 409 at
-- the DB layer instead of silent shadowing.
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_platform_code
    ON releases(app_platform_id, version_code);

-- Prune override for a platform: how many direct-channel releases to keep
-- when auto-pruning runs after an upload. NULL (default) = inherit the
-- hub-wide Settings value; 0 = keep everything (never prune).
ALTER TABLE app_platforms ADD COLUMN prune_keep INTEGER;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (008, '008-release-retention');
