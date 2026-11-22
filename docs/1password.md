# 1Password vault

Signing keystores and Play service-account keys can live in a **1Password
vault** instead of sealed in the database, the same way tiny-agents does it:
through **1Password Connect**, which the release-hub image runs itself, so
there is no second service to deploy.

**Set up**

1. In 1Password, create a vault for release-hub (e.g. `release-hub`).
2. On 1Password.com → Developer → **Connect servers**, create a server with
   access to **only** that vault and download `1password-credentials.json`.
3. Issue an access token for that server with **read + write** on that vault
   only (set an expiry if you like; Settings warns 14 days before it ends).
4. Set on the container and redeploy:

   | Variable | Notes |
   |---|---|
   | `OP_CONNECT_CREDENTIALS` | The contents of `1password-credentials.json`, as JSON or base64. Makes release-hub run Connect inside its own container. |
   | `OP_CONNECT_TOKEN` | The access token. |
   | `OP_CONNECT_VAULT_ID` | Optional. Without it the vault is the one the token sees. |
   | `OP_CONNECT_HOST` | Instead of the credentials: a Connect server you run elsewhere (e.g. 1Password's two-container compose on another host). |

5. Open **Settings → 1Password**: it says **Ready · <vault>**. Every new key
   (generated or uploaded keystores, Play service accounts) now goes to the vault.

**Moving existing keys.** Settings → 1Password shows **Move N keys to
1Password** while keys are still sealed in the database. Each key is copied
into a 1Password item, read back and compared, then the row points at the
copy (in one guarded update; a key that changed meanwhile, or doesn't match
its recorded sha256, stays where it was and the toast says why). Run it
again any time; it moves only what is left. Scripted:

```bash
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" https://release-hub.example.com/api/vault
curl -X POST -H "Authorization: Bearer $RELEASE_HUB_TOKEN" https://release-hub.example.com/api/vault/migrate
# → {"moved":["tinyfirewall/android signing key","Play service account …"],"failed":[]}
```

`RELEASE_HUB_SECRET_KEY` must still be set for the move (it opens the sealed
keys); once nothing is left in the database it is no longer needed.

**Items.** One item per signing key (`release-hub: <slug> (<platform>) signing key`,
fields `keystore_base64`, `signing_config`, plus readable `key_alias`,
`store_password`, `key_password`, `keystore_sha256`) and one per Play
service account (`service_account_json`), all tagged `release-hub`. The
database keeps references (`op:<vault>/<item>#<field>`). CI and the API see
no difference: `GET /api/apps/{slug}/signing` returns the same bytes and headers.
Release-hub **never deletes** a 1Password item: when a key is replaced or
removed (or its app deleted), the item is tagged `release-hub/retired` and kept,
since a lost signing key can't be recovered.

**Behaviour**

- *Scoping guard*: the token must see exactly the pinned vault; a token that
  sees others is refused (re-checked at least every 6 hours).
- *Availability*: values read are kept in memory for 5 minutes, and when
  Connect is down (e.g. still starting after a deploy) the last value read is
  served. With none yet, a read retries for about 8 seconds, then fails.
- *Bundled Connect*: the Dockerfile copies `connect-api` and `connect-sync`
  from 1Password's images at a pinned version and digest (upgrade = bump the
  tag and both digests). With `OP_CONNECT_CREDENTIALS` and no
  `OP_CONNECT_HOST`, release-hub starts both as its own user, with Connect's
  state in `/data/op-connect` and the credentials passed only through
  `OP_SESSION` in their environment. They listen on 11280–11283 inside the
  container (not published); every request needs the token. A process that
  exits is restarted with backoff. Linux amd64/arm64.
- Not a layout 1Password officially supports (it ships images, not
  binaries), and it puts the credentials file and the token on one host. To
  keep them apart, run Connect elsewhere and set `OP_CONNECT_HOST`.
