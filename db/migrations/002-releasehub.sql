-- release-hub schema: apps, releases, api tokens, sessions

CREATE TABLE IF NOT EXISTS apps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,              -- short name used in URLs, e.g. "tinyfirewall"
    package_name TEXT NOT NULL,             -- e.g. io.nesin.tinyfirewall (android) or bundle id
    platform TEXT NOT NULL DEFAULT 'android', -- android | ios
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS releases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version_code INTEGER NOT NULL,          -- monotonically increasing per app
    version_name TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT 'internal', -- public | internal | api-share
    notes TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    file_name TEXT NOT NULL,                -- artifact filename on disk
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(app_id, version_code)
);

-- Tokens for programmatic (curl/CI) access: hashed at rest, shown once.
CREATE TABLE IF NOT EXISTS api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,        -- sha256(token) hex
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

-- Browser sessions for the UI (self-hosted; no exe.dev dependency).
CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,            -- sha256(cookie value) hex
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

-- First-run bootstrap: the admin user. Single-user to start.
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
