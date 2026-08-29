# Publishing to Google Play (internal → alpha → beta → public)

End-to-end guide for pushing releases from the hub to Google Play:
one-time Play Console + service-account setup, enabling it per app in the
hub, and the day-2 release loop. Everything here is also in `README.md` in
compressed form — this is the walkthrough version.

## How the hub talks to Play

- Uploads with `channel=internal` and an `.aab` go to the Play **internal
  testing** track; `channel=alpha` goes to the hub's **closed testing**
  track (`alpha` — created automatically on first use via
  `edits.tracks.create`, no Console visit needed); `channel=beta` goes to
  **open testing** (Google's track id `beta`); `channel=public` goes to
  **production**; `channel=direct` (or any `.apk`) never touches Play.
- Play track ids per the Publishing API docs: `production`, `beta` (open
  testing), `internal` (internal testing; alias `qa`), plus closed tracks
  with free-form names you chose at creation. `GET
  /api/apps/{slug}/{platform}/tracks` lists the app's real tracks and the
  hub channel value for each.
- Publishing uses the official Publishing API (`androidpublisher` v3) with a
  service-account JSON key stored **encrypted at rest** (AES-256-GCM, key
  from `RELEASE_HUB_SECRET_KEY` — the hub's existing secret).
- The release is recorded in the hub even if Play publishing fails; the API
  response carries `playRelease` (success, Play release name) or `playError`
  (failure reason) so CI can decide whether to fail the build.

## Where things happen (read this first)

The JSON key is **not created in Google Play** — setup spans two consoles:

| Console | What you do there |
|---|---|
| **Play Console** (play.google.com/console) | Link a Google Cloud project (Setup → API access); later invite the service account as a user with release permissions |
| **Google Cloud Console** (console.cloud.google.com) | Create the service account and download its **JSON key** — this file is the credential the hub stores |

## One-time: link API access in Play Console

The console UI moves this page around between redesigns. Reliable ways in:

- The **search box** at the top of Play Console: type "API access".
- Manually: console **home** (not inside an app) → ☰ menu → bottom group →
  **Setup** → **API access** (older UIs: **Users and permissions → API
  access**).
- A deep link that works when signed into a single account:
  `https://play.google.com/console/u/0/api-access` — bump `/u/0/` to
  `/u/1/` etc. if you're signed into multiple Google profiles and 0 is the
  wrong one. If this 404s, use the manual path above.

1. Sign in at <https://play.google.com/console> with the **account owner**
   Google account (e.g. the one owning `io.nesin.tinyfirewall`).
2. Either accept the suggested Google Cloud project (**View → Link**), or
   create/link a project of your choice. Any ordinary project works — it
   just hosts the service account.
3. If Play shows a setup wizard, you can follow it, or continue with the
   next section directly in Google Cloud Console.

## One-time: service account + JSON key

1. Open **Google Cloud Console → IAM & Admin → Service Accounts**
   (<https://console.cloud.google.com/iam-admin/serviceaccounts>) and make
   sure the project selector at the top shows the project you linked above.
2. **+ Create service account** → name it (e.g. `release-hub`) → **Create
   and continue** → skip roles (**no GCP roles needed** — permissions come
   from Play Console in the next section) → **Done**.
3. Open the new account → **Keys** tab → **Add key → Create new key →
   JSON → Create**. The key downloads immediately; **this is the only
   copy** — store it somewhere safe.
4. Note the account's email, e.g.
   `release-hub@my-project.iam.gserviceaccount.com`.

A single service account with account-wide release permissions can serve
many apps; uploading the same JSON per app in the hub is fine.

## One-time: grant the service account release access in Play

1. Back in **Play Console → Invite new users** (top search: "invite users",
   or **Users and permissions** in the sidebar).
2. Paste the service-account email from the previous step.
3. Under **Account permissions** check:
   - **View app information and download bulk reports**
   - **Create and manage releases** — covers testing tracks
   - **Release to production** (optional now; needed later for `public`)
4. **Invite user**. Permissions on a freshly linked API project can take up
   to 24h to activate — usually minutes.

## One-time: store the service account in the hub

The service account is **hub-wide** — one credential, every app. Upload it
once; enabling apps is just a toggle (no per-app credential copies).

**UI (recommended):** **Settings → Google Play service accounts** → choose
the JSON → **Save service account**. Re-uploading a JSON with the same
service-account email replaces the old key (rotation).

**API:**

```bash
# store / rotate the shared account
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=service-account.json \
     https://hub.example.com/api/play-accounts
# → {"id":1,"serviceAccount":"release-hub@….gserviceaccount.com"}

GET /api/play-accounts                # list ids + emails
POST /api/play-accounts/delete -F id=1  # remove (disables linked apps)
```

## Per app: enable Play publishing

**UI:** open the app's page — the **Google Play** box lists shared accounts
in a dropdown; pick one and **Enable Play publishing** (or upload a new key
inline, which also creates the shared account). A green dot shows it's on;
**disable** turns it off.

**API:**

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" -F account=1 \
     https://hub.example.com/api/apps/tinyfirewall/play
# → {"playEnabled":true,"serviceAccount":"release-hub@….gserviceaccount.com"}

# disable:
curl -H "Authorization: Bearer $HUB_TOKEN" -F enable=false \
     https://hub.example.com/api/apps/tinyfirewall/play
```

The legacy form (`-F file=service-account.json` posted to
`/api/apps/{slug}/play`) still works — it creates/updates the shared
account and enables the app on it.

## One-time: testers

- **Internal testing** — Play Console → the app → **Testing → Internal
  testing → Testers**: create an email list with the testers' Gmail
  addresses. The **opt-in link** on that page is opened once by each tester;
  after opting in, the Play Store app offers install/updates like any normal
  app. Email lists are Console-only — the API can neither create email
  lists nor read them (`edits.testers` documents: "email lists are not
  supported by this resource"), and it rejects group lists here too
  ("Cannot set tester group on an internal track"), so the hub never
  touches internal testers. (The API <i>can</i> read the number of joined
  testers via `tracks` list, but not the list itself.)
- **Closed testing (alpha)** — the hub's `alpha` channel uses its own
  closed track, created via API on first publish or first invite — nothing
  to set up. Attach testers as the hub's Google Group (invite-testers
  button, or `POST /api/apps/{slug}/{platform}/testers` with
  `channel=alpha`).
- **Open testing (beta)** — no tester list; anyone with the opt-in link can join.

## Day 2: cut a release

```bash
./gradlew bundleRelease
HUB=https://hub.example.com   # https://canele-hydrofoil.exe.xyz/hub in dev

curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app/build/outputs/bundle/release/app-release.aab \
     -F channel=internal -F versionName=1.22 \
     $HUB/api/apps/tinyfirewall/releases
# → {"apkUrl":…, "playRelease":"1.22 (212)"}
```

What happens: the hub records the release, stores the artifact, then opens
a Play edit, uploads the bundle, assigns it to the track and commits. Play
picks the `versionCode` from the bundle itself (must strictly increase per
app; the hub enforces monotonic version codes on its side too).

Promote later by re-uploading the same bundle with a higher versionCode to
`channel=beta` or `channel=public` (a separate release, after the app's
Console declarations — privacy policy, content forms — are complete).
Play's API has no promote call; each track gets its own upload.

## FAQ

**Do I need to create / enable the "Google Play Android Developer API"?**
No creation — that name *is* the Publishing API (`androidpublisher` v3) the
hub calls. Linking the Cloud project via Play Console **Setup → API
access** enables it in the project automatically. Manual enabling (Google
Cloud Console → APIs & Services → Library → "Google Play Android Developer
API" → Enable) is only a fallback for a failed link wizard or a
`403 … API has not been used in project` error. Note enabling the API
alone grants nothing — access comes from the service-account invite in
Play Console. Verify it's on: Cloud Console → APIs & Services → Enabled
APIs & services.

**There's no "create API key" option in Google Play — where is it?**
In Google **Cloud** Console, not Play: IAM & Admin → Service Accounts →
your account → Keys → Add key → JSON. See the table at the top.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `playError`: 403 … not linked | Service account wasn't invited in Play Console, or API access not linked yet. Re-check **Setup → API access** (find it via console search) and the invite. |
| `playError`: 403 … API has not been used in project | The link didn't enable the Play Android Developer API — enable it manually (see FAQ above). |
| `playError`: 401 invalid_grant | Wrong/revoked JSON key — re-download a fresh key and re-upload it in the hub. |
| `playError`: release failed … versionCode | Play rejected the version code (must strictly increase per app). Bump it. |
| Upload OK but testers see nothing | Track has no tester list / opt-in not completed, or Play still processing the release (minutes). |
| First upload ever fails on declarations | Play requires app declarations (privacy policy, content rating, target audience…) before it accepts artifacts. Finish them in Console. |
| `decrypt play credentials: …` | `RELEASE_HUB_SECRET_KEY` changed since upload — re-upload the JSON (stored data is unreadable without the original key). |
| Shared account deleted but app still shows enabled | Shouldn't happen (delete clears flags); re-toggle the app or re-enable it. |

## Security notes

- The JSON key unlocks Play releases for the account — treat it like a
  production credential. It is stored encrypted; the hub never logs it.
- The service account is shared hub-wide and stored **encrypted at rest**
  (AES-256-GCM, key from `RELEASE_HUB_SECRET_KEY` — the hub's existing
  secret); apps only carry an enabled flag.
- The UI routes are session-auth; the API routes are bearer-auth. Uploads
  are validated (must parse as service-account JSON with `client_email`)
  before storing.
- Deleting a shared account disables every app linked to it (flags cleared,
  credentials gone).
- Keep the hub behind TLS in production (session cookie + bearer tokens).
- If a key is compromised: delete it in Google Cloud (Service Accounts →
  Keys) and upload a fresh one — Play-side permissions survive key rotation.
