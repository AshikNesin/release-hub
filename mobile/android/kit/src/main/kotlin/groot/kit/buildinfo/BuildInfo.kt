package groot.kit.buildinfo

import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.os.Build

/** What build is running. Shown on About screens and attached to bug reports. */
public data class BuildInfo(
    val versionName: String,
    val versionCode: Long,
    val commit: String,
    val isDebug: Boolean,
) {
    public companion object {
        /** Reads version and debuggable flag from the installed package; [commit] comes from the app's BuildConfig. */
        public fun from(context: Context, commit: String): BuildInfo {
            val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                context.packageManager.getPackageInfo(context.packageName, PackageManager.PackageInfoFlags.of(0))
            } else {
                @Suppress("DEPRECATION")
                context.packageManager.getPackageInfo(context.packageName, 0)
            }
            return BuildInfo(
                versionName = info.versionName ?: "unknown",
                versionCode = info.longVersionCode,
                commit = commit,
                isDebug = (context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE) != 0,
            )
        }
    }
}
