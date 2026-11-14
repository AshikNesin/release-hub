package groot.kit

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import groot.kit.tokens.GrootColors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class GrootColorsTest {
    private val modes = mapOf(
        "Light" to GrootColors.Light,
        "Dark" to GrootColors.Dark,
    )

    @Test
    fun `text on every surface meets WCAG AA contrast in every mode`() {
        modes.forEach { (name, c) ->
            val pairs = mapOf(
                "onPrimary/primary" to (c.onPrimary to c.primary),
                "onPrimaryContainer/primaryContainer" to (c.onPrimaryContainer to c.primaryContainer),
                "onSecondaryContainer/secondaryContainer" to (c.onSecondaryContainer to c.secondaryContainer),
                "onBackground/background" to (c.onBackground to c.background),
                "onSurface/surface" to (c.onSurface to c.surface),
                "onSurface/surfaceContainer" to (c.onSurface to c.surfaceContainer),
                "onSurfaceVariant/surfaceContainer" to (c.onSurfaceVariant to c.surfaceContainer),
                "primary/surfaceContainer" to (c.primary to c.surfaceContainer),
                "onSurface/cardSurface" to (c.onSurface to c.cardSurface),
                "onSurfaceVariant/cardSurface" to (c.onSurfaceVariant to c.cardSurface),
                "primary/cardSurface" to (c.primary to c.cardSurface),
                "error/cardSurface" to (c.error to c.cardSurface),
                "onError/error" to (c.onError to c.error),
            )
            pairs.forEach { (pair, colors) ->
                val ratio = contrast(colors.first, colors.second)
                assertTrue("$name $pair contrast is $ratio, needs >= 4.5", ratio >= 4.5)
            }
        }
    }

    @Test
    fun `dark mode is tiny-firewall's true black with grey cards and a white accent`() {
        val d = GrootColors.Dark
        assertEquals(Color.Black, d.background)
        assertEquals(Color.Black, d.surface)
        assertEquals(Color(0xFF1A1A1A), d.cardSurface)
        assertEquals(Color.White, d.primary)
        assertEquals(Color.White, d.onSurface)
        assertEquals(Color(0xFF999999), d.onSurfaceVariant)
    }

    @Test
    fun `light mode mirrors dark`() {
        val l = GrootColors.Light
        assertEquals(Color.White, l.background)
        assertEquals(Color.Black, l.primary)
        assertEquals(Color.Black, l.onSurface)
        assertTrue(l.cardSurface.luminance() < l.background.luminance())
    }

    @Test
    fun `light and dark differ`() {
        assertNotEquals(GrootColors.Light, GrootColors.Dark)
    }

    private fun contrast(a: Color, b: Color): Double {
        val la = a.luminance() + 0.05
        val lb = b.luminance() + 0.05
        return maxOf(la, lb) / minOf(la, lb)
    }
}
