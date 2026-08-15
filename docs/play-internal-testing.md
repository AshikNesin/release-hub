# Publishing to Google Play (internal testing → production)

End-to-end guide for pushing releases from the hub to Google Play:
one-time Play Console + service-account setup, enabling it per app in the
hub, and the day-2 release loop. Everything here is also in `README.md` in
compressed form — this is the walkthrough version.

## How the hub talks to Play

- Uploads with `channel=internal` and an `.aab` go to the Play **internal
testing** track; `channel=public` goes to **production**; `channel=direct`
  (or any `.apk`) never touches Play.
- Publishing uses the official Publishing API (`androidpublisher` v3) with a
  service-account JSON key stored **encrypted at rest** (AES-256-GCM, key
  from `RELEASE_HUB_SECRET_KEY` — the hub's existing secret).
- The release is recorded in the hub even if Play publishing fails; the API
  response carries `playRelease` (success, Play release name) or `playError`
  (failure reason) so CI can decide whether to fail the build.

## One-time: link API access in Play Console

1. Sign in at <https://play.google.com/console> with the account that owns
   the app (e.g. the account owning `io.nesin.tinyfirewall`).
2. Left sidebar, bottom: **Users and permissions → API access**.
3. Either accept the suggested Google Cloud project (**View → Link**), or
   create/link a project of your choice. Any ordinary project works — it
   just hosts the service account.
4. If Play shows a setup wizard, you can follow it, or continue with the
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

1. Back in **Play Console → Users and permissions → Invite new users**.
2. Paste the service-account email from the previous step.
3. Under **Account permissions** check:
   - **View app information and download bulk reports**
   - **Create and manage releases** — covers testing tracks
   - **Release to production** (optional now; needed later for `public`)
4. **Invite user**. Permissions on a freshly linked API project can take up
   to 24h to activate — usually minutes.

## One-time: enable Play for the app in the hub

Two ways, same result (credentials validated, encrypted, stored):

**UI (recommended):** open the app's page in the hub. If the android
platform has no Play credentials yet, a **Google Play** box offers a file
picker — choose the service-account JSON → **Enable Play publishing**. The
box then shows the service-account email, the channel→track mapping and a
disable button.

**API:**

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=service-account.json \
     https://hub.example.com/api/apps/tinyfirewall/play
# → {"playEnabled":true,"serviceAccount":"release-hub@….gserviceaccount.com"}

# disable:
curl -H "Authorization: Bearer $HUB_TOKEN" -F enable=false \
     https://hub.example.com/api/apps/tinyfirewall/play
```

## One-time: testers for the internal track

Play Console → the app → **Testing → Internal testing → Testers**: create
an email list with the testers' Gmail addresses. The **opt-in link** on
that page is opened once by each tester; after opting in, the Play Store
app offers install/updates like any normal app.

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

Promote later by uploading with `channel=public` (a separate production
release, after the app's Console declarations — privacy policy, content
forms — are complete).

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `playError`: 403 … not linked | Service account wasn't invited in Play Console, or API access not linked yet. Re-check **Users and permissions → API access** and the invite. |
| `playError`: 401 invalid_grant | Wrong/revoked JSON key — re-download a fresh key and re-upload it in the hub. |
| `playError`: release failed … versionCode | Play rejected the version code (must strictly increase per app). Bump it. |
| Upload OK but testers see nothing | Track has no tester list / opt-in not completed, or Play still processing the release (minutes). |
| First upload ever fails on declarations | Play requires app declarations (privacy policy, content rating, target audience…) before it accepts artifacts. Finish them in Console. |
| `decrypt play credentials: …` | `RELEASE_HUB_SECRET_KEY` changed since upload — re-upload the JSON (stored data is unreadable without the original key). |

## Security notes

- The JSON key unlocks Play releases for the account — treat it like a
  production credential. It is stored encrypted; the hub never logs it.
- The UI route is session-auth; the API route is bearer-auth. Both validate
  the file (parses as service-account JSON with `client_email`) before
  storing.
- Keep the hub behind TLS in production (session cookie + bearer tokens).
- If a key is compromised: delete it in Google Cloud (Service Accounts →
  Keys) and upload a fresh one — Play-side permissions survive key rotation.
