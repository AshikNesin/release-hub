package groot.mobile.data

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.os.Build
import androidx.core.graphics.createBitmap

/** What's installed on this device for one package. */
data class InstalledApp(
    val packageName: String,
    val label: String,
    val versionCode: Long,
    val versionName: String,
    /** Package that installed it (this app, Play, a browser…), when Android reports it. */
    val installer: String?,
)

/** Reads installed packages. A fake stands in for it in tests. */
interface InstalledApps {
    fun get(packageName: String): InstalledApp?

    fun icon(packageName: String, sizePx: Int): Bitmap?

    fun launchIntent(packageName: String): Intent?
}

/** [InstalledApps] over PackageManager. Needs QUERY_ALL_PACKAGES (an app store's case) to see other apps. */
class PackageManagerInstalledApps(context: Context) : InstalledApps {
    private val pm: PackageManager = context.applicationContext.packageManager

    override fun get(packageName: String): InstalledApp? {
        val info = try {
            pm.getPackageInfo(packageName, 0)
        } catch (_: PackageManager.NameNotFoundException) {
            return null
        }
        val label = info.applicationInfo?.loadLabel(pm)?.toString() ?: packageName
        return InstalledApp(
            packageName = packageName,
            label = label,
            versionCode = info.longVersionCode,
            versionName = info.versionName.orEmpty(),
            installer = installerOf(packageName),
        )
    }

    override fun icon(packageName: String, sizePx: Int): Bitmap? = runCatching {
        val drawable = pm.getApplicationIcon(packageName)
        createBitmap(sizePx, sizePx).also { bitmap ->
            drawable.setBounds(0, 0, sizePx, sizePx)
            drawable.draw(Canvas(bitmap))
        }
    }.getOrNull()

    override fun launchIntent(packageName: String): Intent? = pm.getLaunchIntentForPackage(packageName)

    private fun installerOf(packageName: String): String? = runCatching {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            pm.getInstallSourceInfo(packageName).installingPackageName
        } else {
            @Suppress("DEPRECATION")
            pm.getInstallerPackageName(packageName)
        }
    }.getOrNull()
}
