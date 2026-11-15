package groot.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.releasehub.ReleaseHubApp
import groot.mobile.data.releasehub.ReleaseHubException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CatalogRepositoryTest {
    @Test
    fun `entries get a status from installed vs latest, updates first`() = runTest {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.api.apps = listOf(
            ReleaseHubApp("current", "io.current"),
            ReleaseHubApp("fresh", "io.fresh"),
            ReleaseHubApp("stale", "io.stale"),
            ReleaseHubApp("dev", "io.dev"),
            ReleaseHubApp("empty", "io.empty"),
            ReleaseHubApp("release-hub", SELF_PACKAGE),
        )
        app.api.manifests = mapOf("current" to manifest(5), "fresh" to manifest(1), "stale" to manifest(9), "dev" to manifest(3), "release-hub" to manifest(2))
        app.installed.install("io.current", 5)
        app.installed.install("io.stale", 8)
        app.installed.install("io.dev", 4)
        app.installed.install(SELF_PACKAGE, 2)

        val entries = app.catalog.refresh()

        val status = entries.associate { it.slug to it.status }
        assertEquals(AppStatus.UpToDate, status["current"])
        assertEquals(AppStatus.NotInstalled, status["fresh"])
        assertEquals(AppStatus.UpdateAvailable, status["stale"])
        assertEquals(AppStatus.AheadOfReleaseHub, status["dev"])
        assertEquals(AppStatus.NoRelease, status["empty"])
        assertEquals("stale", entries.first().slug)
        assertTrue(entries.single { it.slug == "release-hub" }.isSelf)
        assertTrue(app.catalog.state.value.loaded)
        assertEquals(1_000L, app.settings.updatePrefs.first().lastCheckedAt)
    }

    @Test
    fun `one app's manifest failing doesn't hide the others`() = runTest {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.api.apps = listOf(ReleaseHubApp("ok", "io.ok"), ReleaseHubApp("broken", "io.broken"))
        app.api.manifests = mapOf("ok" to manifest(1))
        app.api.manifestFailures = mapOf("broken" to ReleaseHubException("Release hub error 500", 500))

        val entries = app.catalog.refresh()

        assertEquals(AppStatus.NotInstalled, entries.single { it.slug == "ok" }.status)
        assertEquals("Release hub error 500", entries.single { it.slug == "broken" }.error)
    }

    @Test
    fun `a rejected token fails the refresh and is kept in state`() = runTest {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.api.failure = ReleaseHubException("The release hub rejected the API token", 401)
        try {
            app.catalog.refresh()
            fail("expected ReleaseHubException")
        } catch (e: ReleaseHubException) {
            assertTrue(e.unauthorized)
        }
        assertEquals("The release hub rejected the API token", app.catalog.state.value.error)
        assertFalse(app.catalog.state.value.refreshing)
    }

    @Test
    fun `installed versions refresh without a release hub call`() = runTest {
        val app = TestApp(backgroundScope)
        app.settings.connect(fakeEndpoint)
        app.api.apps = listOf(ReleaseHubApp("a", "io.a"))
        app.api.manifests = mapOf("a" to manifest(3))
        app.catalog.refresh()
        app.api.failure = ReleaseHubException("offline")

        app.installed.install("io.a", 3)
        app.catalog.refreshInstalled("io.a")

        assertEquals(AppStatus.UpToDate, app.catalog.entry("a")?.status)
    }
}
