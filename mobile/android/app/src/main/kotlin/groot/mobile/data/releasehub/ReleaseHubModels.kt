package groot.mobile.data.releasehub

/** An app registered on the release hub with an android platform (`GET /api/apps`). */
data class ReleaseHubApp(val slug: String, val packageName: String)

/**
 * The newest release on a channel (`GET /api/apps/{slug}/manifest?channel=direct`).
 * [notes] and [createdAt] are empty on release  release hubs older than the Android app.
 */
data class ReleaseHubManifest(
    val versionCode: Long,
    val versionName: String,
    val apkUrl: String,
    val sha256: String,
    val size: Long,
    val notes: String = "",
    val createdAt: String = "",
)

/** One row of an app's release history (`GET /api/apps/{slug}/releases`). */
data class ReleaseHubRelease(
    val versionCode: Long,
    val versionName: String,
    val channel: String,
    val url: String,
    val sha256: String,
    val size: Long,
    val createdAt: String,
    val fileName: String = "",
    val notes: String = "",
) {
    /** Only direct-channel APKs install on a device; .aab uploads go to Play. */
    val installable: Boolean
        get() = channel == ReleaseHubChannels.DIRECT && !fileName.endsWith(".aab", ignoreCase = true)

    fun toManifest(): ReleaseHubManifest = ReleaseHubManifest(versionCode, versionName, url, sha256, size, notes, createdAt)
}

object ReleaseHubChannels {
    /** Direct APK distribution: the channel the release hub serves update manifests for. */
    const val DIRECT = "direct"
}

/** A failed hub call, with a message fit for the screen. */
class ReleaseHubException(message: String, val status: Int = 0, cause: Throwable? = null) : Exception(message, cause) {
    val unauthorized: Boolean get() = status == 401
}
