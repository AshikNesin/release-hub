-- Play publish status per release: the hub records a release even when the
-- Play push fails, and the failure almost always needs a MANUAL action in
-- Play Console (send for review, grant permission, enable API…). Persist
-- the outcome so the UI can show it until superseded by a newer release.
--   pending: Play publishing not applicable/not attempted (direct, .apk)
--   ok:      landed on the Play track (play_release carries its name)
--   error:   push failed; play_error is the actionable reason + remedy
ALTER TABLE releases ADD COLUMN play_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE releases ADD COLUMN play_error TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN play_release TEXT NOT NULL DEFAULT '';

-- No backfill guess: historical Play outcomes are unknown, so pre-existing
-- rows stay 'pending'. The app page reconciles lazily against the live
-- Play track state when Play is enabled (see settlePlayStatus).

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (009, '009-play-status');
