# Keyless publishing (OIDC)

CI can publish with **no stored release hub secret**. The release hub trusts a GitHub repo's
Actions identity: register a grant binding that repo to **one app** (one repo
→ one app), and the workflow exchanges its short-lived job token for a release hub
token on every run.

**Release Hub side, one click (recommended)**: connect GitHub once in
Settings → GitHub. Release Hub registers **its own private GitHub App** through
GitHub's [manifest flow](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest):
you confirm once on GitHub, then pick the repositories it may see. Then on
App → Configuration → Keyless publishing, pick a repository and press
**Connect**. That:

- creates a **repository grant**, bound to the repo's immutable ids
  (`repository_id`, `repository_owner_id` claims) and its default branch
  (`ref` claim), so subject formats, renames and transfers don't matter;
- sets the repo's Actions variables `RELEASE_HUB_URL` (this release hub's
  `RELEASE_HUB_BASE_URL`) and `RELEASE_HUB_APP` (the app slug). If one already
  holds something else, you're asked before it's replaced.

Requirements: `RELEASE_HUB_BASE_URL` must be the public `https://` URL (GitHub
redirects your browser there; runners call it), and the app's private key needs
a home, so set `RELEASE_HUB_SECRET_KEY` or configure 1Password. The app asks only
for **Metadata: read** and **Variables: read and write**; webhooks are off, so
GitHub never calls the release hub. Installation tokens are minted on demand
and kept in memory. Removing a repository grant also removes the two variables
where they still hold Release Hub's values. **Disconnect** in Settings → GitHub
forgets the app locally (existing grants keep working); delete the app on
GitHub too if you no longer need it. Moving an already-stored GitHub App key
into 1Password isn't automatic yet: disconnect and connect again after setting
up 1Password.

**Release Hub side, by hand** (any OIDC issuer, or no GitHub App): under
"Other CI, or set up by hand", paste the repo's OIDC
subject — a bare repo prefix is completed to the default-branch ref automatically
(`repo:owner/name` or `repo:owner@OWNER-ID/name@REPO-ID` becomes
`…:ref:refs/heads/main`). The prefix depends on the repo's age:

- repos created before 2026-07-15: `repo:owner/name:ref:refs/heads/main`
- newer repos use **immutable subject claims** (see the repo's
  Settings → Actions → OIDC configuration): `repo:owner@OWNER-ID/name@REPO-ID:ref:refs/heads/main`

The subject differs per ref/event; grab it from a failed exchange's 401
message or the workflow's OIDC debug output. The issuer is fixed to GitHub's
(`https://token.actions.githubusercontent.com`); any other OIDC issuer works
too if it's registered in the grant. Then set a `RELEASE_HUB_URL` repo variable
yourself.

**CI side**: request the job token with audience `release-hub` and exchange
it:

```bash
# inside a GitHub Actions job with `permissions: id-token: write`
oidc=$(curl -fsS -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \
  -H "Accept: application/json; api-version=2.0" \
  "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=release-hub" | jq -r .value)
RELEASE_HUB_TOKEN=$(curl -fsS -X POST "$RELEASE_HUB_URL/api/oidc/exchange" \
  --data-urlencode "token=$oidc" | jq -r .token)
# the reply's .app is the granted app's slug (also in vars.RELEASE_HUB_APP)
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
