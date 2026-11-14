# Testing

| Layer | Where | Runs |
|---|---|---|
| Kit unit | `android/kit/src/test/` | Every PR |
| App unit + Compose UI (Robolectric) | `android/app/src/test/` | Every PR |
| Sync patterns | `.groot/tests/` | Every PR |
| Instrumented | the app's `src/androidTest` source set (add it when a real device is required) | On demand |

What the reference tests cover:

- `GrootColorsTest` — every text/background token pair meets WCAG AA (4.5:1) in light, dark and true black.
- `ThemeModeStoreTest`, `GrootThemeTest`, `GrootSettingsScreenTest` — persistence, system-follow, overrides,
  the settings screen (theme picker, reset confirmation, feedback mail draft, footer).
- `SettingsViewModelTest`, `NavigationTest` — app state and navigation.
- `KitContractTest` — the theme's colors and spacing reach an app screen. Keep one in your app: it's the
  check that catches a kit sync breaking your UI, and it asserts no layout or copy.
- `ScreenshotTest` — renders screens to PNGs for review with `pnpm screenshots`. It is not a golden-image gate.

Rules: fakes by constructor (see `FakePreferencesDataStore`), no network, clock or real disk, no sleeps.
