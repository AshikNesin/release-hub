package groot.mobile.data.catalog

import groot.mobile.data.InstalledApp
import groot.mobile.data.hub.HubApp
import groot.mobile.data.hub.HubManifest

/** Where an app stands on this device against the hub's newest direct release. */
enum class AppStatus {
    UpdateAvailable,
    NotInstalled,
    UpToDate,

    /** Installed build is newer than the hub's (e.g. a local debug build). */
    AheadOfHub,

    /** The hub has no direct-channel release for it yet. */
    NoRelease,
}

/** One hub app as this device sees it. */
data class CatalogEntry(
    val app: HubApp,
    val installed: InstalledApp?,
    val latest: HubManifest?,
    /** True for Release Hub's own entry: installing it updates this app. */
    val isSelf: Boolean = false,
    /** Why the latest release couldn't be read, if it couldn't. */
    val error: String? = null,
) {
    val slug: String get() = app.slug
    val packageName: String get() = app.packageName
    val title: String get() = installed?.label ?: app.slug

    val status: AppStatus
        get() {
            val latest = latest ?: return AppStatus.NoRelease
            val installed = installed ?: return AppStatus.NotInstalled
            return when {
                installed.versionCode < latest.versionCode -> AppStatus.UpdateAvailable
                installed.versionCode > latest.versionCode -> AppStatus.AheadOfHub
                else -> AppStatus.UpToDate
            }
        }

    companion object {
        /** Updates first, then not installed, installed, ahead, and apps with nothing to install; by title within each. */
        val ORDER: Comparator<CatalogEntry> = compareBy<CatalogEntry> { it.status.ordinal }.thenBy { it.title.lowercase() }
    }
}
