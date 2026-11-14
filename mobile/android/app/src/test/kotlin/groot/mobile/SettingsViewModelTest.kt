package groot.mobile

import groot.kit.theme.ThemeMode
import groot.mobile.feature.settings.SettingsViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SettingsViewModelTest {
    private val themeStore = fakeThemeModeStore()
    private val prefs = fakeAppPreferences()
    private lateinit var viewModel: SettingsViewModel

    @Before
    fun setUp() {
        Dispatchers.setMain(UnconfinedTestDispatcher())
        viewModel = SettingsViewModel(themeStore, prefs)
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun TestScope.collectState() {
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { viewModel.themeMode.collect {} }
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { viewModel.showHomeIntro.collect {} }
    }

    @Test
    fun `starts at System with the home intro on, and saves a new theme`() = runTest {
        collectState()
        assertEquals(ThemeMode.System, viewModel.themeMode.value)
        assertTrue(viewModel.showHomeIntro.value)

        viewModel.setThemeMode(ThemeMode.Dark)

        assertEquals(ThemeMode.Dark, themeStore.themeMode.first())
        assertEquals(ThemeMode.Dark, viewModel.themeMode.value)
    }

    @Test
    fun `home intro setting is saved`() = runTest {
        collectState()
        viewModel.setShowHomeIntro(false)
        assertFalse(prefs.showHomeIntro.first())
        assertFalse(viewModel.showHomeIntro.value)
    }

    @Test
    fun `reset clears app preferences and returns the theme to System`() = runTest {
        collectState()
        viewModel.setThemeMode(ThemeMode.Dark)
        viewModel.setShowHomeIntro(false)

        viewModel.resetAll()

        assertEquals(ThemeMode.System, themeStore.themeMode.first())
        assertTrue(prefs.showHomeIntro.first())
    }
}
