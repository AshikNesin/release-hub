package groot.mobile

import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.kit.theme.ThemeMode
import groot.mobile.data.UpdatePrefs
import groot.mobile.feature.settings.SettingsViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(AndroidJUnit4::class)
class SettingsTest {
    @Before
    fun setUp() = Dispatchers.setMain(UnconfinedTestDispatcher())

    @After
    fun tearDown() = Dispatchers.resetMain()

    @Test
    fun `the token is stored encrypted, never as typed`() = runTest {
        val store = FakePreferencesDataStore()
        val settings = fakeHubSettings(store)
        settings.connect(fakeEndpoint)

        val stored = store.data.first()[stringPreferencesKey("token_encrypted")]
        assertEquals("enc:tset_hr", stored)
        assertEquals(fakeEndpoint, settings.currentEndpoint())
    }

    @Test
    fun `update preferences have safe defaults and per-app opt-outs`() = runTest {
        val settings = fakeHubSettings()
        val defaults = settings.currentUpdatePrefs()
        assertTrue(defaults.autoUpdate)
        assertTrue(defaults.unmeteredOnly)
        assertEquals(UpdatePrefs.DEFAULT_INTERVAL_HOURS, defaults.checkIntervalHours)

        settings.setAutoUpdateFor("demo", enabled = false)
        assertFalse(settings.currentUpdatePrefs().autoUpdates("demo"))
        assertTrue(settings.currentUpdatePrefs().autoUpdates("other"))
        settings.setAutoUpdateFor("demo", enabled = true)
        assertTrue(settings.currentUpdatePrefs().autoUpdates("demo"))
    }

    @Test
    fun `settings screen saves changes, disconnects and resets`() = runTest {
        val app = TestApp(backgroundScope)
        val themeStore = fakeThemeModeStore()
        val vm = SettingsViewModel(themeStore, app.settings, app.catalog)
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { vm.hubUrl.collect {} }
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { vm.updatePrefs.collect {} }
        app.settings.connect(fakeEndpoint)
        assertEquals(HUB_URL, vm.hubUrl.value)

        vm.setThemeMode(ThemeMode.Dark)
        vm.setUnmeteredOnly(false)
        vm.setCheckIntervalHours(24)
        assertEquals(ThemeMode.Dark, themeStore.themeMode.first())
        assertFalse(vm.updatePrefs.value.unmeteredOnly)
        assertEquals(24, vm.updatePrefs.value.checkIntervalHours)

        vm.disconnect()
        assertNull(app.settings.currentEndpoint())
        // The URL stays, so reconnecting only needs a token.
        assertEquals(HUB_URL, app.settings.baseUrl.first())

        vm.resetAll()
        assertEquals(ThemeMode.System, themeStore.themeMode.first())
        assertNull(app.settings.baseUrl.first())
        assertTrue(app.settings.currentUpdatePrefs().unmeteredOnly)
    }
}
