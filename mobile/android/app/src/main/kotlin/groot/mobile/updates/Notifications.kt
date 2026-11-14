package groot.mobile.updates

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.provider.Settings
import androidx.core.net.toUri
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import groot.mobile.MainActivity
import groot.mobile.R

/** Every notification the app posts. Silently does nothing while notifications aren't allowed. */
class Notifications(context: Context) {
    private val appContext = context.applicationContext
    private val manager = NotificationManagerCompat.from(appContext)

    fun createChannels() {
        val nm = appContext.getSystemService(NotificationManager::class.java) ?: return
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL_UPDATES, appContext.getString(R.string.notif_channel_updates), NotificationManager.IMPORTANCE_DEFAULT),
        )
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL_INSTALLS, appContext.getString(R.string.notif_channel_installs), NotificationManager.IMPORTANCE_HIGH),
        )
    }

    /** Updates the person has to install themselves (auto-update off for them, or it couldn't finish). */
    fun updatesAvailable(titles: List<String>) {
        if (titles.isEmpty()) {
            manager.cancel(ID_UPDATES)
            return
        }
        val text = titles.joinToString(", ")
        post(
            ID_UPDATES,
            NotificationCompat.Builder(appContext, CHANNEL_UPDATES)
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(appContext.resources.getQuantityString(R.plurals.notif_updates_title, titles.size, titles.size))
                .setContentText(text)
                .setStyle(NotificationCompat.BigTextStyle().bigText(text))
                .setContentIntent(openApp())
                .setAutoCancel(true),
        )
    }

    fun confirmInstall(packageName: String, title: String, confirmIntent: Intent) {
        val pending = PendingIntent.getActivity(appContext, packageName.hashCode(), confirmIntent, IMMUTABLE_UPDATE)
        post(
            idFor(packageName),
            NotificationCompat.Builder(appContext, CHANNEL_INSTALLS)
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(appContext.getString(R.string.notif_confirm_title, title))
                .setContentText(appContext.getString(R.string.notif_confirm_text))
                .setContentIntent(pending)
                .setAutoCancel(true),
        )
    }

    fun installed(packageName: String, title: String, versionName: String) = post(
        idFor(packageName),
        NotificationCompat.Builder(appContext, CHANNEL_UPDATES)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(appContext.getString(R.string.notif_installed_title, title))
            .setContentText(appContext.getString(R.string.notif_installed_text, versionName))
            .setContentIntent(openApp())
            .setAutoCancel(true),
    )

    fun failed(packageName: String, title: String, message: String) = post(
        idFor(packageName),
        NotificationCompat.Builder(appContext, CHANNEL_INSTALLS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(appContext.getString(R.string.notif_failed_title, title))
            .setContentText(message)
            .setStyle(NotificationCompat.BigTextStyle().bigText(message))
            .setContentIntent(openApp())
            .setAutoCancel(true),
    )

    fun needsInstallPermission() {
        val intent = Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, "package:${appContext.packageName}".toUri())
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        post(
            ID_PERMISSION,
            NotificationCompat.Builder(appContext, CHANNEL_INSTALLS)
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(appContext.getString(R.string.notif_permission_title))
                .setContentText(appContext.getString(R.string.notif_permission_text))
                .setContentIntent(PendingIntent.getActivity(appContext, ID_PERMISSION, intent, IMMUTABLE_UPDATE))
                .setAutoCancel(true),
        )
    }

    fun cancel(packageName: String) = manager.cancel(idFor(packageName))

    private fun post(id: Int, builder: NotificationCompat.Builder) {
        val allowed = Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(appContext, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        if (!allowed) return
        try {
            manager.notify(id, builder.build())
        } catch (_: SecurityException) {
            // Revoked between the check and the post; nothing to show.
        }
    }

    private fun openApp(): PendingIntent = PendingIntent.getActivity(
        appContext,
        0,
        Intent(appContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
        IMMUTABLE_UPDATE,
    )

    private fun idFor(packageName: String) = ID_PACKAGE_BASE + (packageName.hashCode() and 0xFFFF)

    private companion object {
        const val CHANNEL_UPDATES = "updates"
        const val CHANNEL_INSTALLS = "installs"
        const val ID_UPDATES = 1
        const val ID_PERMISSION = 2
        const val ID_PACKAGE_BASE = 1_000
        const val IMMUTABLE_UPDATE = PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
    }
}
