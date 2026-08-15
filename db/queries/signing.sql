-- name: SetSigningConfig :exec
UPDATE apps SET sign_keystore = ?, sign_config = ?, sign_sha256 = ? WHERE id = ?;
