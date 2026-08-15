-- name: SetPlayConfig :exec
UPDATE app_platforms SET play_enabled = ?, play_credentials = ? WHERE id = ?;

-- name: ListPlatformsWithPlay :many
SELECT ap.* FROM app_platforms ap WHERE ap.play_enabled = 1;
