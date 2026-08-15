-- Rename channel 'api-share' → 'direct' (clearer; legacy name still accepted
-- on input). Idempotent.
UPDATE releases SET channel = 'direct' WHERE channel = 'api-share';

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (005, '005-channel-rename');
