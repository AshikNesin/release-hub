package groot.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.mobile.data.releasehub.ReleaseHubApp
import groot.mobile.data.releasehub.ReleaseHubException
import groot.mobile.data.releasehub.ReleaseHubJson
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** The release hub's wire formats, as srv/ writes them (public.go, api_apps.go, api_releases.go). Robolectric: org.json is Android's. */
@RunWith(AndroidJUnit4::class)
class ReleaseHubJsonTest {
    @Test
    fun `apps keep only android platforms, sorted by slug`() {
        val apps = ReleaseHubJson.parseApps(
            """
            [{"slug":"tinyfirewall","platforms":[{"platform":"android","packageName":"io.nesin.tinyfirewall"},
                                                  {"platform":"ios","packageName":"io.nesin.tinyfirewall.ios"}]},
             {"slug":"ios-only","platforms":[{"platform":"ios","packageName":"io.x"}]},
             {"slug":"budget","platforms":[{"platform":"android","packageName":"io.nesin.budget"}]},
             {"slug":"empty","platforms":[]}]
            """,
        )
        assertEquals(listOf(ReleaseHubApp("budget", "io.nesin.budget"), ReleaseHubApp("tinyfirewall", "io.nesin.tinyfirewall")), apps)
    }

    @Test
    fun `manifest reads the new fields and tolerates older hubs without them`() {
        val full = ReleaseHubJson.parseManifest(
            """{"versionCode":142,"versionName":"1.15","apkUrl":"https://hub/a.apk","sha256":"ab","size":10,"notes":"Fixes","createdAt":"2026-11-01T10:00:00Z"}""",
        )
        assertEquals(142L, full.versionCode)
        assertEquals("Fixes", full.notes)
        val old = ReleaseHubJson.parseManifest("""{"versionCode":7,"versionName":"0.7","apkUrl":"https://release-hub/b.apk","sha256":"cd","size":3}""")
        assertEquals("", old.notes)
        assertEquals("", old.createdAt)
    }

    @Test
    fun `releases are newest first and only direct APKs are installable`() {
        val releases = ReleaseHubJson.parseReleases(
            """
            [{"versionCode":10,"versionName":"1.0","channel":"direct","url":"u","sha256":"","size":1,"createdAt":"","fileName":"a-10.apk"},
             {"versionCode":12,"versionName":"1.2","channel":"internal","url":"u","sha256":"","size":1,"createdAt":"","fileName":"a-12.aab"},
             {"versionCode":11,"versionName":"1.1","channel":"direct","url":"u","sha256":"","size":1,"createdAt":"","fileName":"a-11.aab"}]
            """,
        )
        assertEquals(listOf(12L, 11L, 10L), releases.map { it.versionCode })
        assertFalse(releases[0].installable)
        assertFalse(releases[1].installable)
        assertTrue(releases[2].installable)
    }

    @Test(expected = ReleaseHubException::class)
    fun `garbage is a ReleaseHubException, not a crash`() {
        ReleaseHubJson.parseApps("<html>proxy error</html>")
    }
}
