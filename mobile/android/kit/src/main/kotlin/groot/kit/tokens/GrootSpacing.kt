package groot.kit.tokens

import androidx.compose.runtime.Immutable
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/** Spacing scale. Read it as `S.md` inside `GrootTheme`; never use raw dp values in screens. */
@Immutable
public data class GrootSpacing(
    val xxs: Dp = 2.dp,
    val xs: Dp = 4.dp,
    val sm: Dp = 8.dp,
    val md: Dp = 12.dp,
    val lg: Dp = 16.dp,
    val xl: Dp = 24.dp,
    val xxl: Dp = 32.dp,
    /** Minimum touch target height for rows and buttons. */
    val touchTarget: Dp = 48.dp,
    /** Corner radius for cards. */
    val cornerRadius: Dp = 16.dp,
    /** Leading icon in list rows. */
    val icon: Dp = 20.dp,
    /** App icon on the About screen. */
    val appIcon: Dp = 64.dp,
    /** Hairline for card outlines and row dividers. */
    val borderWidth: Dp = 1.dp,
) {
    public companion object {
        public val Default: GrootSpacing = GrootSpacing()
    }
}
