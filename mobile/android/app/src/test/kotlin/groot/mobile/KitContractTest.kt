package groot.mobile

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.test.assertLeftPositionInRootIsEqualTo
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.theme.GrootTheme
import groot.kit.theme.ThemeMode
import groot.kit.tokens.GrootSpacing
import groot.mobile.feature.home.HOME_CONTENT_TAG
import groot.mobile.feature.home.HomeScreen
import groot.mobile.ui.brand.BrandPalette
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.GraphicsMode

/**
 * Kit contract test (RFC 0001 §5.4): an app screen rendered inside GrootTheme picks up the
 * theme's colors and spacing. It asserts no layout or copy, so product changes don't break it;
 * it fails only if a kit sync stops the theme from applying.
 */
@RunWith(AndroidJUnit4::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class KitContractTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `light theme background reaches the screen`() = assertBackground(ThemeMode.Light, BrandPalette.light.background)

    @Test
    fun `dark theme background reaches the screen`() = assertBackground(ThemeMode.Dark, BrandPalette.dark.background)

    @Test
    fun `theme spacing reaches the screen`() {
        compose.setContent {
            GrootTheme(mode = ThemeMode.Light, palette = BrandPalette, spacing = GrootSpacing(xl = 28.dp)) { Home() }
        }
        compose.onNodeWithTag(HOME_CONTENT_TAG).assertLeftPositionInRootIsEqualTo(28.dp)
    }

    private fun assertBackground(mode: ThemeMode, expected: Color) {
        compose.setContent { GrootTheme(mode = mode, palette = BrandPalette) { Home() } }
        val pixel = compose.onRoot().captureToImage().toPixelMap()[2, 2]
        assertEquals(expected, pixel)
    }

    @androidx.compose.runtime.Composable
    private fun Home() = HomeScreen(showHomeIntro = true, onOpenComponents = {}, onOpenSettings = {})
}
