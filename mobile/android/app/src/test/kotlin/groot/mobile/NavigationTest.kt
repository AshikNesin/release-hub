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
import groot.mobile.data.releasehub.ReleaseHubApp
import groot.mobile.navigation.AppNavHost
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.runBlocking
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NavigationTest {
    @get:Rule
    val compose = createComposeRule()

    private val app = TestApp(CoroutineScope(SupervisorJob()))

    private fun setApp() = compose.setContent { GrootTheme { AppNavHost(graph = app.graph) } }

    @Test
    fun `without a release hub, home offers to connect, settings and about open and back returns`() {
        setApp()
        compose.onNodeWithText("Your own app store").assertExists()

        compose.onNodeWithText("Connect to a release hub").performClick()
        compose.onNodeWithText("Release hub URL").assertExists()
        compose.onNodeWithContentDescription("Navigate up").performClick()

        compose.onNodeWithContentDescription("Settings").performClick()
        compose.onNodeWithText("Auto-update apps").assertExists()
        compose.onNodeWithText("Not connected").assertExists()

        compose.onNode(hasText("About") and hasClickAction()).performScrollTo().performClick()
        compose.onNodeWithText("3a4fd6d").assertExists()

        compose.onNodeWithContentDescription("Navigate up").performClick()
        compose.onNodeWithText("Auto-update apps").assertExists()
    }

    @Test
    fun `connected, home lists release hub apps by status and opens an app's page`() {
        runBlocking { app.settings.connect(fakeEndpoint) }
        app.api.apps = listOf(ReleaseHubApp("demo", "io.demo"), ReleaseHubApp("notes", "io.notes"))
        app.api.manifests = mapOf("demo" to manifest(2, notes = "Faster sync"), "notes" to manifest(1))
        app.api.releases = mapOf("demo" to listOf(release(2), release(1)))
        app.installed.install("io.demo", 1, label = "Demo")
        setApp()

        compose.waitUntil(5_000) { compose.onAllNodes(hasText("Demo")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText("UPDATES").assertExists()
        compose.onNodeWithText("NOT INSTALLED").assertExists()
        compose.onNodeWithText("Update").assertExists()
        compose.onNodeWithText("Install").assertExists()

        compose.onNodeWithText("Demo").performClick()
        compose.onNodeWithText("Latest on release hub").assertExists()
        compose.onNodeWithText("Faster sync").assertExists()
        compose.onNodeWithText("RELEASE HISTORY").performScrollTo().assertExists()

        compose.onNodeWithContentDescription("Navigate up").performClick()
        compose.onNodeWithText("UPDATES").assertExists()
    }
}
