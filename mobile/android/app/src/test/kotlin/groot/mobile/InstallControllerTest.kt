package groot.mobile

import android.content.Intent
import android.content.pm.PackageInstaller
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.mobile.data.releasehub.ReleaseHubApp
import groot.mobile.install.InstallException
import groot.mobile.install.InstallResult
import groot.mobile.install.InstallState
import kotlinx.coroutines.async
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class InstallControllerTest {
    private suspend fun TestScope.appWithUpdate(): TestApp {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.api.apps = listOf(ReleaseHubApp("demo", "io.demo"))
        app.api.manifests = mapOf("demo" to manifest(2))
        app.installed.install("io.demo", 1, label = "Demo")
        app.catalog.refresh()
        return app
    }

    @Test
    fun `an update downloads, commits a session and finishes on success`() = runTest {
        val app = appWithUpdate()
        val entry = app.catalog.entry("demo")!!
        val result = async { app.controller.install(entry) }
        runCurrent()
        assertEquals(InstallState.Installing, app.tracker.get("io.demo"))
        assertEquals("io.demo", app.sessions.commits.single().second)

        app.installed.install("io.demo", 2, label = "Demo")
        app.controller.onSessionResult(app.sessions.lastSession(), "io.demo", PackageInstaller.STATUS_SUCCESS, null, null)

        assertEquals(InstallResult.Installed, result.await())
        assertNull(app.tracker.get("io.demo"))
        assertEquals(2L, app.catalog.entry("demo")?.installed?.versionCode)
        assertTrue(app.ui.events.isEmpty())
    }

    @Test
    fun `confirmation opens on screen in the foreground and then waits for the answer`() = runTest {
        val app = appWithUpdate()
        val result = async { app.controller.install(app.catalog.entry("demo")!!) }
        runCurrent()
        val session = app.sessions.lastSession()

        app.controller.onSessionResult(session, "io.demo", PackageInstaller.STATUS_PENDING_USER_ACTION, null, Intent("confirm"))
        runCurrent()
        assertEquals(listOf("confirm-screen"), app.ui.events)
        assertEquals(InstallState.WaitingForUser, app.tracker.get("io.demo"))
        assertTrue(result.isActive)

        app.controller.onSessionResult(session, "io.demo", PackageInstaller.STATUS_FAILURE_ABORTED, null, null)
        assertEquals(InstallResult.Cancelled, result.await())
        assertNull(app.tracker.get("io.demo"))
    }

    @Test
    fun `background installs don't wait on the person - the confirmation becomes a notification`() = runTest {
        val app = appWithUpdate()
        app.ui.foreground = false
        val result = async { app.controller.install(app.catalog.entry("demo")!!, background = true) }
        runCurrent()

        app.controller.onSessionResult(app.sessions.lastSession(), "io.demo", PackageInstaller.STATUS_PENDING_USER_ACTION, null, Intent("confirm"))

        assertEquals(InstallResult.WaitingForUser, result.await())
        assertEquals(listOf("confirm-notification:io.demo"), app.ui.events)
    }

    @Test
    fun `a failed download is reported and nothing is committed`() = runTest {
        val app = appWithUpdate()
        app.downloader.failure = InstallException("Checksum mismatch")

        val result = app.controller.install(app.catalog.entry("demo")!!)

        assertEquals(InstallResult.Failed, result)
        assertEquals(InstallState.Failed("Checksum mismatch"), app.tracker.get("io.demo"))
        assertTrue(app.sessions.commits.isEmpty())
        app.controller.dismissError("io.demo")
        assertNull(app.tracker.get("io.demo"))
    }

    @Test
    fun `a signature conflict explains what to do`() = runTest {
        val app = appWithUpdate()
        val result = async { app.controller.install(app.catalog.entry("demo")!!) }
        runCurrent()
        app.controller.onSessionResult(
            app.sessions.lastSession(), "io.demo", PackageInstaller.STATUS_FAILURE_CONFLICT,
            "INSTALL_FAILED_UPDATE_INCOMPATIBLE: signatures do not match", null,
        )
        assertEquals(InstallResult.Failed, result.await())
        val failed = app.tracker.get("io.demo") as InstallState.Failed
        assertTrue(failed.message, "different key" in failed.message)
    }

    @Test
    fun `without the install permission nothing is downloaded`() = runTest {
        val app = appWithUpdate()
        app.sessions.allowed = false

        assertEquals(InstallResult.NeedsPermission, app.controller.install(app.catalog.entry("demo")!!))
        assertEquals(InstallResult.NeedsPermission, app.controller.install(app.catalog.entry("demo")!!, background = true))

        assertTrue(app.downloader.downloaded.isEmpty())
        assertEquals(listOf("needs-permission"), app.ui.events)
    }
}
