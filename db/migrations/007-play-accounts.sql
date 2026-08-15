-- Hub-wide Google Play service accounts: one credential shared by every
-- app; per-platform rows keep only an enabled flag (+ optional account
-- link for future multi-account use).
CREATE TABLE IF NOT EXISTS play_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    label TEXT NOT NULL DEFAULT '',             -- display label; falls back to the service-account email
    credentials TEXT NOT NULL,                  -- encrypted service-account JSON
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE app_platforms ADD COLUMN play_account_id INTEGER REFERENCES play_accounts(id) ON DELETE SET NULL;

-- Migrate any legacy per-platform credentials into shared account rows
-- (one row per distinct blob), linking platforms to them.
INSERT INTO play_accounts (credentials)
SELECT DISTINCT play_credentials FROM app_platforms WHERE play_credentials != '';

UPDATE app_platforms
SET play_account_id = (SELECT pa.id FROM play_accounts pa WHERE pa.credentials = app_platforms.play_credentials)
WHERE play_credentials != '';

ALTER TABLE app_platforms DROP COLUMN play_credentials;
