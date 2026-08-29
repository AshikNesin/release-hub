-- Repair: the first rollout of 009 backfilled Play-channel rows as
-- 'error' with empty text (guess, not truth). Real errors always carry a
-- message; empty-text errors are backfill artifacts. Reset those to
-- 'pending' so the lazy reconciliation (app page / releases API) can
-- settle them against the live Play track state.
UPDATE releases SET play_status = 'pending', play_error = ''
WHERE play_status = 'error' AND play_error = '';

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (010, '010-repair-play-status');
