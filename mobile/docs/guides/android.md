# Android

## Layout

- `android/kit/` — the kit library (`groot.kit`): tokens, `GrootTheme`, `ThemeModeStore`, `GrootSettingsScreen` and settings rows,
  `BuildInfo`, `Log`, `ReportFullyDrawn`. Explicit API mode is on.
- `android/app/` — the reference app (`groot.mobile`): Home, Settings, About. Yours to change.
- `android/build-logic/` — convention plugins: `groot.android.application`, `groot.android.library`,
  `groot.compose`, `groot.test`. Module build files stay a few lines long.
- `android/gradle/libs.versions.toml` — every dependency and version.

## SDK levels

minSdk 29, targetSdk 36, compileSdk 37 (current AndroidX releases require it). Set in
`android/build-logic/convention/src/main/kotlin/GrootAndroid.kt`.

## Edge-to-edge and the first frame

`MainActivity` calls `enableEdgeToEdge()`; screens use Material 3 `Scaffold`, which applies insets, and
`GrootTheme` sets the system-bar icon contrast for the current mode.

The saved theme mode must be right on the first frame. On API 31+ `ThemeModeStore` mirrors the choice to
`UiModeManager.setApplicationNightMode`, which the system applies from the splash screen on. On API 29–30
`MainActivity` reads the one DataStore key before `setContent`.

## Navigation and theme changes

Every screen uses one transition, defined in `android/app/src/main/kotlin/groot/mobile/navigation/AppNavHost.kt`:
only the screen being opened or closed moves: it slides in from the end over a still background
(250 ms), and slides back out on back. No fades, no parallax.
Gesture back (predictive back, on by default when targeting Android 16) has separate transitions in Navigation
2.10 — `predictivePopEnterTransition` / `predictivePopExitTransition` — that default to shrinking the screen into a
card. Set them to the same slide whenever you change the transitions. `MainActivity` handles
`uiMode` changes itself (`android:configChanges="uiMode"`), so switching the theme recolors the app in place
instead of recreating the screen.

## Size

`pnpm size:android` builds the release AAB (R8 full mode + resource shrinking) and measures the download size
for arm64-v8a / xxhdpi / en with bundletool. Budget: 3 MB. Icons are vector drawables in the app, not an
icon library.

## Background work and battery

The kit does no background work. If your app adds some, Play's **excessive partial wake locks** vital applies:
a session is excessive above 2 cumulative hours of non-exempt wake locks in 24 hours, and an app is flagged when
more than 5% of sessions over 28 days are excessive (Play may lower its visibility). Use WorkManager with
constraints, exit workers early when preconditions fail, and log `WorkInfo.getStopReason()`.
