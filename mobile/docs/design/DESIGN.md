# Design tokens

Single source of truth for kit tokens. The Android values live in
`android/kit/src/main/kotlin/groot/kit/tokens/` and `ios/GrootKit/Sources/GrootKit/Tokens/`; they must match.

## Color

Monochrome, after tiny-firewall (RFC 0001 §4.1). Dark mode is true black with `#1A1A1A` cards, white
text and controls, grey secondary text; white is the only accent (switch on, selected radio, primary
button). Light mode is the mirror image. Red marks destructive actions only. Every text/background pair
meets WCAG AA (4.5:1), checked by `GrootColorsTest`.

| Role | Light | Dark |
|---|---|---|
| primary / onPrimary | `#000000` / `#FFFFFF` | `#FFFFFF` / `#000000` |
| primaryContainer / onPrimaryContainer | `#E0E0E0` / `#000000` | `#333333` / `#FFFFFF` |
| secondary / onSecondary | `#666666` / `#FFFFFF` | `#999999` / `#000000` |
| secondaryContainer / onSecondaryContainer (secondary buttons) | `#E0E0E0` / `#000000` | `#333333` / `#FFFFFF` |
| background, surface / onSurface | `#FFFFFF` / `#000000` | `#000000` / `#FFFFFF` |
| cardSurface (section cards, bottom sheets) | `#F2F2F2` | `#1A1A1A` |
| surfaceContainerLow / surfaceContainer / surfaceContainerHigh | `#F7F7F7` / `#F2F2F2` / `#E0E0E0` | `#0D0D0D` / `#1A1A1A` / `#333333` |
| surfaceVariant / onSurfaceVariant | `#F2F2F2` / `#666666` | `#1A1A1A` / `#999999` |
| outline (switch track off) / outlineVariant (hairlines) | `#8A8A8A` / `#DADADA` | `#666666` / `#2E2E2E` |
| error / onError | `#C62828` / `#FFFFFF` | `#FF6E66` / `#000000` |

## Components

- Section labels are small, all caps, grey; each section is one borderless card with hairline dividers.
- Row titles 15 sp, summaries 12 sp, grey 20 dp icons, values right-aligned in grey.
- Switch: on = white track, black thumb; off = grey track, white thumb.
- **Picking one option from a list always opens a bottom sheet** (`SettingsChoiceRow` → `SettingsChoiceSheet`),
  as in tiny-firewall's exit-node picker: radio per option, optional second line, unavailable options faded
  and not selectable, tap applies and closes. No dropdowns, spinners or dialog radio lists.

## Spacing (`S`)

| Token | Value |
|---|---|
| xxs | 2 |
| xs | 4 |
| sm | 8 |
| md | 12 |
| lg | 16 |
| xl | 24 |
| xxl | 32 |
| touchTarget | 48 |
| cornerRadius | 16 |
| borderWidth | 1 |
| icon (row icons) | 20 |
| appIcon | 64 |

Values are dp on Android and points on iOS.

## Type

System fonts with Material 3's default type scale. No bundled fonts.
