package groot.mobile.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

private val Context.appPreferencesDataStore: DataStore<Preferences> by preferencesDataStore(name = "app_prefs")

/** The reference app's own preferences (app-owned; the kit only stores the theme). */
class AppPreferences(private val dataStore: DataStore<Preferences>) {
    val showHomeIntro: Flow<Boolean> = dataStore.data
        .map { it[SHOW_HOME_INTRO] ?: true }
        .distinctUntilChanged()

    suspend fun setShowHomeIntro(show: Boolean) {
        dataStore.edit { it[SHOW_HOME_INTRO] = show }
    }

    suspend fun clear() {
        dataStore.edit { it.clear() }
    }

    companion object {
        private val SHOW_HOME_INTRO = booleanPreferencesKey("show_home_intro")

        fun create(context: Context) = AppPreferences(context.applicationContext.appPreferencesDataStore)
    }
}
