package groot.mobile.install

import android.app.Activity
import android.app.Application
import android.content.Context
import android.content.Intent
import android.os.Bundle
import groot.mobile.updates.Notifications

/** Counts started activities, so installs know whether the person is looking at the app. */
class AppVisibility : Application.ActivityLifecycleCallbacks {
    @Volatile
    private var started = 0

    val isForeground: Boolean get() = started > 0

    override fun onActivityStarted(activity: Activity) {
        started++
    }

    override fun onActivityStopped(activity: Activity) {
        started = (started - 1).coerceAtLeast(0)
    }

    override fun onActivityCreated(activity: Activity, savedInstanceState: Bundle?) = Unit
    override fun onActivityResumed(activity: Activity) = Unit
    override fun onActivityPaused(activity: Activity) = Unit
    override fun onActivitySaveInstanceState(activity: Activity, outState: Bundle) = Unit
    override fun onActivityDestroyed(activity: Activity) = Unit
}

class AndroidInstallUi(
    context: Context,
    private val visibility: AppVisibility,
    private val notifications: Notifications,
) : InstallUi {
    private val appContext = context.applicationContext

    override fun isForeground(): Boolean = visibility.isForeground

    override fun startConfirmation(intent: Intent) {
        runCatching { appContext.startActivity(intent) }
    }

    override fun notifyConfirmation(packageName: String, title: String, intent: Intent) =
        notifications.confirmInstall(packageName, title, intent)

    override fun notifyInstalled(packageName: String, title: String, versionName: String) =
        notifications.installed(packageName, title, versionName)

    override fun notifyFailed(packageName: String, title: String, message: String) =
        notifications.failed(packageName, title, message)

    override fun notifyNeedsPermission() = notifications.needsInstallPermission()
}
