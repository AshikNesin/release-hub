package groot.mobile.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.kit.theme.ThemeMode
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.HubSettings
import groot.mobile.data.UpdatePrefs
import groot.mobile.data.catalog.CatalogRepository
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

class SettingsViewModel(
    private val themeModeStore: ThemeModeStore,
    private val hubSettings: HubSettings,
    private val catalog: CatalogRepository,
) : ViewModel() {
    val themeMode: StateFlow<ThemeMode> = themeModeStore.themeMode
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), ThemeMode.System)

    /** The connected hub's URL, or null when not connected. */
    val hubUrl: StateFlow<String?> = hubSettings.endpoint.map { it?.baseUrl }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    val updatePrefs: StateFlow<UpdatePrefs> = hubSettings.updatePrefs
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), UpdatePrefs())

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { themeModeStore.setThemeMode(mode) }
    }

    fun setAutoUpdate(enabled: Boolean) {
        viewModelScope.launch { hubSettings.setAutoUpdate(enabled) }
    }

    fun setUnmeteredOnly(enabled: Boolean) {
        viewModelScope.launch { hubSettings.setUnmeteredOnly(enabled) }
    }

    fun setCheckIntervalHours(hours: Int) {
        viewModelScope.launch { hubSettings.setCheckIntervalHours(hours) }
    }

    fun disconnect() {
        viewModelScope.launch {
            hubSettings.disconnect()
            catalog.clear()
        }
    }

    /** "Reset all data": hub connection and update settings forgotten, theme back to System. */
    fun resetAll() {
        viewModelScope.launch {
            hubSettings.clear()
            catalog.clear()
            themeModeStore.setThemeMode(ThemeMode.System)
        }
    }

    companion object {
        fun factory(themeModeStore: ThemeModeStore, hubSettings: HubSettings, catalog: CatalogRepository): ViewModelProvider.Factory =
            viewModelFactory { initializer { SettingsViewModel(themeModeStore, hubSettings, catalog) } }
    }
}
