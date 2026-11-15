package groot.mobile

import android.app.Application
import android.content.Context
import android.util.LruCache
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import groot.kit.buildinfo.BuildInfo
import groot.kit.log.Log
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.ReleaseHubSettings
import groot.mobile.data.InstalledApps
import groot.mobile.data.PackageManagerInstalledApps
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.releasehub.HttpReleaseHubApi
import groot.mobile.data.releasehub.ReleaseHubApi
import groot.mobile.install.AndroidInstallUi
import groot.mobile.install.AppVisibility
import groot.mobile.install.HttpApkDownloader
import groot.mobile.install.InstallController
import groot.mobile.install.InstallTracker
import groot.mobile.install.PackageInstallerSessionInstaller
import groot.mobile.navigation.AppGraph
import groot.mobile.ui.AppIconLoader
import groot.mobile.updates.Notifications
import groot.mobile.updates.UpdateRunner
import groot.mobile.updates.UpdateScheduler
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

class GrootMobileApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        Log.enabled = BuildConfig.DEBUG
        container = AppContainer(this)
        registerActivityLifecycleCallbacks(container.visibility)
        container.notifications.createChannels()
        container.keepUpdateScheduleInSync()
    }
}

/** Manual dependency wiring; no DI framework (RFC 0001 §3). */
class AppContainer(context: Context) {
    private val appContext = context.applicationContext

    /** Process-wide scope for work that outlives a screen (schedule upkeep). */
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    val themeModeStore: ThemeModeStore = ThemeModeStore.create(appContext)

    val releaseHubSettings: ReleaseHubSettings = ReleaseHubSettings.create(appContext)

    val releaseHubApi: ReleaseHubApi = HttpReleaseHubApi()

    val installedApps: InstalledApps = PackageManagerInstalledApps(appContext)

    val catalog = CatalogRepository(releaseHubApi, releaseHubSettings, installedApps, selfPackage = appContext.packageName)

    val visibility = AppVisibility()

    val notifications = Notifications(appContext)

    val installTracker = InstallTracker()

    val installController = InstallController(
        downloader = HttpApkDownloader(),
        sessions = PackageInstallerSessionInstaller(appContext),
        tracker = installTracker,
        catalog = catalog,
        ui = AndroidInstallUi(appContext, visibility, notifications),
        apkDir = File(appContext.cacheDir, "apks"),
    )

    val updateRunner = UpdateRunner(releaseHubSettings, catalog, installController, notifications::updatesAvailable)

    private val scheduler = UpdateScheduler(appContext)

    /** Lazy: the kit reads nothing but the theme key at startup. */
    val buildInfo: BuildInfo by lazy { BuildInfo.from(appContext, commit = BuildConfig.GIT_COMMIT) }

    private val iconSizePx = (ICON_DP * appContext.resources.displayMetrics.density).toInt()
    private val iconCache = LruCache<String, ImageBitmap>(ICON_CACHE_SIZE)

    val graph = AppGraph(
        themeModeStore = themeModeStore,
        releaseHubSettings = releaseHubSettings,
        releaseHubApi = releaseHubApi,
        catalog = catalog,
        installController = installController,
        installTracker = installTracker,
        installedApps = installedApps,
        icons = AppIconLoader { packageName ->
            val version = installedApps.get(packageName)?.versionCode ?: return@AppIconLoader null
            val key = "$packageName@$version"
            iconCache.get(key) ?: installedApps.icon(packageName, iconSizePx)?.asImageBitmap()?.also { iconCache.put(key, it) }
        },
        appScope = appScope,
        buildInfo = { buildInfo },
        selfPackage = appContext.packageName,
        allowCleartext = BuildConfig.DEBUG,
    )

    /** Schedules, reschedules or cancels the background check whenever the release hub or update settings change. */
    fun keepUpdateScheduleInSync() {
        appScope.launch {
            combine(releaseHubSettings.endpoint, releaseHubSettings.updatePrefs) { endpoint, prefs ->
                (endpoint != null) to prefs.copy(lastCheckedAt = 0, excludedSlugs = emptySet())
            }
                .distinctUntilChanged()
                .collect { (connected, prefs) -> scheduler.apply(connected, prefs) }
        }
    }

    private companion object {
        /** Largest icon on screen (the detail header); smaller ones scale down. */
        const val ICON_DP = 64
        const val ICON_CACHE_SIZE = 64
    }
}
