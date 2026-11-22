# Keyless publishing (OIDC)

CI can publish with **no stored release hub secret**. The release hub trusts a GitHub repo's
Actions identity: register a grant binding that repo to **one app** (one repo
→ one app), and the workflow exchanges its short-lived job token for a release hub
token on every run.

**Release Hub side** (App → Configuration → Keyless publishing): paste the repo's OIDC
subject — a bare repo prefix is completed to the default-branch ref automatically
(`repo:owner/name` or `repo:owner@OWNER-ID/name@REPO-ID` becomes
`…:ref:refs/heads/main`). The prefix depends on the repo's age:

- repos created before 2026-07-15: `repo:owner/name:ref:refs/heads/main`
- newer repos use **immutable subject claims** (see the repo's
  Settings → Actions → OIDC configuration): `repo:owner@OWNER-ID/name@REPO-ID:ref:refs/heads/main`

The subject differs per ref/event; grab it from a failed exchange's 401
message or the workflow's OIDC debug output. The issuer is fixed to GitHub's
(`https://token.actions.githubusercontent.com`); any other OIDC issuer works
too if it's registered in the grant.

**CI side**: request the job token with audience `release-hub` and exchange
it:

```bash
# inside a GitHub Actions job with `permissions: id-token: write`
oidc=$(curl -fsS -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \
  -H "Accept: application/json; api-version=2.0" \
  "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=release-hub" | jq -r .value)
RELEASE_HUB_TOKEN=$(curl -fsS -X POST "$RELEASE_HUB_URL/api/oidc/exchange" \
  --data-urlencode "token=$oidc" | jq -r .token)
# use $RELEASE_HUB_TOKEN like any bearer, for one hour, on this app's publish
# endpoints only (upload/list/delete releases, manifest, signing-key fetch)
```

What the minted token can and cannot do:

- ✅ upload/list/delete releases **of the granted app**, read its manifest,
  fetch its signing key (CI signs the build with it)
- ❌ anything else: other apps, token creation, Play config, testers,
  release-hub-wide listings: 403
- Revoking the grant kills its outstanding minted tokens immediately; expired
  tokens are deleted on sight.

`.github/workflows/mobile-release.yml` in this repo is already wired this way.
