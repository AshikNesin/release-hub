package groot.kit.theme

import android.app.UiModeManager
import android.content.Context
import android.os.Build
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.grootKitDataStore: DataStore<Preferences> by preferencesDataStore(name = "groot_kit")

/**
 * Persists the user's [ThemeMode] — the only key the kit stores, and the only thing it reads at startup.
 *
 * [onModeChanged] runs after each change. [create] wires it to `UiModeManager.setApplicationNightMode`
 * on API 31+, so the system applies the saved mode from the splash screen onward (no theme flash).
 */
public class ThemeModeStore(
    private val dataStore: DataStore<Preferences>,
    private val onModeChanged: (ThemeMode) -> Unit = {},
) {
    /** The saved mode; emits [ThemeMode.System] until the user picks something else. */
    public val themeMode: Flow<ThemeMode> = dataStore.data
        .map { prefs -> prefs[KEY].toThemeMode() }
        .distinctUntilChanged()

    /** One-shot read, for the first frame on API 29–30 (see RFC 0001 Q4). */
    public suspend fun current(): ThemeMode = themeMode.first()

    public suspend fun setThemeMode(mode: ThemeMode) {
        dataStore.edit { prefs -> prefs[KEY] = mode.name }
        onModeChanged(mode)
    }

    public companion object {
        private val KEY = stringPreferencesKey("theme_mode")

        /** The app-wide store. Create once (e.g. in your Application) and share it. */
        public fun create(context: Context): ThemeModeStore {
            val app = context.applicationContext
            return ThemeModeStore(app.grootKitDataStore) { mode -> app.applySystemNightMode(mode) }
        }

        private fun String?.toThemeMode(): ThemeMode =
            ThemeMode.entries.firstOrNull { it.name == this } ?: ThemeMode.System
    }
}

private fun Context.applySystemNightMode(mode: ThemeMode) {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return
    val uiModeManager = getSystemService(UiModeManager::class.java) ?: return
    uiModeManager.setApplicationNightMode(
        when (mode) {
            ThemeMode.System -> UiModeManager.MODE_NIGHT_AUTO
            ThemeMode.Light -> UiModeManager.MODE_NIGHT_NO
            ThemeMode.Dark -> UiModeManager.MODE_NIGHT_YES
        },
    )
}
