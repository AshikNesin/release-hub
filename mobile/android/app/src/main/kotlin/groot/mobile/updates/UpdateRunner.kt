package groot.mobile.updates

import groot.mobile.data.HubSettings
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.install.InstallController
import groot.mobile.install.InstallResult

/** What one background check did. */
data class UpdateRunSummary(
    val checked: Int,
    val installed: List<String>,
    /** Updates left for the person: auto-update off for them, or they need a confirmation or failed. */
    val pending: List<String>,
)

/**
 * One background update pass: refresh the catalog, install the updates auto-update covers (Release Hub's
 * own update last, since installing it restarts the app), and notify about the rest. Kept free of
 * WorkManager so it's tested with fakes.
 */
class UpdateRunner(
    private val settings: HubSettings,
    private val catalog: CatalogRepository,
    private val installer: InstallController,
    private val notify: (List<String>) -> Unit,
) {
    /** Throws [groot.mobile.data.hub.HubException] when the hub can't be read. */
    suspend fun run(): UpdateRunSummary {
        if (settings.currentEndpoint() == null) return UpdateRunSummary(0, emptyList(), emptyList())
        val prefs = settings.currentUpdatePrefs()
        val entries = catalog.refresh()
        val updates = entries.filter { it.status == AppStatus.UpdateAvailable }
        val (auto, manual) = updates.partition { prefs.autoUpdates(it.slug) }

        val installed = mutableListOf<String>()
        val pending = manual.map(CatalogEntry::title).toMutableList()
        for (entry in auto.sortedBy { it.isSelf }) {
            when (installer.install(entry, background = true)) {
                InstallResult.Installed -> installed += entry.title
                // Confirmation and failure already have their own notification.
                InstallResult.WaitingForUser, InstallResult.Failed -> Unit
                InstallResult.Cancelled, InstallResult.NeedsPermission -> pending += entry.title
            }
        }
        notify(pending)
        return UpdateRunSummary(entries.size, installed, pending)
    }
}
