package groot.mobile

import groot.mobile.data.hub.HubEndpoint
import groot.mobile.install.InstallException
import groot.mobile.install.verify
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HubEndpointTest {
    @Test
    fun `typed URLs are normalized`() {
        assertEquals("https://hub.example.com", HubEndpoint.normalizeUrl(" hub.example.com/ "))
        assertEquals("http://192.168.1.5:9100", HubEndpoint.normalizeUrl("http://192.168.1.5:9100"))
        assertEquals("https://hub.example.com/rh", HubEndpoint.normalizeUrl("https://hub.example.com/rh/"))
        assertNull(HubEndpoint.normalizeUrl(""))
        assertNull(HubEndpoint.normalizeUrl("ftp://hub.example.com"))
        assertNull(HubEndpoint.normalizeUrl("https://"))
    }

    @Test
    fun `the token goes only to the hub itself`() {
        val hub = HubEndpoint("https://hub.example.com", "rh_x")
        assertTrue(hub.isHubUrl("https://hub.example.com/artifacts/a/android/a.apk"))
        assertFalse(hub.isHubUrl("https://bucket.s3.amazonaws.com/a.apk"))
        assertFalse(hub.isHubUrl("http://hub.example.com/api/apps"))
        assertFalse(hub.isHubUrl("https://hub.example.com:8443/api/apps"))
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
