-- Per-app signing keystore storage (encrypted at rest, like play creds).
ALTER TABLE apps ADD COLUMN sign_keystore TEXT NOT NULL DEFAULT '';  -- encrypted keystore bytes
ALTER TABLE apps ADD COLUMN sign_config TEXT NOT NULL DEFAULT '';   -- encrypted {storePassword,keyAlias,keyPassword}
ALTER TABLE apps ADD COLUMN sign_sha256 TEXT NOT NULL DEFAULT '';   -- plaintext sha256 of keystore (for gradle verification)

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (004, '004-signing');
