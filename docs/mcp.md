# MCP server

release-hub has a built-in [Model Context Protocol](https://modelcontextprotocol.io)
server, so an AI client such as Claude Code, Claude Desktop or Cursor can do
what the web app does: register an app, add its Android or iOS platform, get
the direct download link, check releases, wire Google Play, create tokens and
set up keyless publishing.

- **Endpoint:** `https://<your-release-hub>/mcp` (Streamable HTTP, stateless)
- **Auth:** the same `Authorization: Bearer rh_...` API token CI uses. No OAuth.

## Connect

Create a token in **Settings → API tokens** (name it after the client, e.g.
`claude-code`, so you can revoke it on its own later).

**Claude Code**

```sh
claude mcp add --transport http release-hub https://release-hub.example.com/mcp \
  --header "Authorization: Bearer $RELEASE_HUB_TOKEN"
```

Add `--scope user` to have it in every project. `claude mcp list` should show
`release-hub … ✓ Connected`.

**Clients with a JSON config that speak HTTP** (Cursor, Windsurf, …):

```json
{
  "mcpServers": {
    "release-hub": {
      "type": "http",
      "url": "https://release-hub.example.com/mcp",
      "headers": { "Authorization": "Bearer rh_..." }
    }
  }
}
```

Claude Desktop only speaks stdio to local servers, so bridge it:

```json
{
  "mcpServers": {
    "release-hub": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://release-hub.example.com/mcp",
               "--header", "Authorization: Bearer rh_..."]
    }
  }
}
```

Then ask things like *"create an app called tiny-notes with the android
package com.example.tinynotes"* or *"what's the direct download link for
tiny-firewall?"*.

## Tools

| Area | Tools |
|---|---|
| Apps | `list_apps`, `get_app`, `create_app`, `delete_app`, `add_platform`, `set_retention` |
| Releases and links | `list_releases`, `get_download_links`, `delete_release` |
| Google Play | `set_play_publishing`, `check_play_setup`, `list_play_tracks`, `get_testers`, `invite_testers`, `list_play_accounts`, `add_play_account`, `delete_play_account` |
| Keyless publishing | `list_keyless_grants`, `create_keyless_grant`, `delete_keyless_grant`, `github_status`, `list_github_repositories`, `connect_github_repository` |
| Settings | `get_settings`, `update_settings`, `list_tokens`, `create_token`, `delete_token`, `vault_status`, `vault_migrate` |

`platform` defaults to `android` wherever it is optional. Destructive tools
(`delete_*`) are marked with MCP's `destructiveHint`, so clients ask before
running them.

`get_download_links` returns, per platform, the stable
`/apps/<slug>/download?channel=…` links (they always resolve to the newest
build), the in-app update manifest URL, and the newest direct build's artifact
URL with its SHA-256.

What MCP doesn't do:

- **Uploading builds.** Artifacts are tens of megabytes and come from CI:
  keep using `POST /api/apps/{slug}/{platform}/releases` (see
  [API reference](api.md)).
- **Browser flows.** Creating and installing the GitHub App redirects through
  github.com, so do that once in **Settings → GitHub**. After that,
  `connect_github_repository` works from MCP.
- **Reading signing keys.** Only the fingerprint is exposed; CI still fetches
  the keystore through the bearer API.

## How it works

`srv/mcp.go` implements the stateless form of MCP's Streamable HTTP transport:
each JSON-RPC request is one `POST /mcp` answered with one JSON response, so
there are no sessions and `GET /mcp` returns 405. The route sits behind the
same `requireAPI` middleware as the rest of the bearer API.

Each tool runs the existing handler in-process, the same one the bearer API
or the UI's `/api/v1` uses, so validation, side effects and error messages are
identical everywhere. A handler's error becomes a tool result with
`isError: true` carrying its message.

Tokens minted by keyless publishing are scoped to one app's publish endpoints
and are refused on `/mcp` (403): only tokens you create in Settings can use it.
Anyone holding such a token can do anything the web app can, so treat it like
the admin password and revoke it in **Settings → API tokens** when a client no
longer needs it.
