-- name: CreatePlayAccount :one
INSERT INTO play_accounts (label, credentials) VALUES (?, ?) RETURNING *;

-- name: DeletePlayAccount :exec
DELETE FROM play_accounts WHERE id = ?;

-- name: ListPlayAccounts :many
SELECT * FROM play_accounts ORDER BY id;

-- name: PlayAccountByID :one
SELECT * FROM play_accounts WHERE id = ?;

-- name: SetPlayAccountForPlatform :exec
UPDATE app_platforms SET play_enabled = ?, play_account_id = ? WHERE id = ?;

-- name: ListPlatformsWithPlay :many
SELECT ap.* FROM app_platforms ap WHERE ap.play_enabled = 1;
