# Customizing UI

## Brand the app

Pass your own tokens to `GrootTheme` from the app's brand folder,
`android/app/src/main/kotlin/groot/mobile/ui/brand/Brand.kt`:

```kotlin
val BrandPalette = GrootPalette(
    light = GrootColors.Light.copy(primary = Color(0xFF6750A4)),
    dark = GrootColors.Dark.copy(primary = Color(0xFFD0BCFF)),
)
```

`brand/` is the one place in app code where color literals are allowed. Spacing works the same way with
`GrootSpacing`.

## App-only tokens

Add a `CompositionLocal` in app code and provide it inside `GrootTheme`. Don't add app tokens to the kit.

## Custom components

Put them in `android/app/src/main/kotlin/groot/mobile/ui/components/` and read colors from
`MaterialTheme.colorScheme` and spacing from `S`.

## The settings screen

The kit ships `GrootSettingsScreen`, modelled on gold-view and one-song: a large collapsing title, your
sections, then Appearance (theme picker), About (version, or an "About" row when you pass
`onOpenAbout`; rate, feedback mail draft, privacy policy), Data
(reset all data, with confirmation), an optional privacy note and a "Built by <maker>" footer.

Add your own sections with the kit's building blocks so they match:

```kotlin
GrootSettingsScreen(themeMode, onThemeModeChange, about = SettingsAbout(appName, buildInfo, feedbackEmail, makerName, makerUrl)) {
    SettingsSection("Security") {
        SettingsSwitchRow(title = "App Lock", summary = "Require fingerprint", checked = locked, onCheckedChange = setLocked)
        SettingsNavigationRow(title = "City", value = "Chennai", onClick = openCityPicker)
    }
}
```

Each section is one quiet card; dividers between rows are drawn for you. Rows: `SettingsSwitchRow`,
`SettingsNavigationRow` (in-app, chevron; `value` shows on the right), `SettingsLinkRow` (leaves the app,
outward arrow), `SettingsChoiceRow` (options listed in place, selected one checked), `SettingsActionRow`
(`destructive = true` for danger), `SettingsInfoRow`, plus `SettingsAppHeader` (centered identity for About).
Keep rows to one line where you can; color is reserved for state and danger. The app's identity (feedback email, maker) lives in
`android/app/src/main/res/values/config.xml`; `pnpm groot:setup` fills it in.

You get fixes to the screen through sync. If you need a different layout, copy it into your app and stop
calling the kit version. Never patch the kit copy.
