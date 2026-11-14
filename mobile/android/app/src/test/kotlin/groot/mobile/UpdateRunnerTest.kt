package groot.mobile

import android.content.pm.PackageInstaller
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.mobile.data.hub.HubApp
import groot.mobile.updates.UpdateRunner
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class UpdateRunnerTest {
    @Test
    fun `does nothing without a hub`() = runTest {
        val app = TestApp(backgroundScope)
        val notified = mutableListOf<List<String>>()
        val summary = UpdateRunner(app.settings, app.catalog, app.controller) { notified += it }.run()
        assertEquals(0, summary.checked)
        assertTrue(notified.isEmpty())
    }

    @Test
    fun `installs auto-updatable apps, Release Hub itself last, and notifies about the rest`() = runTest {
        val app = TestApp(backgroundScope)
        app.ui.foreground = false
        app.settings.connect(fakeEndpoint)
        app.settings.setAutoUpdateFor("manual", enabled = false)
        app.api.apps = listOf(HubApp("hub", SELF_PACKAGE), HubApp("auto", "io.auto"), HubApp("manual", "io.manual"), HubApp("new", "io.new"))
        app.api.manifests = mapOf("hub" to manifest(3), "auto" to manifest(2), "manual" to manifest(2), "new" to manifest(1))
        app.installed.install(SELF_PACKAGE, 2, label = "Release Hub")
        app.installed.install("io.auto", 1, label = "Auto")
        app.installed.install("io.manual", 1, label = "Manual")

        val notified = mutableListOf<List<String>>()
        val run = async { UpdateRunner(app.settings, app.catalog, app.controller) { notified += it }.run() }
        runCurrent()
        // Silent update of the first app, then of Release Hub itself.
        assertEquals("io.auto", app.sessions.commits.last().second)
        app.controller.onSessionResult(app.sessions.lastSession(), "io.auto", PackageInstaller.STATUS_SUCCESS, null, null)
        runCurrent()
        assertEquals(SELF_PACKAGE, app.sessions.commits.last().second)
        app.controller.onSessionResult(app.sessions.lastSession(), SELF_PACKAGE, PackageInstaller.STATUS_SUCCESS, null, null)

        val summary = run.await()
        assertEquals(listOf("Auto", "Release Hub"), summary.installed)
        assertEquals(listOf("Manual"), summary.pending)
        assertEquals(listOf(listOf("Manual")), notified)
        assertEquals(listOf("installed:io.auto:1.2", "installed:$SELF_PACKAGE:1.3"), app.ui.events)
        // Not-installed apps are never installed in the background.
        assertTrue(app.sessions.commits.none { it.second == "io.new" })
    }

    @Test
    fun `auto-update off leaves every update to the person`() = runTest {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.settings.setAutoUpdate(false)
        app.api.apps = listOf(HubApp("a", "io.a"))
        app.api.manifests = mapOf("a" to manifest(2))
        app.installed.install("io.a", 1, label = "A")

        val notified = mutableListOf<List<String>>()
        val summary = UpdateRunner(app.settings, app.catalog, app.controller) { notified += it }.run()

        assertTrue(app.sessions.commits.isEmpty())
        assertEquals(listOf("A"), summary.pending)
        assertEquals(listOf(listOf("A")), notified)
    }
}
