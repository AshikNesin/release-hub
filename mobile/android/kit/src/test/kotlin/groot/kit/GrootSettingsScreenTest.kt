package groot.kit

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.UriHandler
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.buildinfo.BuildInfo
import groot.kit.settings.GrootSettingsScreen
import groot.kit.settings.SettingsAbout
import groot.kit.settings.SettingsSection
import groot.kit.settings.SettingsSwitchRow
import groot.kit.theme.GrootTheme
import groot.kit.theme.ThemeMode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config

@RunWith(AndroidJUnit4::class)
@Config(qualifiers = "w412dp-h1400dp")
class GrootSettingsScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val about = SettingsAbout(
        appName = "Sample",
        buildInfo = BuildInfo(versionName = "1.2.3", versionCode = 45, commit = "abc1234", isDebug = false),
        feedbackEmail = "help@example.com",
        rateUrl = "https://store.example/app",
        makerName = "Example Co",
        makerUrl = "https://example.com",
    )

    private val opened = mutableListOf<String>()
    private val uriHandler = object : UriHandler {
        override fun openUri(uri: String) {
            opened += uri
        }
    }

    private fun setScreen(
        mode: ThemeMode = ThemeMode.System,
        onModeChange: (ThemeMode) -> Unit = {},
        onReset: (() -> Unit)? = null,
        appSwitch: Boolean? = null,
        onAppSwitch: (Boolean) -> Unit = {},
        onOpenAbout: (() -> Unit)? = null,
    ) {
        compose.setContent {
            CompositionLocalProvider(LocalUriHandler provides uriHandler) {
                GrootTheme {
                    GrootSettingsScreen(
                        themeMode = mode,
                        onThemeModeChange = onModeChange,
                        about = about,
                        onResetData = onReset,
                        onOpenAbout = onOpenAbout,
                        privacyNote = "Your data stays here.",
                    ) {
                        if (appSwitch != null) {
                            SettingsSection("App") {
                                SettingsSwitchRow(title = "Sync", checked = appSwitch, onCheckedChange = onAppSwitch)
                            }
                        }
                    }
                }
            }
        }
    }

    @Test
    fun `theme row opens a bottom sheet, picking applies and closes it`() {
        var picked: ThemeMode? = null
        setScreen(mode = ThemeMode.Light, onModeChange = { picked = it })

        compose.onNodeWithText("Theme").performClick()
        compose.onNodeWithText("Match your device setting").assertExists()
        compose.onNodeWithText("Dark").performClick()
        compose.waitForIdle()

        assertEquals(ThemeMode.Dark, picked)
        compose.onNodeWithText("Match your device setting").assertDoesNotExist()
    }

    @Test
    fun `selected theme option is marked selected`() {
        setScreen(mode = ThemeMode.Dark)
        compose.onNodeWithText("Theme").performClick()
        compose.onNode(hasText("Dark") and isSelectable()).assertIsSelected()
    }

    @Test
    fun `reset asks for confirmation before running`() {
        var resets = 0
        setScreen(onReset = { resets++ })

        compose.onNodeWithText("Reset all data").performScrollTo().performClick()
        assertEquals(0, resets)
        compose.onNodeWithText("Cancel").performClick()
        assertEquals(0, resets)

        compose.onNodeWithText("Reset all data").performClick()
        compose.onNodeWithText("Reset").performClick()
        assertEquals(1, resets)
    }

    @Test
    fun `with onOpenAbout, About is a row that opens the about screen`() {
        var opened = 0
        setScreen(onOpenAbout = { opened++ })
        compose.onNodeWithText("1.2.3 (45)").assertDoesNotExist()
        compose.onNode(hasText("About") and hasClickAction()).performScrollTo().performClick()
        assertEquals(1, opened)
    }

    @Test
    fun `rate opens the store`() {
        setScreen()
        compose.onNodeWithText("Rate the app").performScrollTo().performClick()
        assertEquals(listOf("https://store.example/app"), opened)
    }

    @Test
    fun `no reset section without a reset action`() {
        setScreen(onReset = null)
        compose.onNodeWithText("Reset all data").assertDoesNotExist()
    }

    @Test
    fun `about shows version and build, feedback opens a mail draft with the version`() {
        setScreen()
        compose.onNodeWithText("1.2.3 (45)").performScrollTo().assertExists()

        compose.onNodeWithText("Send feedback").performScrollTo().performClick()

        assertEquals(1, opened.size)
        assertTrue(opened[0].startsWith("mailto:help@example.com?subject="))
        assertTrue(opened[0].contains("Sample%20Feedback%20(v1.2.3)"))
    }

    @Test
    fun `footer credits the maker and opens their site`() {
        setScreen()
        compose.onNodeWithText("Built by Example Co").performScrollTo().performClick()
        assertEquals(listOf("https://example.com"), opened)
        compose.onNodeWithText("Your data stays here.").assertExists()
    }

    @Test
    fun `app sections render above the kit sections and their switches toggle`() {
        var toggled: Boolean? = null
        setScreen(appSwitch = false, onAppSwitch = { toggled = it })

        compose.onNodeWithText("Sync").assertIsOff().performClick()
        assertEquals(true, toggled)
    }

    @Test
    fun `switch row reflects checked state`() {
        setScreen(appSwitch = true)
        compose.onNodeWithText("Sync").assertIsOn()
    }
}
