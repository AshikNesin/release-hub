package groot.mobile

import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.UriHandler
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.theme.GrootTheme
import groot.mobile.feature.about.AboutScreen
import groot.mobile.ui.withRef
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MakerLinkTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `ref is added to a bare url and to one that already has a query`() {
        assertEquals("https://nesin.io?ref=io.nesin.groot", withRef("https://nesin.io", "io.nesin.groot"))
        assertEquals("https://nesin.io/apps?x=1&ref=io.nesin.groot", withRef("https://nesin.io/apps?x=1", "io.nesin.groot"))
    }

    @Test
    fun `about footer credits the maker and opens their site with the app id`() {
        var opened: String? = null
        compose.setContent {
            CompositionLocalProvider(LocalUriHandler provides object : UriHandler {
                override fun openUri(uri: String) { opened = uri }
            }) {
                GrootTheme { AboutScreen(buildInfo = fakeBuildInfo, onNavigateUp = {}) }
            }
        }
        compose.onNodeWithText("Built by Nesin Technologies LLP").performScrollTo().performClick()
        assertEquals("https://nesin.io?ref=${BuildConfig.APPLICATION_ID}", opened)
    }
}
