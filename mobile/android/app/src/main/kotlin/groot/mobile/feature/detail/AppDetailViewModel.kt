package groot.mobile.feature.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.mobile.data.HubSettings
import groot.mobile.data.UpdatePrefs
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.hub.HubException
import groot.mobile.data.hub.HubManifest
import groot.mobile.data.hub.HubRelease
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
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

data class HistoryState(
    val loading: Boolean = true,
    /** Installable (direct-channel APK) releases, newest first. */
    val releases: List<HubRelease> = emptyList(),
    val error: String? = null,
)

data class AppDetailUiState(
    val entry: CatalogEntry? = null,
    val install: InstallState? = null,
    val prefs: UpdatePrefs = UpdatePrefs(),
    val history: HistoryState = HistoryState(),
)

class AppDetailViewModel(
    private val slug: String,
    private val settings: HubSettings,
    private val catalog: CatalogRepository,
    private val installer: InstallController,
    tracker: InstallTracker,
    private val installScope: CoroutineScope,
) : ViewModel() {
    private val history = MutableStateFlow(HistoryState())
    private val _needsInstallPermission = MutableStateFlow(false)
    val needsInstallPermission: StateFlow<Boolean> = _needsInstallPermission.asStateFlow()

    val state: StateFlow<AppDetailUiState> =
        combine(catalog.state, tracker.states, settings.updatePrefs, history) { catalog, installs, prefs, history ->
            val entry = catalog.entries.firstOrNull { it.slug == slug }
            AppDetailUiState(entry, entry?.let { installs[it.packageName] }, prefs, history)
        }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), AppDetailUiState(entry = catalog.entry(slug)))

    init {
        loadHistory()
    }

    fun loadHistory() {
        history.value = history.value.copy(loading = true, error = null)
        viewModelScope.launch {
            history.value = try {
                HistoryState(loading = false, releases = catalog.releases(slug).filter { it.installable })
            } catch (e: HubException) {
                HistoryState(loading = false, error = e.message)
            }
        }
    }

    /** Installs [release] (the newest by default). */
    fun install(release: HubManifest? = null) {
        val entry = state.value.entry ?: return
        installScope.launch {
            if (installer.install(entry, release ?: entry.latest) == InstallResult.NeedsPermission) {
                _needsInstallPermission.value = true
            }
        }
    }

    fun setAutoUpdate(enabled: Boolean) {
        viewModelScope.launch { settings.setAutoUpdateFor(slug, enabled) }
    }

    fun dismissError() {
        state.value.entry?.let { installer.dismissError(it.packageName) }
    }

    fun onResume() {
        state.value.entry?.let { catalog.refreshInstalled(it.packageName) }
    }

    fun installPermissionHandled() {
        _needsInstallPermission.value = false
    }

    companion object {
        fun factory(
            slug: String,
            settings: HubSettings,
            catalog: CatalogRepository,
            installer: InstallController,
            tracker: InstallTracker,
            installScope: CoroutineScope,
        ): ViewModelProvider.Factory = viewModelFactory {
            initializer { AppDetailViewModel(slug, settings, catalog, installer, tracker, installScope) }
        }
    }
}
