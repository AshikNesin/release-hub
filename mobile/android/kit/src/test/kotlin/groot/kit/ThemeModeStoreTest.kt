package groot.kit

import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import groot.kit.theme.ThemeMode
import groot.kit.theme.ThemeModeStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class ThemeModeStoreTest {
    @Test
    fun `defaults to System`() = runTest {
        val store = ThemeModeStore(FakePreferencesDataStore())
        assertEquals(ThemeMode.System, store.current())
    }

    @Test
    fun `persists the chosen mode and notifies`() = runTest {
        val changes = mutableListOf<ThemeMode>()
        val store = ThemeModeStore(FakePreferencesDataStore()) { changes += it }

        store.setThemeMode(ThemeMode.Dark)

        assertEquals(ThemeMode.Dark, store.themeMode.first())
        assertEquals(listOf(ThemeMode.Dark), changes)
    }

    @Test
    fun `unknown stored value falls back to System`() = runTest {
        val prefs = preferencesOf(stringPreferencesKey("theme_mode") to "Sepia")
        val store = ThemeModeStore(FakePreferencesDataStore(prefs))
        assertEquals(ThemeMode.System, store.current())
    }
}
