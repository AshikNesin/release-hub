package groot.mobile

import android.app.Application
import android.content.Context
import groot.kit.buildinfo.BuildInfo
import groot.kit.log.Log
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.AppPreferences

class GrootMobileApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        Log.enabled = BuildConfig.DEBUG
        container = AppContainer(this)
    }
}

/** Manual dependency wiring; no DI framework (RFC 0001 §3). Add your repositories here. */
class AppContainer(context: Context) {
    private val appContext = context.applicationContext

    val themeModeStore: ThemeModeStore = ThemeModeStore.create(appContext)

    val appPreferences: AppPreferences = AppPreferences.create(appContext)

    /** Lazy: the kit reads nothing but the theme key at startup. */
    val buildInfo: BuildInfo by lazy { BuildInfo.from(appContext, commit = BuildConfig.GIT_COMMIT) }
}
