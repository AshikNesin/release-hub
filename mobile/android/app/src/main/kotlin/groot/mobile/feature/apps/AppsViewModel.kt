package groot.mobile.feature.apps

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.mobile.data.ReleaseHubSettings
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.catalog.CatalogState
import groot.mobile.install.InstallController
import groot.mobile.install.InstallResult
import groot.mobile.install.InstallState
import groot.mobile.install.InstallTracker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

data class AppsUiState(
    /** Null until the saved settings are read, so the connect prompt never flashes. */
    val connected: Boolean? = null,
    val catalog: CatalogState = CatalogState(),
    val installs: Map<String, InstallState> = emptyMap(),
) {
    val updates: List<CatalogEntry> get() = catalog.entries.filter { it.status == AppStatus.UpdateAvailable }
}

/**
 * The app list. Installs run in [installScope] (the process scope), not the ViewModel's, so leaving
 * the screen mid-download doesn't cancel them.
 */
class AppsViewModel(
    private val settings: ReleaseHubSettings,
    private val catalog: CatalogRepository,
    private val installer: InstallController,
    tracker: InstallTracker,
    private val installScope: CoroutineScope,
) : ViewModel() {
    private val connected = settings.endpoint.map { it != null }.distinctUntilChanged()

    val state: StateFlow<AppsUiState> = combine(connected, catalog.state, tracker.states) { connected, catalog, installs ->
        AppsUiState(connected, catalog, installs)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), AppsUiState())

    private val _needsInstallPermission = MutableStateFlow(false)

    /** True when an install was refused because "Install unknown apps" is off; the screen opens Settings. */
    val needsInstallPermission: StateFlow<Boolean> = _needsInstallPermission.asStateFlow()

    init {
        // Load whenever a release hub is (re)connected; forget the old hub's apps on disconnect.
        viewModelScope.launch {
            settings.endpoint.distinctUntilChanged().collect { endpoint ->
                if (endpoint == null) catalog.clear() else catalog.refreshQuietly()
            }
        }
    }

    fun refresh() {
        viewModelScope.launch { catalog.refreshQuietly() }
    }

    /** Back on screen: installs or uninstalls may have happened elsewhere. */
    fun onResume() = catalog.refreshAllInstalled()

    fun install(entry: CatalogEntry) {
        installScope.launch { handle(installer.install(entry)) }
    }

    fun updateAll() {
        val updates = state.value.updates.sortedBy { it.isSelf }
        installScope.launch {
            for (entry in updates) {
                val result = installer.install(entry)
                handle(result)
                if (result == InstallResult.NeedsPermission) break
            }
        }
    }

    fun dismissError(packageName: String) = installer.dismissError(packageName)

    fun canInstall(): Boolean = installer.canInstall()

    fun installPermissionHandled() {
        _needsInstallPermission.value = false
    }

    private fun handle(result: InstallResult) {
        if (result == InstallResult.NeedsPermission) _needsInstallPermission.value = true
    }

    companion object {
        fun factory(
            settings: ReleaseHubSettings,
            catalog: CatalogRepository,
            installer: InstallController,
            tracker: InstallTracker,
            installScope: CoroutineScope,
        ): ViewModelProvider.Factory = viewModelFactory {
            initializer { AppsViewModel(settings, catalog, installer, tracker, installScope) }
        }
    }
}
