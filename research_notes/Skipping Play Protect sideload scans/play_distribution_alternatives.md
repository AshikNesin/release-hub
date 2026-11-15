# Play-based and enterprise alternatives to raw APK sideloading (as of 2026-11-15)

Scope: a developer who has a Google Play Console account, self-distributes APKs, and runs a self-hosted updater app wants to install their own apps on their own or test devices without the "Scan app with Play Protect?" / "Play Protect hasn't seen this app before" prompt. Sources were fetched 2026-11-15 unless a date is given. "Official" means a Google docs page; "anecdote/third-party" is labelled as such.

Background on the prompt itself (official):
- The install-time prompt says "Play Protect hasn't seen this app before" and offers "Scan app" / "Don't install app". Play Protect shows it for apps it hasn't seen before. Google says the way to avoid it is to make sure Play Protect is aware of your app, and that appeals don't remove this message. — [Play Protect developer warning guidance](https://developers.google.com/android/play-protect/warning-dev-guidance)
- Google's Android Enterprise help: "This prompt will not appear on fully managed devices or in Work Profile. Users can still receive this prompt for apps in the personal profile." Play Protect still scans "apps that were either sideloaded or installed through an EMM". — [Google Play Protect (Android Enterprise help)](https://support.google.com/work/android/answer/15162069)
- Separate, newer layer (official): Android developer verification. Protections began **2026-09-30** in Brazil, Indonesia, Singapore and Thailand for installs on certified devices running Android 7+, with global expansion in 2027. Limited distribution accounts and the power-user "advanced flow" launched in August 2026. "Google Play automatically registers 99% of apps." Limited distribution accounts can share apps with **up to 20 devices** without government ID or a fee. — [Android developer verification](https://developer.android.com/developer-verification)
- Press reports say ADB installs are exempt from the advanced flow and its 24-hour wait. Google's guidance says developers and power users can still use ADB to install unverified apps on their own devices. — [The Hacker News, 2026-03](https://thehackernews.com/2026/03/google-adds-24-hour-wait-for-unverified.html); [Android Authority](https://androidauthority.com/how-to-bypass-android-developer-verification-sideloading-rules-3718002)

## Internal app sharing (Play Console)

### Takeaway
Internal app sharing is the fastest Play channel. You upload any-signed APK/AAB (UI or API), there is no review, and you get a link that installs through the Play Store app. Because the Play Store is the installer, the sideload "scan app" prompt shouldn't apply. The catch is signing: Play re-signs every upload with a per-app *internal app sharing key*. These builds can't update, or be updated by, your sideloaded or production-signed builds of the same package without an uninstall. That makes it a poor fit for a device you also run your self-hosted updater on.

### Cited Findings
- **How it works:** upload an AAB or APK on the internal app sharing page and get a download link. Access can be limited to email lists or open to anyone with the link. These artifacts can't be promoted to testing or production tracks and don't appear in App bundle explorer. — [Play Console Help: internal app sharing](https://support.google.com/googleplay/android-developer/answer/9844679)
- **Limits:** up to **100 users can download per link**. To reach more, re-upload for a new link. **Links expire 60 days** after upload. — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **No version-code constraint:** "Version codes don't need to be new or unique." Debuggable builds are allowed. — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **Signing:** uploads can be signed with any key. Google "automatically re-signs them with an Internal App Sharing key created for your app". The test certificate can be downloaded from the "Uploaders and testers" tab under "Internal test certificate". — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **Device toggle:** testers turn it on in the Play Store app: Settings > About > tap the Play Store version 7 times, then turn on "Internal app sharing". — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **Who can upload:** users with "Release apps to testing tracks" permission. Authorized uploaders don't need to be Play Console users. Testers must have access to the app's Play Store listing, so country or track availability can block them. — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **Review:** the help page describes no review step. The flow is upload, then an immediate link. — [same](https://support.google.com/googleplay/android-developer/answer/9844679)
- **API/CI:** the Play Developer API has `internalappsharingartifacts.uploadapk` and `.uploadbundle`. Each returns `downloadUrl` (opens the install in the Play Store app for authorized users), `certificateFingerprint` (SHA-256 of the signing cert) and `sha256`. — [Android Publisher API reference](https://developers.google.com/android-publisher/api-ref/rest/v3/internalappsharingartifacts)
- **Automatic protection:** "When uploading your app to internal app sharing, protection is not applied." This is deliberate, so debug builds can be shared. Google advises sharing these links only with trusted team members. — [Play Console Help: automatic protection](https://support.google.com/googleplay/android-developer/answer/10183279)

### Inferences
- Installs and updates go through the Play Store app (the link "opens the install in the Play Store app"). The installer is therefore Play, not a browser or file manager, and the unknown-source "Play Protect hasn't seen this app" prompt shouldn't fire. I found no official sentence saying "internal app sharing skips the scan", so this rests on how the mechanism works.
- Signing consequence: the internal app sharing key differs from both your self-signed sideload key and the Play app-signing (production) key. Moving a device between internal sharing builds and any other channel for the same package requires an uninstall, which loses app data. In practice: either dedicate test devices to internal app sharing, or use a different `applicationId` suffix (e.g. `.internal`) for those builds.
- Release Hub's existing updater can't silently update an internal-app-sharing install, because the signatures differ. Each new build needs a fresh link opened on the device, though that link could be generated from CI via `uploadapk` and surfaced in Release Hub.
- The 100-downloads-per-link cap is irrelevant for a single developer, and CI can upload per build. The 60-day expiry is also irrelevant if you upload per build.

### Gaps
- No official Google statement or reliable community test confirming that internal-app-sharing installs never show the Play Protect scan prompt. This is inferred from the Play Store being the installer.
- Whether the "Internal app sharing" Play Store toggle survives Play Store updates or resets wasn't found.
- API quotas and file-size limits for `internalappsharingartifacts` aren't stated on the reference page.

## Internal testing track, closed/open testing, and Publishing API automation

### Takeaway
The internal testing track (≤100 testers, available "within minutes") delivers builds through the Play Store signed with your Play app-signing key. Updates then come automatically from Play, with no sideload prompt. It's the most "production-like" low-friction channel. It requires monotonically increasing version codes. The 12-testers/14-days rule for new personal accounts gates **production** only, not internal testing.

### Cited Findings
- "An internal test can have up to 100 testers per app." A new release "becomes available to testers within minutes." On the first upload, the temporary name and listing info show for up to 48 hours. — [Play Console Help: testing tracks](https://support.google.com/googleplay/android-developer/answer/9845334)
- Closed testing: up to 200 email lists per account and 50 per track, each up to 2,000 testers, or Google Groups (no size limit). After the first publish "the test link can take several hours to become available." — [same](https://support.google.com/googleplay/android-developer/answer/9845334)
- Open testing: unlimited testers, or a cap of at least 1,000. Anyone with the link can join. — [same](https://support.google.com/googleplay/android-developer/answer/9845334)
- Testers must opt in via the opt-in link, which "displays only when an app status is 'Published'". — [same](https://support.google.com/googleplay/android-developer/answer/9845334)
- Personal accounts created after **2023-11-13** must run a **closed test with ≥12 testers opted in continuously for ≥14 days** before Production (and pre-registration) unlocks. Internal testing has no such requirement. — [Play Console Help: testing requirements for new personal accounts](https://support.google.com/googleplay/android-developer/answer/14151465)
- Google Play automatically delivers *protected* builds across all tracks (internal, closed, open, production) if automatic protection is on. Google recommends not promoting unprotected versions to open or production. With automatic protection, a copy obtained outside Play runs an installer check at launch, and "If the installer check fails, users will be prompted to get your app on Google Play." — [Play Console Help: automatic protection](https://support.google.com/googleplay/android-developer/answer/10183279)
- Third-party advice (not official): a sideloaded debug build looks like a rarely-seen APK with an unknown signing key, so distribute test builds via internal/closed testing with Play App Signing instead. — [ptkd.com article](https://ptkd.com/journal/blocked-by-play-protect-warning-fix)

### Inferences
- Internal testing builds are signed with the Play app-signing key, so they're the *same* signature as production. A device can move between internal, closed and production of the same package without an uninstall. That's the key advantage over internal app sharing.
- Conflict with self-hosting: if automatic protection is on and the self-hosted updater ships the *Play-protected* artifact, the in-app installer check would prompt users to get the app on Google Play. Ship only unprotected builds through the self-hosted channel, or leave automatic protection off.
- Signature conflict: a self-hosted APK signed with your upload/local key can't update a Play-installed build unless you sign the self-hosted build with the same key Play uses. With Play App Signing, Google holds that key, so you'd have to download Play-signed universal APKs (e.g. from App bundle explorer) to self-host. App bundle explorer wasn't verified in this session.
- Automation is via the Play Developer Publishing API (edits → upload bundle → assign to `internal` track → commit). This is the standard fastlane/gradle-play-publisher path. I didn't fetch the edits API page in this session.
- The review requirement for internal-track updates wasn't confirmed here. The "within minutes" wording implies little or no review delay, but don't state that as fact.

### Gaps
- Whether internal-track updates go through app review, and how long that takes in 2026. The official page says "within minutes" but doesn't say "no review".
- No official statement that Play-installed apps never trigger the scan prompt. This is inferred, since the prompt targets apps Play Protect "hasn't seen before" and Play-distributed apps are, by definition, known to Play.

## Managed Google Play private apps / Android Enterprise

### Takeaway
Private apps on managed Google Play are auto-approved, typically ready in about 10 minutes, and installed by Play. Separately, Google's own docs say the scan prompt "will not appear on fully managed devices or in Work Profile". An individual can therefore avoid the prompt by putting their test devices (or a work profile) under management. But it takes real setup effort: an EMM or Android Management API project, an enterprise binding, and device enrollment or work profile creation. Apps installed in the personal profile still get the prompt.

### Cited Findings
- Private apps published via the managed Google Play iframe are "typically ready for distribution within 10 minutes" (auto-approved). The first publish auto-creates a Play Developer account for the organization with no registration fee. Limit: 15 private apps per day, web apps included. Updates are made by uploading a new AAB or APK. — [Managed Google Play Help: publish private apps](https://support.google.com/googleplay/work/answer/9146439)
- Restrictions: iframe-published private apps can't become public or be transferred between developer accounts. AABs uploaded for new apps via the iframe use a **Google-generated signing key** unless you publish via Play Console, or create the app with an APK and enroll in Play App Signing first. Deletion needs Play Console support and doesn't free the package name. — [same](https://support.google.com/googleplay/work/answer/9146439)
- "App Scan Recommended" / "Send App for Security Check": "This prompt will not appear on fully managed devices or in Work Profile. Users can still receive this prompt for apps in the personal profile." — [Android Enterprise Help: Google Play Protect](https://support.google.com/work/android/answer/15162069)
- Third-party (Jason Bayton, Android Enterprise GDE; page last edited 2026-03-23): since **September 2024**, Play Protect "no longer sends sideloaded applications for cloud scanning on enterprise-managed devices", following enterprise feedback. On-device detection still flags known malware, and unmanaged devices keep the prompt. — [bayton.org FAQ](https://bayton.org/android/android-enterprise-faq/play-protect-sideload-scanning-change/); [bayton.org blog 2024-09](https://bayton.org/blog/2024/09/play-protect-changes-2024/)
- Anecdote: Android Enterprise Community and Meraki forum threads from admins complain about the prompt on managed deployments, with workarounds such as disabling Play Protect. Some of these likely predate the 2024 change. — [Android Enterprise Community thread](https://www.androidenterprise.community/t5/general-discussions/is-there-any-way-to-disable-google-play-protect-gpp-from-an-emm/m-p/2507); [Meraki community](https://community.meraki.com/t5/Mobile-Device-Management/Disabling-Play-Protect-for-Internal-Apps-Not-Published-on-Google/m-p/269155)

### Inferences
- For an individual, the cheapest way to get a managed device is the Android Management API with a self-created enterprise (managed Google Play account enterprise, no Workspace needed), or a free EMM tier. The work-profile route keeps the phone personal while the test apps live in the work profile, where the prompt is suppressed per Google's doc. None of these setup steps were fetched in this session (developers.google.com/android/management wasn't retrieved).
- On a fully managed (device-owner) device, a DPC or EMM can install silently. Combined with the managed-device prompt suppression, even the self-hosted APK installed by a device-owner app could avoid the prompt. This is a factory-reset-and-provision commitment, though, and is unrealistic for a daily-driver phone.
- Private-app signing caveat: if created via the iframe with an AAB, Google holds the key, so updates from the self-hosted channel would fail the signature check. Create the app via Play Console with your own Play App Signing setup instead.

### Gaps
- Android Management API enterprise sign-up steps, pricing (free) and quotas weren't fetched this session.
- Whether a *work profile* created by a minimal custom DPC (TestDPC) counts as "Work Profile" for prompt suppression wasn't confirmed. Inferred yes, since the doc keys on profile type.
- Exact definition of "enterprise-managed" in the Bayton FAQ (does it include work profile on a personally owned device?) isn't spelled out there.

## Other mechanisms: installer of record, update ownership, Play "Update from Play", Firebase App Distribution, device-owner installs

### Takeaway
The installer of record and update ownership govern *who can update silently*, not whether Play Protect scans. A self-hosted updater can update apps it installed itself without user action, if it meets the `setRequireUserAction` conditions. It can't claim enforced update ownership, which needs the privileged `ENFORCE_UPDATE_OWNERSHIP` permission. Firebase App Distribution APK builds are ordinary sideloads, so expect the prompt (not confirmed officially).

### Cited Findings
- `setRequestUpdateOwnership(boolean)` (API 34): with enforcement on, other installers "will need the user action to update the package even if the installers have been granted the INSTALL_PACKAGES permission". It can only be enabled on initial install. The installer must hold `ENFORCE_UPDATE_OWNERSHIP`. Default is false. — [PackageInstaller.SessionParams reference](https://developer.android.com/reference/android/content/pm/PackageInstaller.SessionParams#setRequestUpdateOwnership(boolean))
- `setRequireUserAction(USER_ACTION_NOT_REQUIRED)`: an installer holding `REQUEST_INSTALL_PACKAGES` can update **without user action** only if all of these hold:
  - The target-SDK floor is met. For example, the app must target ≥ API 34 on Android B (16), and ≥ API 35 on Android C, which is the next release after 16.
  - The installer is the app's update owner (if enforcement is on) or its installing package (if off), or it is updating itself.
  - The installer declares `UPDATE_PACKAGES_WITHOUT_USER_ACTION`.
  - The reference notes the target-API floor "will advance in future Android versions".
  — [same](https://developer.android.com/reference/android/content/pm/PackageInstaller.SessionParams#setRequireUserAction(int))
- `SessionParams.setExtensionParams` (added 36.1) passes context to the "developer verification service provider (a.k.a., the verifier)". Developer verification is wired into PackageInstaller sessions. — [same](https://developer.android.com/reference/android/content/pm/PackageInstaller.SessionParams#setExtensionParams(android.os.PersistableBundle))
- Android 14 update ownership: the initial installer can claim ownership, and other installers then need user approval. Play initially claimed ownership only for core apps. In **August 2024** Play began offering "Update from Play" for sideloaded apps, which switches update ownership to Play. — [AOSP: app ownership](https://source.android.com/docs/setup/create/app-ownership); [Android Authority 2023](https://androidauthority.com/android-14-sideload-warning-update-ownership-3345719); [Android Authority 2024](https://www.androidauthority.com/play-store-update-permission-apk-3466169/)
- Firebase App Distribution (Android APK): builds must be signed with your debug or app-signing key. Builds stay available for **150 days**, and invitations expire after 30 days. Testers get email invites and can optionally use the App Tester app. The docs fetched say nothing about Play Protect. — [Firebase: distribute via console](https://firebase.google.com/docs/app-distribution/android/distribute-console); [Firebase: tester setup](https://firebase.google.com/docs/app-distribution/get-set-up-as-a-tester?platform=android)
- Play Protect scans apps installed through an EMM as well as sideloaded ones (on-device scanning still happens even where the prompt is suppressed). — [Android Enterprise Help](https://support.google.com/work/android/answer/15162069)

### Inferences
- The self-hosted updater keeps "trust" only in the sense of silent updates for apps *it* installed. It doesn't make Play Protect treat those apps as known. If an app was installed from Play (installing package = Play, possibly update owner = Play), the self-hosted updater needs user confirmation to update it. It also needs a matching signature, and with Play App Signing that's the Google-held key.
- If a user accepts Play's "Update from Play" on a sideloaded app, ownership moves to Play, and the self-hosted updater then needs user action for each update.
- Firebase App Distribution APKs install via the App Tester app or browser, both non-Play installers. That makes them sideloads, and they should behave like the current self-hosted channel for the scan prompt. Firebase's AAB distribution is generally documented as using Play internal app sharing (requires linking Firebase to Play), which would inherit internal app sharing's Play-installer path and re-signing. **Not verified this session.** The AAB docs page didn't render.
- "Play Auto Install" is an OEM preload program, not a developer channel, and no source was found for it. Device-owner silent install is covered under the enterprise section.
- Comparison summary for the report writer:

| Channel | Skips scan prompt? | Speed | Limits | Signing / interop with self-hosted builds | Setup |
|---|---|---|---|---|---|
| Raw sideload / self-hosted updater (status quo) | No on unmanaged devices (official); ADB exempt from the verification advanced flow (press) | Instant | None (verification limits from 2026-09-30 in 4 countries; 2027 global) | Your key; silent self-updates possible | Done |
| Internal app sharing | Very likely (Play is the installer; inferred) | Upload → link immediately, no review | 100 downloads/link, 60-day expiry | Re-signed with IAS key; can't cross-update with other channels | Low; API `uploadapk`/`uploadbundle` |
| Internal testing track | Very likely (Play install; inferred) | "Within minutes" | 100 testers; increasing versionCode | Play app-signing key = production; self-hosted builds must use the Play-signed artifact | Low-medium; Publishing API |
| Closed/open testing | Very likely (inferred) | Hours for first link | Large | Same as internal | Medium; first opt-in requires Published |
| Managed Google Play private app + work profile/fully managed | Yes on fully managed or work profile (official) | ~10 min first publish | 15 private apps/day | Use Play Console-created app to keep your key | High (enterprise + enrollment) |
| Device-owner/DPC silent install of self-hosted APK | Prompt suppressed on fully managed (official); on-device scan still runs | Instant | Device must be provisioned from factory reset | Your key | High |
| Firebase App Distribution (APK) | Probably no (sideload; unconfirmed) | Fast | 150-day build retention | Your key | Low |

### Gaps
- No official Google statement or reliable community test on whether Firebase App Distribution APK installs show the "scan app" prompt.
- Couldn't verify how Firebase App Distribution's AAB path relates to internal app sharing in 2026 (page didn't render).
- No r/androiddev or Stack Overflow thread was found documenting observed scan-prompt behaviour for internal-app-sharing or internal-track installs. Evidence that Play channels skip the prompt is mechanistic, not observational.
- Whether `UPDATE_PACKAGES_WITHOUT_USER_ACTION` silent updates by a self-hosted updater still show the scan prompt for *new versions* of an app Play Protect hasn't seen wasn't found.
