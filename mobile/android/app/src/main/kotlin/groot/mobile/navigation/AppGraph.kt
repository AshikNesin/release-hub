package groot.mobile.navigation

import groot.kit.buildinfo.BuildInfo
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.ReleaseHubSettings
import groot.mobile.data.InstalledApps
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.releasehub.ReleaseHubApi
import groot.mobile.install.InstallController
import groot.mobile.install.InstallTracker
import groot.mobile.ui.AppIconLoader
import kotlinx.coroutines.CoroutineScope

/** Everything the screens need, so tests can build the whole app from fakes. */
class AppGraph(
    val themeModeStore: ThemeModeStore,
    val releaseHubSettings: ReleaseHubSettings,
    val releaseHubApi: ReleaseHubApi,
    val catalog: CatalogRepository,
    val installController: InstallController,
    val installTracker: InstallTracker,
    val installedApps: InstalledApps,
    val icons: AppIconLoader,
    /** Outlives screens: installs run here. */
    val appScope: CoroutineScope,
    val buildInfo: () -> BuildInfo,
    val selfPackage: String,
    /** Plain-http hubs: debug builds only. */
    val allowCleartext: Boolean,
)
