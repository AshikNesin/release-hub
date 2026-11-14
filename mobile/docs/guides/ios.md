# iOS

Needs macOS with Xcode 26 (CI pins the version in `.github/actions/setup-ios/action.yml`). Open
`ios/GrootMobile.xcodeproj` and run the `GrootMobile` scheme.

## Layout

- `ios/GrootKit/` — the kit, a local Swift package with no dependencies: tokens (`GrootColors`, `GrootPalette`,
  `GrootSpacing`), `.grootTheme(mode:palette:spacing:)`, `ThemeMode` + `ThemeModeStore`, `GrootSettingsView` and the
  settings rows, `BuildInfo`, `Log`, `StartupSignpost`. Same names and values as the Android kit.
- `ios/App/` — the reference app: Home, Components, Settings, About. Yours to change. `App/` is an Xcode
  synchronized folder, so adding or moving files never touches `project.pbxproj`.
- `ios/AppTests/` (Swift Testing) and `ios/AppUITests/` (launch metric + one flow) sit next to `App/`, so test files
  never compile into the app.
- `ios/App/Config/Base.xcconfig` — bundle id, display name, team. `ios/App/Config/Version.xcconfig` — version.
  `pnpm groot:setup` writes these; the project file never holds them.
- `ios/App/Sources/AppConfig.swift` — feedback email and the "Built by" name and link.

## Reading tokens

```swift
@Environment(\.grootColors) private var colors
@Environment(\.grootSpacing) private var S

Text("Hi").foregroundStyle(colors.onSurfaceVariant).padding(S.lg)
```

Brand colors go in `ios/App/Sources/UI/Brand/Brand.swift`, the only app folder where color literals are allowed.

## Screens

Build list screens from `SettingsList` + `SettingsSection` and the rows (`SettingsNavigationRow`, `SettingsLinkRow`,
`SettingsSwitchRow`, `SettingsInfoRow`, `SettingsActionRow`, `SettingsChoiceRow`). Picking one option always opens
`SettingsChoiceSheet` from the bottom; never use `Picker` menus or wheels. Icons are SF Symbol names.

## Tests

```bash
xcodebuild test -project ios/GrootMobile.xcodeproj -scheme GrootMobile -testPlan Unit \
  -destination 'platform=iOS Simulator,name=iPhone 17'
```

`Unit.xctestplan` runs `AppTests`; `Perf.xctestplan` runs `AppUITests`. The kit's `GrootKitTests` run from the
package's own scheme (`cd ios/GrootKit && xcodebuild test -scheme GrootKit -destination …`); `pnpm ios:test` runs
both. CI's iOS job does the same, on the newest installed Xcode that has a matching simulator (pinned to 26.6 in
`.github/actions/setup-ios/action.yml`).

## Lint, size and API checks

```bash
pnpm ios:lint   # SwiftLint (.swiftlint.yml), --strict in CI
pnpm ios:test   # app + kit tests on the iPhone 17 simulator
pnpm size:ios   # download size vs. the 3 MB budget
```

`pnpm check` runs SwiftLint, the kit API check and the unit tests when it runs on a Mac.

`scripts/size/ios-size.sh` archives a Release build. When a team is set (`DEVELOPMENT_TEAM` in
`ios/App/Config/Base.xcconfig`), it exports with App Thinning for one device and reads
`App Thinning Size Report.txt`. Without signing (CI by default), it zips the unsigned `.app`. That zip is an upper
bound because it keeps every image scale. The JSON report it writes under ios/build/size records which method it used.

`scripts/checks/kit-api-ios.sh` compares GrootKit's public API with the latest `v*` tag that contains the package
(`swift package diagnose-api-breaking-changes`). It runs only in the boilerplate. A break needs a `major` changeset.

`.github/workflows/perf.yml` runs `Perf.xctestplan` (launch metric + the bottom-sheet flow) nightly on a simulator.

## Not built yet

The TestFlight release script (M4, release-hub).
