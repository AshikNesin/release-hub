package groot.mobile.data.hub

import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLEncoder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** The hub calls the app makes. A fake stands in for it in tests. */
interface HubApi {
    /** Apps with an android platform. Needs the bearer token. */
    suspend fun listApps(endpoint: HubEndpoint): List<HubApp>

    /** Newest direct-channel release, or null when the app has none yet. Public on the hub. */
    suspend fun latest(endpoint: HubEndpoint, slug: String): HubManifest?

    /** Release history, newest first. Needs the bearer token. */
    suspend fun releases(endpoint: HubEndpoint, slug: String): List<HubRelease>
}

/** [HubApi] over HttpURLConnection and org.json: no networking library, nothing added to the download size. */
class HttpHubApi : HubApi {
    override suspend fun listApps(endpoint: HubEndpoint): List<HubApp> =
        HubJson.parseApps(get(endpoint, "/api/apps", auth = true))

    override suspend fun latest(endpoint: HubEndpoint, slug: String): HubManifest? = try {
        HubJson.parseManifest(get(endpoint, "/api/apps/${enc(slug)}/android/manifest?channel=${HubChannels.DIRECT}", auth = false))
    } catch (e: HubException) {
        if (e.status == HttpURLConnection.HTTP_NOT_FOUND) null else throw e
    }

    override suspend fun releases(endpoint: HubEndpoint, slug: String): List<HubRelease> =
        HubJson.parseReleases(get(endpoint, "/api/apps/${enc(slug)}/android/releases", auth = true))

    private suspend fun get(endpoint: HubEndpoint, path: String, auth: Boolean): String = withContext(Dispatchers.IO) {
        val connection = try {
            URL(endpoint.baseUrl + path).openConnection() as HttpURLConnection
        } catch (e: IOException) {
            throw HubException("Can't reach ${endpoint.baseUrl}", cause = e)
        }
        try {
            connection.connectTimeout = TIMEOUT_MS
            connection.readTimeout = TIMEOUT_MS
            connection.setRequestProperty("Accept", "application/json")
            if (auth) connection.setRequestProperty("Authorization", "Bearer ${endpoint.token}")
            val status = connection.responseCode
            if (status !in 200..299) {
                val detail = connection.errorStream?.bufferedReader()?.use { it.readText() }?.take(200)?.trim().orEmpty()
                throw HubException(messageFor(status, detail), status)
            }
            connection.inputStream.bufferedReader().use { it.readText() }
        } catch (e: IOException) {
            throw HubException("Can't reach ${endpoint.baseUrl}: ${e.message ?: e.javaClass.simpleName}", cause = e)
        } finally {
            connection.disconnect()
        }
    }

    private fun messageFor(status: Int, detail: String): String = when (status) {
        HttpURLConnection.HTTP_UNAUTHORIZED -> "The hub rejected the API token"
        HttpURLConnection.HTTP_NOT_FOUND -> "Not found on this hub" + if (detail.isNotEmpty()) ": $detail" else ""
        else -> "Hub error $status" + if (detail.isNotEmpty()) ": $detail" else ""
    }

    private fun enc(s: String) = URLEncoder.encode(s, "UTF-8")

    private companion object {
        const val TIMEOUT_MS = 15_000
    }
}
