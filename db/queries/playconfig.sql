-- name: SetPlayConfig :exec
UPDATE apps SET play_enabled = ?, play_credentials = ? WHERE id = ?;

-- name: ListAppsWithPlay :many
SELECT * FROM apps WHERE play_enabled = 1;
