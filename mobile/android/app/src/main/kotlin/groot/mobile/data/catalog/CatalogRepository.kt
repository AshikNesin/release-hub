package groot.mobile.data.catalog

import groot.mobile.data.ReleaseHubSettings
import groot.mobile.data.InstalledApps
import groot.mobile.data.releasehub.ReleaseHubApi
import groot.mobile.data.releasehub.ReleaseHubEndpoint
import groot.mobile.data.releasehub.ReleaseHubException
import groot.mobile.data.releasehub.ReleaseHubRelease
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit

data class CatalogState(
    val entries: List<CatalogEntry> = emptyList(),
    val refreshing: Boolean = false,
    /** The last refresh's failure, cleared by the next success. */
    val error: String? = null,
    /** True once a refresh has completed, so an empty list means "no apps", not "not loaded". */
    val loaded: Boolean = false,
)

/**
 * The release hub's apps joined with what's installed. One instance per process, shared by every screen and
 * the background worker, so a refresh anywhere shows up everywhere.
 */
class CatalogRepository(
    private val api: ReleaseHubApi,
    private val settings: ReleaseHubSettings,
    private val installedApps: InstalledApps,
    private val selfPackage: String,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private val _state = MutableStateFlow(CatalogState())
    val state: StateFlow<CatalogState> = _state.asStateFlow()

    /** Reloads from the release hub. Returns the fresh entries, or throws [ReleaseHubException] (also recorded in [state]). */
    suspend fun refresh(): List<CatalogEntry> {
        val endpoint = settings.currentEndpoint() ?: throw ReleaseHubException("Not connected to a release hub")
        _state.update { it.copy(refreshing = true) }
        return try {
            val entries = load(endpoint)
            settings.markChecked(clock())
            _state.value = CatalogState(entries = entries, loaded = true)
            entries
        } catch (e: ReleaseHubException) {
            _state.update { it.copy(refreshing = false, error = e.message) }
            throw e
        }
    }

    /** Refreshes and swallows the error (it's in [state]); for pull-to-refresh and screen starts. */
    suspend fun refreshQuietly() {
        runCatching { refresh() }
    }

    /** Re-reads one package's installed version after an install or uninstall, without a release hub round trip. */
    fun refreshInstalled(packageName: String) {
        _state.update { state ->
            state.copy(
                entries = state.entries
                    .map { if (it.packageName == packageName) it.copy(installed = installedApps.get(packageName)) else it }
                    .sortedWith(CatalogEntry.ORDER),
            )
        }
    }

    /** Re-reads every installed version; for returning to the app after an uninstall or an outside install. */
    fun refreshAllInstalled() {
        _state.update { state ->
            state.copy(entries = state.entries.map { it.copy(installed = installedApps.get(it.packageName)) }.sortedWith(CatalogEntry.ORDER))
        }
    }

    fun entry(slug: String): CatalogEntry? = _state.value.entries.firstOrNull { it.slug == slug }

    suspend fun releases(slug: String): List<ReleaseHubRelease> {
        val endpoint = settings.currentEndpoint() ?: throw ReleaseHubException("Not connected to a release hub")
        return api.releases(endpoint, slug)
    }

    /** Forgets everything loaded from the previous release hub. */
    fun clear() {
        _state.value = CatalogState()
    }

    private suspend fun load(endpoint: ReleaseHubEndpoint): List<CatalogEntry> = coroutineScope {
        val apps = api.listApps(endpoint)
        val limit = Semaphore(PARALLEL_REQUESTS)
        apps.map { app ->
            async {
                val installed = installedApps.get(app.packageName)
                val isSelf = app.packageName == selfPackage
                try {
                    val latest = limit.withPermit { api.latest(endpoint, app.slug) }
                    CatalogEntry(app, installed, latest, isSelf)
                } catch (e: ReleaseHubException) {
                    if (e.unauthorized) throw e
                    CatalogEntry(app, installed, latest = null, isSelf = isSelf, error = e.message)
                }
            }
        }.awaitAll().sortedWith(CatalogEntry.ORDER)
    }

    private companion object {
        const val PARALLEL_REQUESTS = 4
    }
}
