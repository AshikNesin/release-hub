# Signing keystores (per app)

App signing keys live in the release hub, encrypted like Play credentials
(`RELEASE_HUB_SECRET_KEY`) or in a [1Password vault](1password.md), so **any** authenticated build environment can
produce signed releases, and the key survives a lost laptop. One keystore
per app; never share across apps.

**Auto-generated at platform creation.** Adding the android platform to an
app (`POST /api/apps/{slug}/platforms`) generates a fresh RSA-2048 keystore
(30-year self-signed cert, random high-entropy password, alias `release`)
and stores it encrypted: no keytool, no upload, no thinking about keys:

```bash
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" -F slug=myapp \
     https://release-hub.example.com/api/apps
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F platform=android -F packageName=io.example.myapp \
     https://release-hub.example.com/api/apps/myapp/platforms
# → {"slug":"myapp","platform":"android","signingKey":"generated","signingSha256":"26a4…"}
```

The generated key never rotates (rotation breaks installed-base update
signature continuity). To bring an app with an existing key history instead,
upload it explicitly; that replaces the generated one:

```bash
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F file=release.jks -F storePassword=... -F keyAlias=release \
     https://release-hub.example.com/api/apps/tinyfirewall/signing
# → {"stored":true,"keystoreSha256":"8862…"}
```

Fetch (CI / any coding env):

```bash
curl -sD /tmp/h -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -o app.jks https://release-hub.example.com/api/apps/tinyfirewall/signing
STORE_PW=$(grep -i x-release-hub-store-password /tmp/h | cut -d' ' -f2 | tr -d '\r')
ALIAS=$(grep -i x-release-hub-key-alias /tmp/h | cut -d' ' -f2 | tr -d '\r')
KEY_PW=$(grep -i x-release-hub-key-password /tmp/h | cut -d' ' -f2 | tr -d '\r')
# verify integrity:
echo "$(grep -i x-release-hub-keystore-sha256 /tmp/h | cut -d' ' -f2 | tr -d '\r')  app.jks" | sha256sum -c
```

Then point gradle at it (`signingConfigs` reading env vars). Delete with
`POST /api/apps/{slug}/signing/delete`.

⚠️ This endpoint is bearer-auth only and returns live key material: the release hub
token now unlocks signing. Keep tokens scoped and the release hub behind TLS.
Also keep an offline backup of the keystore: if Play App Signing is NOT
enrolled, losing this key means users can never update the app.
