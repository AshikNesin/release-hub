package groot.mobile

import androidx.compose.runtime.Composable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.github.takahirom.roborazzi.captureRoboImage
import com.github.takahirom.roborazzi.captureScreenRoboImage
import groot.kit.theme.GrootTheme
import groot.kit.theme.ThemeMode
import groot.mobile.data.InstalledApp
import groot.mobile.data.UpdatePrefs
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.catalog.CatalogState
import groot.mobile.data.releasehub.ReleaseHubApp
import groot.mobile.feature.about.AboutScreen
import groot.mobile.feature.apps.AppsScreen
import groot.mobile.feature.apps.AppsUiState
import groot.mobile.feature.connect.ConnectScreen
import groot.mobile.feature.connect.ConnectUiState
import groot.mobile.feature.detail.AppDetailScreen
import groot.mobile.feature.detail.AppDetailUiState
import groot.mobile.feature.detail.HistoryState
import groot.mobile.feature.settings.SettingsScreen
import groot.mobile.install.InstallState
import groot.mobile.ui.AppIconLoader
import groot.mobile.ui.brand.BrandPalette
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Renders the screens to PNGs for review: `pnpm screenshots`
 * (`./gradlew :app:recordRoborazziDebug`) writes them to app/build/outputs/roborazzi.
 * Not a golden-image gate (RFC 0001 Q9); a normal test run only checks they render.
 * Long screens use a taller window so they fit in one image.
 */
@RunWith(AndroidJUnit4::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w412dp-h915dp-xxhdpi")
class ScreenshotTest {
    @get:Rule
    val compose = createComposeRule()

    @Test fun appsLight() = capture("apps_light", ThemeMode.Light) { Apps() }

    @Test fun appsDark() = capture("apps_dark", ThemeMode.Dark) { Apps() }

    @Test fun connectPromptLight() = capture("connect_prompt_light", ThemeMode.Light) { Apps(AppsUiState(connected = false)) }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun detailLight() = capture("detail_light", ThemeMode.Light) { Detail() }

    @Test
    @Config(qualifiers = "w412dp-h1240dp-xxhdpi")
    fun detailDark() = capture("detail_dark", ThemeMode.Dark) { Detail() }

    @Test fun connectDark() = capture("connect_dark", ThemeMode.Dark) { Connect() }

    @Test
    @Config(qualifiers = "w412dp-h1600dp-xxhdpi")
    fun settingsLight() = capture("settings_light", ThemeMode.Light) { Settings(ThemeMode.Light) }

    @Test
    @Config(qualifiers = "w412dp-h1600dp-xxhdpi")
    fun settingsDark() = capture("settings_dark", ThemeMode.Dark) { Settings(ThemeMode.Dark) }

    @Test
    fun settingsIntervalPickerLight() =
        captureScreen("settings_interval_picker_light", ThemeMode.Light, before = { compose.onNodeWithText("Check for updates").performClick() }) {
            Settings(ThemeMode.Light)
        }

    @Test fun aboutLight() = capture("about_light", ThemeMode.Light) { About() }

    private fun capture(name: String, mode: ThemeMode, before: () -> Unit = {}, content: @Composable () -> Unit) {
        compose.setContent { GrootTheme(mode = mode, palette = BrandPalette, content = content) }
        before()
        compose.waitForIdle()
        compose.onRoot().captureRoboImage("build/outputs/roborazzi/$name.png")
    }

    /** Captures every window, so a bottom sheet (its own dialog window) is in the image. */
    @OptIn(com.github.takahirom.roborazzi.ExperimentalRoborazziApi::class)
    private fun captureScreen(name: String, mode: ThemeMode, before: () -> Unit = {}, content: @Composable () -> Unit) {
        compose.setContent { GrootTheme(mode = mode, palette = BrandPalette, content = content) }
        before()
        compose.mainClock.advanceTimeBy(SHEET_SETTLE_MS)
        compose.waitForIdle()
        captureScreenRoboImage("build/outputs/roborazzi/$name.png")
    }

    @Composable
    private fun Apps(state: AppsUiState = sampleApps) = AppsScreen(
        state = state,
        canInstall = false,
        icons = AppIconLoader.None,
        canOpen = { true },
        onRefresh = {},
        onOpenApp = {},
        onAction = { _, _ -> },
        onUpdateAll = {},
        onAllowInstalls = {},
        onConnect = {},
        onOpenSettings = {},
    )

    @Composable
    private fun Detail() = AppDetailScreen(
        state = AppDetailUiState(
            entry = sampleEntries.first { it.slug == "tinyfirewall" },
            prefs = UpdatePrefs(),
            history = HistoryState(loading = false, releases = listOf(release(143), release(142), release(140))),
        ),
        icons = AppIconLoader.None,
        selfPackage = SELF_PACKAGE,
        canOpen = true,
        onNavigateUp = {},
        onInstall = {},
        onInstallRelease = {},
        onOpen = {},
        onUninstall = {},
        onAutoUpdateChange = {},
        onDismissError = {},
        onRetryHistory = {},
    )

    @Composable
    private fun Connect() = ConnectScreen(
        state = ConnectUiState(url = RELEASE_HUB_URL, releaseHubError = "The release hub rejected the API token"),
        onUrlChange = {},
        onTokenChange = {},
        onConnect = {},
        onNavigateUp = {},
    )

    @Composable
    private fun Settings(mode: ThemeMode) = SettingsScreen(
        themeMode = mode,
        onThemeModeChange = {},
        releaseHubUrl = RELEASE_HUB_URL,
        onOpenReleaseHub = {},
        onDisconnect = {},
        prefs = UpdatePrefs(lastCheckedAt = 1_794_650_000_000),
        onAutoUpdateChange = {},
        onUnmeteredChange = {},
        onIntervalChange = {},
        installAllowed = true,
        notificationsAllowed = false,
        onOpenInstallPermission = {},
        onOpenNotifications = {},
        onResetData = {},
        buildInfo = fakeBuildInfo,
        onNavigateUp = {},
        onOpenAbout = {},
    )

    @Composable
    private fun About() = AboutScreen(buildInfo = fakeBuildInfo, onNavigateUp = {})
}

private fun installed(pkg: String, label: String, code: Long) = InstalledApp(pkg, label, code, "1.$code", SELF_PACKAGE)

private val sampleEntries = listOf(
    CatalogEntry(ReleaseHubApp("tinyfirewall", "io.nesin.tinyfirewall"), installed("io.nesin.tinyfirewall", "Tiny Firewall", 142), manifest(143, notes = "Faster rule sync.\nFixes a crash on Android 16.")),
    CatalogEntry(ReleaseHubApp("budget", "io.nesin.budget"), installed("io.nesin.budget", "Budget", 20), manifest(21)),
    CatalogEntry(ReleaseHubApp("notes", "io.nesin.notes"), null, manifest(3)),
    CatalogEntry(ReleaseHubApp("release-hub", SELF_PACKAGE), installed(SELF_PACKAGE, "Release Hub", 1), manifest(1), isSelf = true),
    CatalogEntry(ReleaseHubApp("groot-demo", "io.nesin.groot"), installed("io.nesin.groot", "Groot Demo", 9), manifest(7)),
    CatalogEntry(ReleaseHubApp("prototype", "io.nesin.prototype"), null, null),
).sortedWith(CatalogEntry.ORDER)

private val sampleApps = AppsUiState(
    connected = true,
    catalog = CatalogState(entries = sampleEntries, loaded = true),
    installs = mapOf("io.nesin.budget" to InstallState.Downloading(0.4f)),
)

private const val SHEET_SETTLE_MS = 1_000L
