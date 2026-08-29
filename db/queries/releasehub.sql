-- apps (product identity: one slug, many platforms)

-- name: CreateApp :execresult
INSERT INTO apps (slug) VALUES (?);

-- name: ListApps :many
SELECT * FROM apps ORDER BY slug;

-- name: AppBySlug :one
SELECT * FROM apps WHERE slug = ?;

-- name: DeleteApp :exec
DELETE FROM apps WHERE slug = ?;

-- app platforms

-- name: CreateAppPlatform :execresult
INSERT INTO app_platforms (app_id, platform, package_name) VALUES (?, ?, ?);

-- name: ListPlatformsByApp :many
SELECT * FROM app_platforms WHERE app_id = ? ORDER BY platform;

-- name: AppPlatformByAppAndPlatform :one
SELECT * FROM app_platforms WHERE app_id = ? AND platform = ?;

-- name: CountPlatformsBySlug :one
SELECT COUNT(*) AS n FROM app_platforms ap JOIN apps a ON a.id = ap.app_id WHERE a.slug = ?;

-- name: PlatformBySlugAndPlatform :one
SELECT ap.* FROM app_platforms ap JOIN apps a ON a.id = ap.app_id
WHERE a.slug = ? AND ap.platform = ?;

-- name: DeleteAppPlatform :exec
DELETE FROM app_platforms WHERE id = ?;

-- releases (per platform)

-- name: CreateRelease :execresult
INSERT INTO releases (app_platform_id, version_code, version_name, channel, notes, sha256, size_bytes, file_name, play_status, play_error, play_release)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: SetPlayStatus :exec
UPDATE releases SET play_status = ?, play_error = ?, play_release = ?
WHERE app_platform_id = ? AND version_code = ?;

-- name: ListReleases :many
SELECT * FROM releases WHERE app_platform_id = ? ORDER BY version_code DESC;

-- name: LatestReleaseForChannel :many
SELECT * FROM releases
WHERE app_platform_id = ? AND channel = ?
ORDER BY version_code DESC LIMIT 1;

-- name: ReleaseByAppAndCode :one
SELECT * FROM releases WHERE app_platform_id = ? AND version_code = ?;

-- name: MaxVersionCode :one
SELECT COALESCE(MAX(version_code), 0) AS max_code FROM releases WHERE app_platform_id = ?;

-- name: DeleteRelease :exec
DELETE FROM releases WHERE app_platform_id = ? AND version_code = ?;

-- api tokens

-- name: CreateApiToken :execresult
INSERT INTO api_tokens (name, token_hash) VALUES (?, ?);

-- name: ListApiTokens :many
SELECT * FROM api_tokens ORDER BY created_at;

-- name: ApiTokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = ?;

-- name: TouchApiToken :exec
UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?;

-- name: DeleteApiToken :exec
DELETE FROM api_tokens WHERE id = ?;

-- sessions

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, created_at, expires_at) VALUES (?, ?, ?);

-- name: SessionByHash :one
SELECT * FROM sessions WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < ?;

-- config

-- name: GetConfig :one
SELECT value FROM config WHERE key = ?;

-- name: SetConfig :exec
INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- release retention (direct channel): prune + per-platform override

-- name: ListReleasesByChannel :many
SELECT * FROM releases WHERE app_platform_id = ? AND channel = ? ORDER BY version_code DESC;

-- name: SetPruneKeep :exec
UPDATE app_platforms SET prune_keep = ? WHERE id = ?;

-- name: PlatformsForPrune :many
SELECT ap.* FROM app_platforms ap;
