---
"release-hub-mobile": patch
---

Terminology: say "release hub" (and `release-hub` in identifiers and URLs) everywhere instead of bare
"hub". The `data/hub` package is now `data/releasehub` (`HubApi` → `ReleaseHubApi`, `HubEndpoint` →
`ReleaseHubEndpoint`, `HubJson` → `ReleaseHubJson`, `HubModels` → `ReleaseHubModels`,
`HubSettings` → `ReleaseHubSettings`), user-visible strings say "Release hub", and setup docs use
`RELEASE_HUB_TOKEN`/`RELEASE_HUB_URL` in examples. Wire format and the `hub_settings` DataStore file
name are unchanged, so existing installs and saved connections keep working.
