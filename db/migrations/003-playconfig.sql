-- Per-app Google Play publishing config, stored in the DB.
ALTER TABLE apps ADD COLUMN play_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE apps ADD COLUMN play_credentials TEXT NOT NULL DEFAULT ''; -- encrypted service-account JSON

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-playconfig');
