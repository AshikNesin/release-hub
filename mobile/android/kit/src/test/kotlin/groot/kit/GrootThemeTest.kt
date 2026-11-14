package groot.kit

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.theme.GrootTheme
import groot.kit.theme.S
import groot.kit.theme.ThemeMode
import groot.kit.tokens.GrootColors
import groot.kit.tokens.GrootPalette
import groot.kit.tokens.GrootSpacing
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config

@RunWith(AndroidJUnit4::class)
class GrootThemeTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `explicit Dark applies dark tokens`() {
        var background = Color.Unspecified
        var dark = false
        compose.setContent {
            GrootTheme(mode = ThemeMode.Dark) {
                background = MaterialTheme.colorScheme.background
                dark = GrootTheme.isDark
            }
        }
        assertEquals(GrootColors.Dark.background, background)
        assertTrue(dark)
    }

    @Test
    @Config(qualifiers = "+night")
    fun `System follows a dark device`() {
        var background = Color.Unspecified
        compose.setContent {
            GrootTheme(mode = ThemeMode.System) { background = MaterialTheme.colorScheme.background }
        }
        assertEquals(GrootColors.Dark.background, background)
    }

    @Test
    @Config(qualifiers = "+night")
    fun `explicit Light overrides a dark device`() {
        var dark = true
        compose.setContent { GrootTheme(mode = ThemeMode.Light) { dark = GrootTheme.isDark } }
        assertFalse(dark)
    }

    @Test
    fun `app palette and spacing reach content`() {
        val brand = GrootColors.Light.copy(primary = Color(0xFF6750A4))
        var primary = Color.Unspecified
        var lg: Dp = 0.dp
        compose.setContent {
            GrootTheme(
                mode = ThemeMode.Light,
                palette = GrootPalette(light = brand, dark = GrootColors.Dark),
                spacing = GrootSpacing(lg = 20.dp),
            ) {
                primary = MaterialTheme.colorScheme.primary
                lg = S.lg
            }
        }
        assertEquals(Color(0xFF6750A4), primary)
        assertEquals(20.dp, lg)
    }
}
