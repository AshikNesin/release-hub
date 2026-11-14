package groot.mobile.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.kit.theme.ThemeMode
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.AppPreferences
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

class SettingsViewModel(
    private val themeModeStore: ThemeModeStore,
    private val appPreferences: AppPreferences,
) : ViewModel() {
    val themeMode: StateFlow<ThemeMode> = themeModeStore.themeMode
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), ThemeMode.System)

    val showHomeIntro: StateFlow<Boolean> = appPreferences.showHomeIntro
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), true)

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { themeModeStore.setThemeMode(mode) }
    }

    fun setShowHomeIntro(show: Boolean) {
        viewModelScope.launch { appPreferences.setShowHomeIntro(show) }
    }

    /** "Reset all data": app preferences cleared, theme back to System. Add your own stores here. */
    fun resetAll() {
        viewModelScope.launch {
            appPreferences.clear()
            themeModeStore.setThemeMode(ThemeMode.System)
        }
    }

    companion object {
        fun factory(themeModeStore: ThemeModeStore, appPreferences: AppPreferences): ViewModelProvider.Factory =
            viewModelFactory { initializer { SettingsViewModel(themeModeStore, appPreferences) } }
    }
}
