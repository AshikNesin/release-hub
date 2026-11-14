package groot.mobile.install

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import groot.mobile.GrootMobileApp

/** Receives PackageInstaller session results and passes them to [InstallController]. */
class InstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION) return
        val controller = (context.applicationContext as GrootMobileApp).container.installController
        val packageName = intent.getStringExtra(EXTRA_TARGET_PACKAGE)
            ?: intent.getStringExtra(PackageInstaller.EXTRA_PACKAGE_NAME)
            ?: return
        val confirm: Intent? = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
        } else {
            @Suppress("DEPRECATION")
            intent.getParcelableExtra(Intent.EXTRA_INTENT)
        }
        controller.onSessionResult(
            sessionId = intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1),
            packageName = packageName,
            status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE),
            message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE),
            confirmIntent = confirm,
        )
    }

    companion object {
        const val ACTION = "groot.mobile.install.SESSION_RESULT"
        const val EXTRA_TARGET_PACKAGE = "groot.mobile.install.TARGET_PACKAGE"
    }
}
