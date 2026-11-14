package groot.mobile

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import groot.kit.buildinfo.BuildInfo
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.AppPreferences
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow

/** In-memory DataStore: unit tests never touch the disk. */
class FakePreferencesDataStore : DataStore<Preferences> {
    private val state = MutableStateFlow(emptyPreferences())
    override val data: Flow<Preferences> = state

    override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
        transform(state.value).also { state.value = it }
}

fun fakeThemeModeStore() = ThemeModeStore(FakePreferencesDataStore())

fun fakeAppPreferences() = AppPreferences(FakePreferencesDataStore())

val fakeBuildInfo = BuildInfo(versionName = "0.1.0", versionCode = 1, commit = "3a4fd6d", isDebug = true)
