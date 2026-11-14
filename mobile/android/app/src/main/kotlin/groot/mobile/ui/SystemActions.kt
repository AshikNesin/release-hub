package groot.mobile.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.provider.Settings
import androidx.core.net.toUri

/** Screens Android owns: other apps, the uninstall dialog, and this app's permission pages. */
object SystemActions {
    fun open(context: Context, intent: Intent?) {
        intent ?: return
        start(context, intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }

    fun uninstall(context: Context, packageName: String) =
        start(context, Intent(Intent.ACTION_DELETE, "package:$packageName".toUri()))

    /** "Install unknown apps" for this app. */
    fun installPermission(context: Context) =
        start(context, Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, "package:${context.packageName}".toUri()))

    fun notificationSettings(context: Context) =
        start(context, Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName))

    private fun start(context: Context, intent: Intent) {
        try {
            context.startActivity(intent)
        } catch (_: ActivityNotFoundException) {
            // Nothing on this device handles it; the screen stays as it was.
        }
    }
}
