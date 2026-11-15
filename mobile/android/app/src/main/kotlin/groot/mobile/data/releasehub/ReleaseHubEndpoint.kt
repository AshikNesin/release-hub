package groot.mobile.data.releasehub

import java.net.URI

/** Where the release hub lives and how to reach it. [token] is a bearer token (`rh_…`) created in the release hub's UI. */
data class ReleaseHubEndpoint(val baseUrl: String, val token: String) {
    /** True for a URL on the release hub itself, where the bearer token may be sent; never for S3/CDN artifact URLs. */
    fun isReleaseHubUrl(url: String): Boolean = runCatching {
        val base = URI(baseUrl)
        val other = URI(url)
        base.scheme.equals(other.scheme, ignoreCase = true) &&
            base.host.equals(other.host, ignoreCase = true) &&
            base.port == other.port
    }.getOrDefault(false)

    companion object {
        /**
         * Normalizes what a person types: trims, adds https:// when no scheme is given, drops a trailing
         * slash. Returns null for anything that isn't an http(s) URL with a host.
         */
        fun normalizeUrl(input: String): String? {
            val trimmed = input.trim()
            if (trimmed.isEmpty()) return null
            // Look for the scheme before trimming slashes, so "https://" alone stays a URL without a host.
            val withScheme = (if ("://" in trimmed) trimmed else "https://$trimmed").trimEnd('/')
            val uri = runCatching { URI(withScheme) }.getOrNull() ?: return null
            if (uri.scheme?.lowercase() !in setOf("https", "http") || uri.host.isNullOrEmpty()) return null
            return withScheme
        }
    }
}
