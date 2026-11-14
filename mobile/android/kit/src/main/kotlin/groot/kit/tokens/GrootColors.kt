package groot.kit.tokens

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color

/**
 * Color tokens for one mode (light or dark). Screens read these through `GrootTheme.colors`
 * or `MaterialTheme.colorScheme`, never as literals. Spec: docs/design/DESIGN.md.
 */
@Immutable
public data class GrootColors(
    val primary: Color,
    val onPrimary: Color,
    val primaryContainer: Color,
    val onPrimaryContainer: Color,
    val secondary: Color,
    val onSecondary: Color,
    val secondaryContainer: Color,
    val onSecondaryContainer: Color,
    val background: Color,
    val onBackground: Color,
    val surface: Color,
    val onSurface: Color,
    val surfaceVariant: Color,
    val onSurfaceVariant: Color,
    val surfaceContainerLow: Color,
    val surfaceContainer: Color,
    val surfaceContainerHigh: Color,
    /** Cards (settings sections, bottom sheets): a step off the page in both modes. */
    val cardSurface: Color,
    val outline: Color,
    val outlineVariant: Color,
    val error: Color,
    val onError: Color,
) {
    public companion object {
        /** Mirror image of [Dark]: white page, light-grey cards, black text and controls. */
        public val Light: GrootColors = GrootColors(
            primary = Color(0xFF000000),
            onPrimary = Color(0xFFFFFFFF),
            primaryContainer = Color(0xFFE0E0E0),
            onPrimaryContainer = Color(0xFF000000),
            secondary = Color(0xFF666666),
            onSecondary = Color(0xFFFFFFFF),
            secondaryContainer = Color(0xFFE0E0E0),
            onSecondaryContainer = Color(0xFF000000),
            background = Color(0xFFFFFFFF),
            onBackground = Color(0xFF000000),
            surface = Color(0xFFFFFFFF),
            onSurface = Color(0xFF000000),
            surfaceVariant = Color(0xFFF2F2F2),
            onSurfaceVariant = Color(0xFF666666),
            surfaceContainerLow = Color(0xFFF7F7F7),
            surfaceContainer = Color(0xFFF2F2F2),
            surfaceContainerHigh = Color(0xFFE0E0E0),
            cardSurface = Color(0xFFF2F2F2),
            outline = Color(0xFF8A8A8A),
            outlineVariant = Color(0xFFDADADA),
            error = Color(0xFFC62828),
            onError = Color(0xFFFFFFFF),
        )

        /**
         * tiny-firewall's AMOLED look (RFC 0001 §4.1): true-black page, #1A1A1A cards, white text and
         * controls, grey secondary text. White is the only accent.
         */
        public val Dark: GrootColors = GrootColors(
            primary = Color(0xFFFFFFFF),
            onPrimary = Color(0xFF000000),
            primaryContainer = Color(0xFF333333),
            onPrimaryContainer = Color(0xFFFFFFFF),
            secondary = Color(0xFF999999),
            onSecondary = Color(0xFF000000),
            secondaryContainer = Color(0xFF333333),
            onSecondaryContainer = Color(0xFFFFFFFF),
            background = Color(0xFF000000),
            onBackground = Color(0xFFFFFFFF),
            surface = Color(0xFF000000),
            onSurface = Color(0xFFFFFFFF),
            surfaceVariant = Color(0xFF1A1A1A),
            onSurfaceVariant = Color(0xFF999999),
            surfaceContainerLow = Color(0xFF0D0D0D),
            surfaceContainer = Color(0xFF1A1A1A),
            surfaceContainerHigh = Color(0xFF333333),
            cardSurface = Color(0xFF1A1A1A),
            outline = Color(0xFF666666),
            outlineVariant = Color(0xFF2E2E2E),
            error = Color(0xFFFF6E66),
            onError = Color(0xFF000000),
        )
    }
}

/** Light + dark tokens passed to `GrootTheme`. Children brand their app by passing their own palette. */
@Immutable
public data class GrootPalette(
    val light: GrootColors,
    val dark: GrootColors,
) {
    public companion object {
        public val Default: GrootPalette = GrootPalette(light = GrootColors.Light, dark = GrootColors.Dark)
    }
}
