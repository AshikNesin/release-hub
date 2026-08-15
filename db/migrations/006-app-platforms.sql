-- App -> platform restructure: an app is a product (slug identity); each
-- platform variant (android, ios) carries its own package name, signing
-- key, and Play credentials. Releases hang off the platform row, since
-- version codes / artifacts are per-platform.
--
-- No backward compatibility kept (small install base): artifact paths move
-- from <slug>/... to <slug>/<platform>/...

CREATE TABLE apps_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,              -- product identity, e.g. "tinyfirewall"
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO apps_new (id, slug, created_at) SELECT id, slug, created_at FROM apps;

CREATE TABLE app_platforms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps_new(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,                 -- android | ios
    package_name TEXT NOT NULL,             -- application id / bundle id
    play_enabled INTEGER NOT NULL DEFAULT 0,
    play_credentials TEXT NOT NULL DEFAULT '',  -- encrypted service-account JSON
    sign_keystore TEXT NOT NULL DEFAULT '',     -- encrypted keystore bytes
    sign_config TEXT NOT NULL DEFAULT '',       -- encrypted {storePassword,keyAlias,keyPassword}
    sign_sha256 TEXT NOT NULL DEFAULT '',       -- plaintext sha256 of keystore
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(app_id, platform)
);

INSERT INTO app_platforms (app_id, platform, package_name, play_enabled,
                           play_credentials, sign_keystore, sign_config, sign_sha256, created_at)
SELECT id, platform, package_name, play_enabled, play_credentials,
       sign_keystore, sign_config, sign_sha256, created_at
FROM apps;

ALTER TABLE releases RENAME TO old_releases;
CREATE TABLE releases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_platform_id INTEGER NOT NULL REFERENCES app_platforms(id) ON DELETE CASCADE,
    version_code INTEGER NOT NULL,          -- monotonically increasing per platform
    version_name TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT 'internal',
    notes TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    file_name TEXT NOT NULL,                -- artifact filename on disk (<slug>/<platform>/...)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(app_platform_id, version_code)
);

INSERT INTO releases (app_platform_id, version_code, version_name, channel, notes,
                      sha256, size_bytes, file_name, created_at)
SELECT ap.id, r.version_code, r.version_name, r.channel, r.notes,
       r.sha256, r.size_bytes, r.file_name, r.created_at
FROM old_releases r
JOIN apps a ON a.id = r.app_id
JOIN app_platforms ap ON ap.app_id = a.id AND ap.platform = a.platform;

DROP TABLE old_releases;
DROP TABLE apps;
ALTER TABLE apps_new RENAME TO apps;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (006, '006-app-platforms');
