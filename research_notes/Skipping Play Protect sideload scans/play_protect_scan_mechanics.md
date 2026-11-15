# Play Protect scanning of sideloaded APKs: when the prompt appears and what skips it (as of 2026-11-15)

Scope note: the dates on all sources are given inline. "Official" means Google docs, Google blog, AOSP source or the Android Management API reference. "Anecdotal" means issue trackers, blogs or press coverage of observed behaviour. Android developer verification (2026) and Play Console appeals are covered by other researchers and are only flagged here where they intersect.

## Q1. When does "App scan recommended" / "Send app for security check" appear, and what changed with real-time scanning and enhanced fraud protection?

### Takeaway
Google's own docs give a single trigger for the scan prompt: Play Protect "hasn't seen this app before", meaning it does not know the APK. Google publishes no install-count threshold and no other criterion. Separately, two kinds of hard block exist. "Harmful app blocked" is shown when an app is classified as a PHA. "App blocked to protect your device" (enhanced fraud protection) is shown when an app installed from an internet-sideloading source declares SMS, notification-listener or accessibility permissions.

### Cited Findings
- **(Official)** Google's warning guidance lists these warning types:
  - **"App Scan Recommended"**: "Play Protect hasn't seen this app before". It offers "Scan app" / "Don't install app".
  - **"Send App for Security Check"**: "This app is unknown to Play Protect. To protect yourself and others, send it to Google for a security check." For this variant the developer action is to "make sure Play Protect knows about your app", and the page says appeals "won't remove this message".
  - **"Harmful App Blocked"**: shown for PHA/malware/MUwS classifications. Advice includes using valid standard APK/ZIP headers.
  - **"App blocked to protect your device"**: shown for internet-sideloaded apps declaring RECEIVE_SMS, READ_SMS, NOTIFICATION_LISTENER or ACCESSIBILITY.
  - **"Deceptive Accessibility Tool"**: shown for misuse of `isAccessibilityTool="true"`.
  - **"Android App Compatibility Too Low"**: shown when targetSdk is more than 2 levels below the device's API level.
  - **DPC enrollment block**.

  — [Developer Guidance for Google Play Protect Warnings](https://developers.google.com/android/play-protect/warning-dev-guidance)
- **(Official)** For non-Play installs, "If the app is identified as malicious or suspicious, we will warn users or block the installation in extreme cases." When Play Protect "doesn't recognize" the code, it "recommends a real-time code-level scan of the app to extract important signals for evaluation by Google". The user must agree before the app data is uploaded. — [Play Protect client protections](https://developers.google.com/android/play-protect/client-protections)
- **(Official)** User help page: "You may get a recommendation to scan an app from outside of Google Play". "Scanning the app will send app details to Google for a code-level evaluation". Play Protect "may ask you to send unknown apps to Google", and developers may be asked to send each new version of their app. — [Google Play Help: Use Google Play Protect](https://support.google.com/googleplay/answer/2812853)
- **(Press, Oct 18 2023)** Real-time code-level scanning was announced on 2023-10-18. The prompt is shown for apps that "have never been scanned before". Signals are extracted and evaluated on Play Protect's backend; this was intended to catch polymorphic malware. Existing on-device ML and similarity checks continue. The rollout started in India, "expanding to all countries in the coming months". — [9to5Google](https://9to5google.com/2023/10/18/google-play-protect-scan/); Google's post is [Enhanced Google Play Protect real-time scanning for app installs](https://security.googleblog.com/2023/10/enhanced-google-play-protect-real-time.html) (the body could not be fetched).
- **(Press/CSA, Feb 2024)** The enhanced fraud protection pilot started in Singapore with the Cyber Security Agency. It blocks installs "from Internet-sideloading sources (web browsers, messaging apps or file managers)" of apps that request RECEIVE_SMS, READ_SMS, BIND_NOTIFICATIONS or Accessibility. Google reported that more than 95% of installs of the relevant fraud families came from such sources. — [CSA Singapore press release](https://www.csa.gov.sg/news-events/press-releases/google-partners-with-the-cyber-security-agency-of-singapore-to-pilot-a-new-enhanced-security-feature-to-better-protect-android-mobile-users-from-scams/); [The Hacker News, Feb 2024](https://thehackernews.com/2024/02/google-starts-blocking-sideloading-of.html)
- **(Press, Oct 2024)** The India pilot was announced in Oct 2024 and started in Nov 2024. Earlier pilots had run in Singapore, Thailand and Brazil. On a blocked install, "Play Protect will automatically block the installation with an explanation to the user". — [Cyber Security News, citing blog.google India post](https://cybersecuritynews.com/google-to-block-malicious-sideloaded-apps/); [TechCrunch, 2024-10-03](https://techcrunch.com/2024/10/03/google-pilots-blocking-some-sideloaded-apps-in-india)
- **(Press, ~Jan 2025)** Expansion was reported to India, Brazil, Hong Kong, Kenya, Nigeria, the Philippines, South Africa, Thailand and Vietnam. — [Business Standard](https://business-standard.com/technology/tech-news/google-play-protect-s-new-tools-enhances-app-security-on-android-details-125013000970_1.html)
- **(Official/press, 2024–2025)** Live threat detection was announced at I/O 2024 and shipped on Pixel 6+ in Nov 2024. It performs on-device behavioural analysis through Private Compute Core after install. It is not an install-time prompt. In Oct 2025 an OEM expansion was reported as planned. — [Google Security Blog, Nov 2024](https://security.googleblog.com/2024/11/new-real-time-protections-on-Android.html); [Android Authority](https://www.androidauthority.com/play-protect-live-threat-detection-rollout-3499069/); [Dataconomy, 2025-10-23](https://dataconomy.com/2025/10/23/googles-live-threat-detection-is-reportedly-coming-to-more-android-phones/)
- **(Anecdotal, Oct 2026)** One project observed a newer variant: "App blocked to protect your device: Play Protect hasn't seen an app from this developer before. It may be unsafe." The app's manifest declared only INTERNET. The project links this to developer verification, whose enforcement reportedly began 2026-09-30 in selected regions. — [Castlegame issue #296, 2026-10-03](https://github.com/abolfazl2600/Castlegame/issues/296)

### Inferences
- There are two separate gates. The first is about how well Play Protect knows the app; a scan resolves it. The second is about policy, either the sensitive-permission fraud block or PHA classification; a scan does not lift it. Avoiding the fraud-block permissions (SMS, notification listener, accessibility) is the main developer lever for the second gate.
- "Low install count" is not a documented criterion. The documented criterion is "unknown / hasn't seen before", which plausibly correlates with install volume, but that is speculation.
- Google defines internet-sideloading sources as "web browsers, messaging apps or file managers". Whether a dedicated self-hosted installer app counts as such a source is not documented.

### Gaps
- No official, current (Nov 2026) list of countries where enhanced fraud protection is active. No primary Google text on how installer apps other than browsers, messengers or file managers are classified.
- The exact "unknown app" criterion (hash vs. certificate vs. package) is not published.
- The text of the Oct 2023 Google blog post body could not be retrieved.

## Q2. Does a previously scanned APK stop prompting? Is it per device or global?

### Takeaway
Google's docs say only that the prompt is for apps Play Protect "hasn't seen", and that developers may be asked to send "each new version". That implies the unit is the specific APK or version, not the signing certificate. Whether one user's scan clears the prompt for every other device is not confirmed by any official source. Reports show that every new release of a self-updating sideloaded app keeps prompting.

### Cited Findings
- **(Official)** Developers "may be asked to send each new version" of their app to Google. — [Google Play Help 2812853](https://support.google.com/googleplay/answer/2812853)
- **(Official)** For the "unknown to Play Protect" variant, the fix is to "make sure Play Protect knows about your app", and appeals "won't remove this message". — [Warning dev guidance](https://developers.google.com/android/play-protect/warning-dev-guidance)
- **(Official)** When the user agrees, Play Protect uploads app data and reports back "appears safe to install" or "potentially harmful". — [Client protections](https://developers.google.com/android/play-protect/client-protections)
- **(Anecdotal, Oct–Nov 2026)** The Harbor project, which self-updates over a PackageInstaller session using `UPDATE_PACKAGES_WITHOUT_USER_ACTION` with targetSdk ≥33 and the same signing key every release, reports that "App scan recommended" appears on every update on a Samsung phone running Android 14+. Enabling "Improve harmful app detection" did not stop it.
  - The maintainers proposed several fixes: uploading each release to a Play Console track, developer verification registration, an appeal, and dropping sensitive permissions. None was confirmed effective.
  - A follow-up comment on 2026-11-10 says developer verification will not remove this dialog. It also says the claims that Play-generated APKs skip the prompt, and that one user's scan clears it for others, are unconfirmed.

  — [RDevNeo/harbor issue #88](https://github.com/RDevNeo/harbor/issues/88)
- **(Anecdotal blogs)** Tutorials claim that uploading the build, even as a draft, to a Play Console internal test track makes Play Protect "know" the app or key. This is not confirmed by Google docs. — [tutorialpedia](https://www.tutorialpedia.org/blog/installation-app-blocked-by-play-protect/); [search summary of related sources incl. Castlegame #296](https://github.com/abolfazl2600/Castlegame/issues/296)

### Inferences
- In practice, each new APK (new hash) of a sideloaded app is likely "unseen" until Google has scanned or ingested it. A stable signing key and an unchanged package name are not documented as sufficient.
- The plausible developer lever is to make each exact release artifact known to Google before users install it. One route is uploading the identical build to a Play Console track. Uploading it yourself on a test device with "Scan app" or "Improve harmful app detection" turned on might also prime it. Both are unverified. They make the check pass, or possibly skip the prompt; they do not disable it.
- If Play App Signing re-signs the build, the Play-distributed artifact differs from your self-signed APK. Priming via Play only helps if the exact bytes or certificate you distribute are what Google has seen. This is unverified.

### Gaps
- No official statement on whether scan results are keyed to APK digest, signing certificate or package, or whether they are shared across devices and users.
- No controlled test results found (for example, "scan on phone A, then install on phone B").

## Q3. User-side and enterprise toggles

### Takeaway
Turning off "Scan apps with Play Protect" disables the install-time verifier, which removes the prompt and the scan. "Improve harmful app detection" only automates uploads; it does not suppress anything. On managed devices, the Android Management API can only enforce Play Protect or leave it to the user. There is no "disable" value. The legacy DPC user restriction `ENSURE_VERIFY_APPS` forces verification, including for ADB installs.

### Cited Findings
- **(Official)** Play Store > profile > Play Protect > Settings has two toggles. The first is "Scan apps with Play Protect", which turns scanning on or off. The second is "Improve harmful app detection", which allows Play Protect "to automatically send unknown apps to Google". Google recommends keeping Play Protect on. — [Google Play Help 2812853](https://support.google.com/googleplay/answer/2812853)
- **(Anecdotal)** "Improve harmful app detection" did not stop "App scan recommended" on every update. — [harbor #88](https://github.com/RDevNeo/harbor/issues/88)
- **(Official, AMAPI)** `advancedSecurityOverrides.googlePlayProtectVerifyApps` has two meaningful values:
  - `VERIFY_APPS_ENFORCED`: "Default. Force-enables app verification." Unspecified values default to this.
  - `VERIFY_APPS_USER_CHOICE`: "Allows the user to choose whether to enable app verification."

  It replaces the deprecated `ensureVerifyAppsEnabled`. There is no value that disables Play Protect. `untrustedAppsPolicy` defaults to `DISALLOW_INSTALL`, with options `ALLOW_INSTALL_IN_PERSONAL_PROFILE_ONLY` and `ALLOW_INSTALL_DEVICE_WIDE`. — [AMAPI enterprises.policies reference](https://developers.google.com/android/management/reference/rest/v1/enterprises.policies)
- **(Official, AOSP)** With the `UserManager.ENSURE_VERIFY_APPS` restriction set by a DPC, ADB installs are verified even if the ADB-skip setting is 0. On timeout the default verification response becomes `VERIFICATION_REJECT`. — [AOSP VerifyingSession.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/VerifyingSession.java)
- **(Official)** Warning guidance notes that the sensitive-permission block also applies during enterprise enrollment for apps from those sources. — [Warning dev guidance](https://developers.google.com/android/play-protect/warning-dev-guidance)

### Inferences
- An enterprise can at most hand the choice back to the user (`VERIFY_APPS_USER_CHOICE`). The user must still switch Play Protect off themselves.
- For enterprise fleets, distributing the APK as a managed Google Play private app means the app is installed by Play rather than sideloaded. That avoids the sideload path, but it is a different distribution model (inference).
- Turning Play Protect off also removes daily scanning and PHA protection. Google's docs discourage it.

### Gaps
- It is not documented whether turning off "Scan apps with Play Protect" also disables the enhanced-fraud-protection block. That block is part of Play Protect, so it is likely, but this is unconfirmed.
- No official mapping between the Play Store toggle and any `Settings.Global` key in current builds. Historical `package_verifier_enable` / `package_verifier_user_consent` keys are absent from AOSP main's Settings.java, so they are presumably owned by Play services or the Play Store and not settable reliably.

## Q4. Does `adb install` skip the scan? What can be set via `adb shell settings put global`?

### Takeaway
Yes, ADB installs can skip verification. `adb shell settings put global verifier_verify_adb_installs 0`, which is the same as the Developer options toggle "Verify apps over USB", bypasses the verifier for ADB installs only. `pm install --skip-verification` also works, but only for updates of debuggable apps. Neither helps PackageInstaller sessions created by an installer app. A DPC's `ENSURE_VERIFY_APPS` overrides both.

### Cited Findings
- **(Official, AOSP Settings.java)** `Settings.Global.PACKAGE_VERIFIER_INCLUDE_ADB = "verifier_verify_adb_installs"`: "1 = perform package verification on ADB installs (default) 0 = bypass package verification on ADB installs". The key is `@hide`. — [AOSP Settings.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/core/java/android/provider/Settings.java)
- **(Official, AOSP VerifyingSession.isAdbVerificationEnabled)** If the `ENSURE_VERIFY_APPS` user restriction is set, verification always runs. Otherwise, if `verifier_verify_adb_installs` is 0, the verifier is skipped. If the installer requested `--skip-verification`, fresh installs are still verified, and the verifier is skipped only when the APK is `debuggable` ("Only skip when apk is debuggable"). — [AOSP VerifyingSession.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/VerifyingSession.java)
- **(Official, AOSP)** `pm install --skip-verification` sets `INSTALL_DISABLE_VERIFICATION`. A hidden `pm disable-verification-for-uid <uid>` exists; it is callable only by system, root or shell and applies one-off to the next session from that UID. — [AOSP PackageManagerShellCommand.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/PackageManagerShellCommand.java); [AOSP PackageInstallerService.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/PackageInstallerService.java)
- **(Official, AOSP)** Developer options got a setting to "skip package verification when installing over USB/ADB/ADT, but still perform package verification on all other installs". The CTS/test harness runs `settings put global verifier_verify_adb_installs 0`. — [AOSP commits mirrored at /e/ GitLab](https://gitlab.e.foundation/e/os/android_frameworks_base/-/commit/4e5753f5f6c5e0c7f53722e2bfa451ac7f0b27db); [Settings commit](https://gitlab.e.foundation/e/os/android_packages_apps_Settings/-/commit/f04211443e9cae815ad89f821b6ec83ec2790c0c)
- **(Official, AOSP)** Other related Global keys are `verifier_timeout`, `verifier_default_response` and `verifier_setting_visible`. — [AOSP Settings.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/core/java/android/provider/Settings.java)

### Inferences
- `verifier_verify_adb_installs=0` is a per-device developer convenience, for example on QA devices. It needs USB debugging or a shell and does not apply to end-user installs through your installer app.
- Changing `verifier_default_response` affects only what happens on verifier timeout. It does not stop Play's verifier from showing its UI.

### Gaps
- No Google documentation confirms that the Play Store verifier honours every AOSP code path the same way on all OEM builds. The AOSP logic is authoritative for the framework gate only.

## Q5. Does installer identity matter (Play-installed installer, setInstallerPackageName, update ownership)? Do updates re-trigger scanning?

### Takeaway
At the framework level, a normal third-party installer cannot opt out of verification. AOSP strips `INSTALL_DISABLE_VERIFICATION` from every session that is not an ADB test install. Verification runs for fresh installs and for updates. `USER_ACTION_NOT_REQUIRED` and Android 14 update ownership remove the OS confirmation dialog, not the Play Protect verifier. The installer's identity is passed to Play Protect, which uses it for risk classification. For example, the fraud block applies to "internet-sideloading sources".

### Cited Findings
- **(Official, AOSP)** In `PackageInstallerService.createSession`, unless the caller is the one-off UID set by shell or root, or the install is ADB dev mode (`INSTALL_FROM_ADB | INSTALL_ALLOW_TEST`), `params.installFlags &= ~INSTALL_DISABLE_VERIFICATION`. The source comment reads "Only tools under specific conditions ... can disable verification." — [AOSP PackageInstallerService.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/PackageInstallerService.java)
- **(Official, AOSP)** `isVerificationEnabled()` returns true for non-ADB installs. Its only exception is instant apps whose installer is the verifier. There is no check on whether the package already exists, so updates are verified like fresh installs. — [AOSP VerifyingSession.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/VerifyingSession.java)
- **(Official, AOSP)** The verification broadcast carries the following to the verifier (Play Store / Play Protect):
  - `EXTRA_VERIFICATION_INSTALLER_PACKAGE`, the initiating package
  - `EXTRA_VERIFICATION_INSTALLER_UID`
  - `EXTRA_ORIGINATING_URI`
  - `EXTRA_REFERRER`
  - `EXTRA_ORIGINATING_UID`

  The initiating package is the real caller and is not the spoofable `setInstallerPackageName` value. — [AOSP VerifyingSession.populateInstallerExtras](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/VerifyingSession.java)
- **(Official/press)** The fraud block's scope is installs from "Internet-sideloading sources (web browsers, messaging apps or file managers)". — [CSA Singapore](https://www.csa.gov.sg/news-events/press-releases/google-partners-with-the-cyber-security-agency-of-singapore-to-pilot-a-new-enhanced-security-feature-to-better-protect-android-mobile-users-from-scams/)
- **(Anecdotal)** The scan prompt appears on every update installed silently through a `UPDATE_PACKAGES_WITHOUT_USER_ACTION` self-updater. The maintainers' workaround is to start installs only while the app is in the foreground, so the user can tap "Scan app". — [harbor #88](https://github.com/RDevNeo/harbor/issues/88)

### Inferences
- For release-hub's installer, which uses `setRequireUserAction(USER_ACTION_NOT_REQUIRED)`:
  - Silent updates still pass through Play Protect verification.
  - If the APK is unknown, the verifier can surface "App scan recommended", which breaks the "silent" flow.
  - Installs should be committed while the installer UI is in the foreground, or the user should be notified.
- Whether Play Protect treats a Play-installed installer more leniently than a sideloaded one is undocumented. The initiating package and UID are available to it, so differentiation is technically possible.
- `setInstallerPackageName` and `setRequestUpdateOwnership` do not change the initiating package reported to the verifier, so they are unlikely to affect Play Protect. This is inferred from the AOSP code.

### Gaps
- No Google doc on whether installer reputation, for example an installer that is itself on Play, affects the scan or the fraud-block classification.
- No source tested whether the verifier UI appears when the installing app is in the background, or whether the install waits or fails (`verifier_timeout`).

## Q6. Is there an official developer opt-out API or manifest flag?

### Takeaway
No. None of Google's Play Protect pages (client protections, warning guidance, help center) mention an opt-out. AOSP blocks third-party installers from disabling verification. The official levers make the check pass rather than skip it:
- making the app "known" to Play Protect
- avoiding the sensitive permissions that trigger the fraud block
- targeting a recent SDK
- valid APK/ZIP structure
- appeals, for misclassifications only; Google says an appeal does not remove the "unknown app" message
- developer verification registration (2026), which is a separate requirement and per community analysis does not remove the scan prompt

### Cited Findings
- **(Official)** The developer guidance offers only remediation and appeals, with no opt-out flag. Its advice is to minimise permissions, use the SMS Retriever / User Consent APIs instead of READ_SMS, target the latest API, use valid APK/ZIP headers, and "make sure Play Protect knows about your app". — [Warning dev guidance](https://developers.google.com/android/play-protect/warning-dev-guidance)
- **(Official)** The client protections page describes no developer opt-out; it lists only an appeal process. — [Client protections](https://developers.google.com/android/play-protect/client-protections)
- **(Official, AOSP)** `INSTALL_DISABLE_VERIFICATION` is cleared for any session that is not an ADB test install. — [AOSP PackageInstallerService.java](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/pm/PackageInstallerService.java)
- **(Anecdotal, Nov 2026)** "Developer verification won't remove this dialog", but it is needed anyway for updates once enforcement goes global in 2027. — [harbor #88](https://github.com/RDevNeo/harbor/issues/88)

### Inferences
Summary of levers:
- **Skips the prompt:** the user disables "Scan apps with Play Protect"; for ADB installs only, `verifier_verify_adb_installs=0` or USB debugging; installing through Play or managed Play instead of sideloading.
- **Makes it pass or reduces it:** Google already knowing the exact APK; no SMS, notification-listener or accessibility permissions; current targetSdk; clean APK structure.
- **Does not help:** "Improve harmful app detection" (anecdotal), appeals for the unknown-app variant (official), `setInstallerPackageName`, update ownership, `USER_ACTION_NOT_REQUIRED`.

### Gaps
- No Google confirmation that uploading to a Play Console track primes Play Protect for identically signed sideloaded APKs. This needs empirical testing on two devices with different accounts.
