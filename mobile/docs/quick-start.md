# Quick start

## Prerequisites

- JDK 21
- Android SDK with `ANDROID_HOME` set (Android Studio installs one). The build needs platform 37 to compile
  (it targets 36); Gradle downloads missing SDK packages on first build.
- Node 22+ and pnpm 10 (`corepack enable`)
- iOS: macOS with Xcode 26

## First run

```bash
pnpm install
pnpm check              # repo checks + Android lint and unit tests
pnpm android:build      # debug APK at android/app/build/outputs/apk/debug/
```

Open `android/` in Android Studio to run the app on a device or emulator, or `ios/GrootMobile.xcodeproj` in
Xcode for iOS (`docs/guides/ios.md`).

## Starting a new app from this boilerplate

```bash
scripts/new-app.sh ../my-app                      # both platforms
scripts/new-app.sh ../my-app --platforms android  # Android only; iOS never syncs
cd ../my-app && pnpm groot:setup                  # app name, applicationId, bundle id
```

Then keep it current with `pnpm groot:check` and `pnpm groot:sync` (see `docs/guides/sync.md`).

## Where to go next

- Rules for agents and humans: `AGENTS.md`
- Theming and custom UI: `docs/guides/customizing-ui.md`
- Tests: `docs/guides/testing.md`
- Design tokens: `docs/design/DESIGN.md`
