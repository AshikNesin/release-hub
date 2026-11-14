package groot.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import groot.mobile.data.hub.HubException
import groot.mobile.feature.connect.ConnectError
import groot.mobile.feature.connect.ConnectViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
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
class ConnectViewModelTest {
    private val api = FakeHubApi()
    private val settings = fakeHubSettings()

    @Before
    fun setUp() = Dispatchers.setMain(UnconfinedTestDispatcher())

    @After
    fun tearDown() = Dispatchers.resetMain()

    @Test
    fun `a working URL and token are checked against the hub, then saved`() = runTest {
        val vm = ConnectViewModel(api, settings, allowCleartext = false)
        vm.onUrlChange("hub.example.com/")
        vm.onTokenChange(" rh_test ")
        vm.connect()

        assertEquals(listOf(fakeEndpoint), api.endpointsSeen)
        assertEquals(fakeEndpoint, settings.currentEndpoint())
        assertTrue(vm.state.value.connected)
        assertEquals("", vm.state.value.token)
    }

    @Test
    fun `the hub's refusal is shown and nothing is saved`() = runTest {
        api.failure = HubException("The hub rejected the API token", 401)
        val vm = ConnectViewModel(api, settings, allowCleartext = false)
        vm.onUrlChange(HUB_URL)
        vm.onTokenChange("rh_wrong")
        vm.connect()

        assertEquals("The hub rejected the API token", vm.state.value.hubError)
        assertFalse(vm.state.value.connected)
        assertNull(settings.currentEndpoint())
    }

    @Test
    fun `bad input never reaches the hub`() = runTest {
        val vm = ConnectViewModel(api, settings, allowCleartext = false)
        vm.onUrlChange("ftp://nope")
        vm.onTokenChange("rh_x")
        vm.connect()
        assertEquals(ConnectError.InvalidUrl, vm.state.value.inputError)

        vm.onUrlChange("http://192.168.1.5:9100")
        vm.connect()
        assertEquals(ConnectError.Cleartext, vm.state.value.inputError)

        vm.onUrlChange(HUB_URL)
        vm.onTokenChange("")
        vm.connect()
        assertEquals(ConnectError.MissingToken, vm.state.value.inputError)
        assertTrue(api.endpointsSeen.isEmpty())
    }

    @Test
    fun `debug builds may use a plain-http hub on the LAN`() = runTest {
        val vm = ConnectViewModel(api, settings, allowCleartext = true)
        vm.onUrlChange("http://192.168.1.5:9100")
        vm.onTokenChange("rh_test")
        vm.connect()
        assertTrue(vm.state.value.connected)
    }

    @Test
    fun `an empty token keeps the saved one for the same hub`() = runTest {
        settings.connect(fakeEndpoint)
        val vm = ConnectViewModel(api, settings, allowCleartext = false)
        assertEquals(HUB_URL, vm.state.value.url)
        assertTrue(vm.state.value.hasSavedToken)

        vm.connect()

        assertEquals(listOf(fakeEndpoint), api.endpointsSeen)
        assertTrue(vm.state.value.connected)
    }
}
