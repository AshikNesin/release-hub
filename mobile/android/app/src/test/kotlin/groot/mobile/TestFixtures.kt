package groot.mobile

import android.content.Intent
import android.graphics.Bitmap
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import groot.kit.buildinfo.BuildInfo
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.HubSettings
import groot.mobile.data.InstalledApp
import groot.mobile.data.InstalledApps
import groot.mobile.data.SecretCipher
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.hub.HubApi
import groot.mobile.data.hub.HubApp
import groot.mobile.data.hub.HubEndpoint
import groot.mobile.data.hub.HubException
import groot.mobile.data.hub.HubManifest
import groot.mobile.data.hub.HubRelease
import groot.mobile.install.ApkDownloader
import groot.mobile.install.InstallController
import groot.mobile.install.InstallException
import groot.mobile.install.InstallTracker
import groot.mobile.install.InstallUi
import groot.mobile.install.SessionInstaller
import groot.mobile.navigation.AppGraph
import groot.mobile.ui.AppIconLoader
import java.io.File
import java.nio.file.Files
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow

/** In-memory DataStore: unit tests never touch the disk. */
class FakePreferencesDataStore : DataStore<Preferences> {
    private val state = MutableStateFlow(emptyPreferences())
    override val data: Flow<Preferences> = state

    override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
        transform(state.value).also { state.value = it }
}

/** Reversible and visibly "encrypted", so tests can tell the stored form from the token. */
class FakeCipher : SecretCipher {
    override fun encrypt(plain: String) = "enc:" + plain.reversed()

    override fun decrypt(encoded: String) = encoded.removePrefix("enc:").reversed().takeIf { encoded.startsWith("enc:") }
}

fun fakeThemeModeStore() = ThemeModeStore(FakePreferencesDataStore())

fun fakeHubSettings(store: FakePreferencesDataStore = FakePreferencesDataStore()) = HubSettings(store, FakeCipher())

val fakeBuildInfo = BuildInfo(versionName = "0.1.0", versionCode = 1, commit = "3a4fd6d", isDebug = true)

const val HUB_URL = "https://hub.example.com"
val fakeEndpoint = HubEndpoint(HUB_URL, "rh_test")
const val SELF_PACKAGE = "io.nesin.releasehub"

fun manifest(code: Long, name: String = "1.$code", notes: String = "") =
    HubManifest(code, name, "$HUB_URL/artifacts/x/android/app-$code.apk", sha256 = "", size = 0, notes = notes, createdAt = "2026-11-01T10:00:00Z")

fun release(code: Long, channel: String = "direct", fileName: String = "app-$code.apk") =
    HubRelease(code, "1.$code", channel, "$HUB_URL/artifacts/x/android/$fileName", "", 2_048_000, "2026-11-01T10:00:00Z", fileName)

class FakeHubApi(
    var apps: List<HubApp> = emptyList(),
    var manifests: Map<String, HubManifest> = emptyMap(),
    var releases: Map<String, List<HubRelease>> = emptyMap(),
    /** Thrown by every call when set. */
    var failure: HubException? = null,
    /** Per-slug manifest failures. */
    var manifestFailures: Map<String, HubException> = emptyMap(),
) : HubApi {
    val endpointsSeen = mutableListOf<HubEndpoint>()

    override suspend fun listApps(endpoint: HubEndpoint): List<HubApp> {
        endpointsSeen += endpoint
        failure?.let { throw it }
        return apps
    }

    override suspend fun latest(endpoint: HubEndpoint, slug: String): HubManifest? {
        failure?.let { throw it }
        manifestFailures[slug]?.let { throw it }
        return manifests[slug]
    }

    override suspend fun releases(endpoint: HubEndpoint, slug: String): List<HubRelease> {
        failure?.let { throw it }
        return releases[slug].orEmpty()
    }
}

class FakeInstalledApps(val installed: MutableMap<String, InstalledApp> = mutableMapOf()) : InstalledApps {
    override fun get(packageName: String): InstalledApp? = installed[packageName]

    override fun icon(packageName: String, sizePx: Int): Bitmap? = null

    override fun launchIntent(packageName: String): Intent? = if (packageName in installed) Intent(Intent.ACTION_MAIN) else null

    fun install(packageName: String, code: Long, label: String = packageName.substringAfterLast('.'), installer: String? = SELF_PACKAGE) {
        installed[packageName] = InstalledApp(packageName, label, code, "1.$code", installer)
    }
}

class FakeDownloader(var failure: InstallException? = null) : ApkDownloader {
    val downloaded = mutableListOf<HubManifest>()

    override suspend fun download(manifest: HubManifest, dest: File, onProgress: (Float?) -> Unit) {
        failure?.let { throw it }
        onProgress(0.5f)
        dest.parentFile?.mkdirs()
        dest.writeText("apk ${manifest.versionCode}")
        onProgress(1f)
        downloaded += manifest
    }
}

class FakeSessionInstaller(var allowed: Boolean = true) : SessionInstaller {
    val commits = mutableListOf<Pair<Int, String>>()
    private var nextId = 100

    override fun commit(apk: File, packageName: String, onSession: (Int) -> Unit): Int =
        nextId++.also {
            onSession(it)
            commits += it to packageName
        }

    override fun canInstall() = allowed

    fun lastSession(): Int = commits.last().first
}

class FakeInstallUi(var foreground: Boolean = true) : InstallUi {
    val events = mutableListOf<String>()

    override fun isForeground() = foreground

    override fun startConfirmation(intent: Intent) {
        events += "confirm-screen"
    }

    override fun notifyConfirmation(packageName: String, title: String, intent: Intent) {
        events += "confirm-notification:$packageName"
    }

    override fun notifyInstalled(packageName: String, title: String, versionName: String) {
        events += "installed:$packageName:$versionName"
    }

    override fun notifyFailed(packageName: String, title: String, message: String) {
        events += "failed:$packageName"
    }

    override fun notifyNeedsPermission() {
        events += "needs-permission"
    }
}

/** A whole app wired from fakes. */
class TestApp(scope: CoroutineScope) {
    val api = FakeHubApi()
    val installed = FakeInstalledApps()
    val settings = fakeHubSettings()
    val tracker = InstallTracker()
    val catalog = CatalogRepository(api, settings, installed, SELF_PACKAGE, clock = { 1_000L })
    val downloader = FakeDownloader()
    val sessions = FakeSessionInstaller()
    val ui = FakeInstallUi()
    val controller = InstallController(
        downloader, sessions, tracker, catalog, ui,
        apkDir = Files.createTempDirectory("apks").toFile(),
        waitMillis = 5_000,
    )
    val graph = AppGraph(
        themeModeStore = fakeThemeModeStore(),
        hubSettings = settings,
        hubApi = api,
        catalog = catalog,
        installController = controller,
        installTracker = tracker,
        installedApps = installed,
        icons = AppIconLoader.None,
        appScope = scope,
        buildInfo = { fakeBuildInfo },
        selfPackage = SELF_PACKAGE,
        allowCleartext = false,
    )
}
