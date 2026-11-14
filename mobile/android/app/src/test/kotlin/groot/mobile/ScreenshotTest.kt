package groot.mobile

import androidx.compose.runtime.Composable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.github.takahirom.roborazzi.captureRoboImage
import com.github.takahirom.roborazzi.captureScreenRoboImage
import groot.kit.theme.GrootTheme
import groot.kit.theme.ThemeMode
import groot.mobile.feature.about.AboutScreen
import groot.mobile.feature.components.ComponentsScreen
import groot.mobile.feature.home.HomeScreen
import groot.mobile.feature.settings.SettingsScreen
import groot.mobile.ui.brand.BrandPalette
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Renders the reference screens to PNGs for review: `pnpm screenshots`
 * (`./gradlew :app:recordRoborazziDebug`) writes them to app/build/outputs/roborazzi.
 * Not a golden-image gate (RFC 0001 Q9); a normal test run only checks they render.
 * Settings uses a taller window so the whole screen fits in one image.
 */
@RunWith(AndroidJUnit4::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w412dp-h915dp-xxhdpi")
class ScreenshotTest {
    @get:Rule
    val compose = createComposeRule()

    @Test fun homeLight() = capture("home_light", ThemeMode.Light) { Home(ThemeMode.Light) }

    @Test fun homeDark() = capture("home_dark", ThemeMode.Dark) { Home(ThemeMode.Dark) }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun settingsLight() = capture("settings_light", ThemeMode.Light) { Settings(ThemeMode.Light) }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun settingsDark() = capture("settings_dark", ThemeMode.Dark) { Settings(ThemeMode.Dark) }

    @Test
    fun settingsThemePickerLight() =
        captureScreen("settings_theme_picker_light", ThemeMode.Light, before = { compose.onNodeWithText("Theme").performClick() }) {
            Settings(ThemeMode.Light)
        }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun componentsLight() = capture("components_light", ThemeMode.Light) { Components() }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun componentsDark() = capture("components_dark", ThemeMode.Dark) { Components() }

    @Test
    fun componentsPickerDark() =
        captureScreen("components_picker_dark", ThemeMode.Dark, before = { compose.onNodeWithText("Route through").performClick() }) {
            Components()
        }

    @Test fun aboutLight() = capture("about_light", ThemeMode.Light) { About() }

    @Test fun aboutDark() = capture("about_dark", ThemeMode.Dark) { About() }

    private fun capture(name: String, mode: ThemeMode, before: () -> Unit = {}, content: @Composable () -> Unit) {
        compose.setContent { GrootTheme(mode = mode, palette = BrandPalette, content = content) }
        before()
        compose.waitForIdle()
        compose.onRoot().captureRoboImage("build/outputs/roborazzi/$name.png")
    }

    /** Captures every window, so a bottom sheet (its own dialog window) is in the image. */
    @OptIn(com.github.takahirom.roborazzi.ExperimentalRoborazziApi::class)
    private fun captureScreen(name: String, mode: ThemeMode, before: () -> Unit = {}, content: @Composable () -> Unit) {
        compose.setContent { GrootTheme(mode = mode, palette = BrandPalette, content = content) }
        before()
        compose.mainClock.advanceTimeBy(SHEET_SETTLE_MS)
        compose.waitForIdle()
        captureScreenRoboImage("build/outputs/roborazzi/$name.png")
    }

    @Composable
    private fun Home(@Suppress("UNUSED_PARAMETER") mode: ThemeMode) = HomeScreen(
        showHomeIntro = true,
        onOpenComponents = {},
        onOpenSettings = {},
    )

    @Composable
    private fun Components() = ComponentsScreen(onNavigateUp = {}, onOpenSettings = {})

    @Composable
    private fun Settings(mode: ThemeMode) = SettingsScreen(
        themeMode = mode,
        onThemeModeChange = {},
        showHomeIntro = true,
        onShowHomeIntroChange = {},
        onResetData = {},
        buildInfo = fakeBuildInfo,
        onNavigateUp = {},
        onOpenAbout = {},
    )

    @Composable
    private fun About() = AboutScreen(buildInfo = fakeBuildInfo, onNavigateUp = {})
}

private const val SHEET_SETTLE_MS = 1_000L
