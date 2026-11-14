package groot.kit.theme

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat
import groot.kit.tokens.GrootColors
import groot.kit.tokens.GrootPalette
import groot.kit.tokens.GrootSpacing

private val LocalGrootColors = staticCompositionLocalOf { GrootColors.Light }
private val LocalGrootSpacing = staticCompositionLocalOf { GrootSpacing.Default }
private val LocalGrootIsDark = staticCompositionLocalOf { false }

/**
 * Applies kit tokens to everything inside it, as a Material 3 theme plus `GrootTheme.colors` /
 * `GrootTheme.spacing`. Also sets system-bar icon contrast for edge-to-edge windows.
 *
 * Brand an app by passing its own [palette] and [spacing] from app code; don't edit kit tokens.
 */
@Composable
public fun GrootTheme(
    mode: ThemeMode = ThemeMode.System,
    palette: GrootPalette = GrootPalette.Default,
    spacing: GrootSpacing = GrootSpacing.Default,
    content: @Composable () -> Unit,
) {
    val dark = when (mode) {
        ThemeMode.System -> isSystemInDarkTheme()
        ThemeMode.Light -> false
        ThemeMode.Dark -> true
    }
    val colors = if (dark) palette.dark else palette.light

    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = view.context.findActivity()?.window ?: return@SideEffect
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = !dark
                isAppearanceLightNavigationBars = !dark
            }
        }
    }

    CompositionLocalProvider(
        LocalGrootColors provides colors,
        LocalGrootSpacing provides spacing,
        LocalGrootIsDark provides dark,
    ) {
        MaterialTheme(colorScheme = colors.toColorScheme(dark), content = content)
    }
}

/** Read access to the current kit tokens. */
public object GrootTheme {
    public val colors: GrootColors
        @Composable @ReadOnlyComposable
        get() = LocalGrootColors.current

    public val spacing: GrootSpacing
        @Composable @ReadOnlyComposable
        get() = LocalGrootSpacing.current

    public val isDark: Boolean
        @Composable @ReadOnlyComposable
        get() = LocalGrootIsDark.current
}

/** Shorthand for `GrootTheme.spacing`, e.g. `Modifier.padding(S.lg)`. */
public val S: GrootSpacing
    @Composable @ReadOnlyComposable
    get() = LocalGrootSpacing.current

internal fun GrootColors.toColorScheme(dark: Boolean): ColorScheme {
    val base = if (dark) darkColorScheme() else lightColorScheme()
    return base.copy(
        primary = primary,
        onPrimary = onPrimary,
        primaryContainer = primaryContainer,
        onPrimaryContainer = onPrimaryContainer,
        secondary = secondary,
        onSecondary = onSecondary,
        secondaryContainer = secondaryContainer,
        onSecondaryContainer = onSecondaryContainer,
        background = background,
        onBackground = onBackground,
        surface = surface,
        onSurface = onSurface,
        surfaceVariant = surfaceVariant,
        onSurfaceVariant = onSurfaceVariant,
        surfaceContainerLowest = background,
        surfaceContainerLow = surfaceContainerLow,
        surfaceContainer = surfaceContainer,
        surfaceContainerHigh = surfaceContainerHigh,
        surfaceContainerHighest = surfaceContainerHigh,
        surfaceBright = surfaceContainerHigh,
        surfaceDim = background,
        outline = outline,
        outlineVariant = outlineVariant,
        error = error,
        onError = onError,
    )
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
