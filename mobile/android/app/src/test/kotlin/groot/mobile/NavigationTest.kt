package groot.mobile

import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.theme.GrootTheme
import groot.mobile.navigation.AppNavHost
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NavigationTest {
    @get:Rule
    val compose = createComposeRule()

    private fun setApp() = compose.setContent {
        GrootTheme {
            AppNavHost(
                themeModeStore = fakeThemeModeStore(),
                appPreferences = fakeAppPreferences(),
                buildInfo = { fakeBuildInfo },
            )
        }
    }

    @Test
    fun `home opens settings, settings opens about, and back returns`() {
        setApp()
        compose.onNodeWithText("Your app starts here.").assertExists()
        compose.onNodeWithText("About").assertDoesNotExist()

        compose.onNodeWithContentDescription("Settings").performClick()
        compose.onNodeWithText("Intro on Home").assertExists()
        compose.onNodeWithText("System default").assertExists()

        compose.onNode(hasText("About") and hasClickAction()).performScrollTo().performClick()
        compose.onNodeWithText("3a4fd6d").assertExists()

        compose.onNodeWithContentDescription("Navigate up").performClick()
        compose.onNodeWithText("Intro on Home").assertExists()
    }

    @Test
    fun `home opens the components screen and back returns`() {
        setApp()
        compose.onNodeWithText("Browse components").performClick()
        compose.onNodeWithText("Components").assertExists()
        compose.onNodeWithText("Route through").assertExists()

        compose.onNodeWithContentDescription("Navigate up").performClick()
        compose.onNodeWithText("Your app starts here.").assertExists()
    }
}
