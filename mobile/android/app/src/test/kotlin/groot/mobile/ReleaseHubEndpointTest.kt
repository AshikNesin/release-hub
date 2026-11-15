package groot.mobile

import groot.mobile.data.releasehub.ReleaseHubEndpoint
import groot.mobile.install.InstallException
import groot.mobile.install.verify
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseHubEndpointTest {
    @Test
    fun `typed URLs are normalized`() {
        assertEquals("https://release-hub.example.com", ReleaseHubEndpoint.normalizeUrl(" release-hub.example.com/ "))
        assertEquals("http://192.168.1.5:9100", ReleaseHubEndpoint.normalizeUrl("http://192.168.1.5:9100"))
        assertEquals("https://release-hub.example.com/rh", ReleaseHubEndpoint.normalizeUrl("https://release-hub.example.com/rh/"))
        assertNull(ReleaseHubEndpoint.normalizeUrl(""))
        assertNull(ReleaseHubEndpoint.normalizeUrl("ftp://release-hub.example.com"))
        assertNull(ReleaseHubEndpoint.normalizeUrl("https://"))
    }

    @Test
    fun `the token goes only to the release hub itself`() {
        val releaseHub = ReleaseHubEndpoint("https://release-hub.example.com", "rh_x")
        assertTrue(releaseHub.isReleaseHubUrl("https://release-hub.example.com/artifacts/a/android/a.apk"))
        assertFalse(releaseHub.isReleaseHubUrl("https://bucket.s3.amazonaws.com/a.apk"))
        assertFalse(releaseHub.isReleaseHubUrl("http://release-hub.example.com/api/apps"))
        assertFalse(releaseHub.isReleaseHubUrl("https://release-hub.example.com:8443/api/apps"))
    }

    @Test
    fun `downloads are checked against the manifest`() {
        val release = manifest(5).copy(size = 4, sha256 = "AB12")
        verify(release, size = 4, sha256 = "ab12")
        assertThrows { verify(release, size = 3, sha256 = "ab12") }
        assertThrows { verify(release, size = 4, sha256 = "ffff") }
        // Hubs that omit size or hash skip that check.
        verify(manifest(5), size = 99, sha256 = "anything")
    }

    private fun assertThrows(block: () -> Unit) {
        try {
            block()
        } catch (_: InstallException) {
            return
        }
        throw AssertionError("expected InstallException")
    }
}
