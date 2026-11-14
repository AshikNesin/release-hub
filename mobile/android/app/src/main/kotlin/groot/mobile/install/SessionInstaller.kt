package groot.mobile.install

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import java.io.File

/** Hands an APK to Android's installer. The result arrives later, at [InstallResultReceiver]. */
interface SessionInstaller {
    /**
     * Commits [apk] for [packageName] and returns the session id. [onSession] gets the id before the
     * commit, so the result (which can arrive at once) always finds its install registered.
     */
    fun commit(apk: File, packageName: String, onSession: (Int) -> Unit): Int

    /** Whether the person has allowed this app to install apps ("Install unknown apps"). */
    fun canInstall(): Boolean
}

/**
 * [SessionInstaller] over PackageInstaller sessions. Asks for no user action (API 31+): Android skips the
 * confirmation for updates of apps this app installed, when it holds UPDATE_PACKAGES_WITHOUT_USER_ACTION.
 * Everything else comes back as STATUS_PENDING_USER_ACTION and is confirmed by the person.
 */
class PackageInstallerSessionInstaller(context: Context) : SessionInstaller {
    private val appContext = context.applicationContext
    private val installer: PackageInstaller = appContext.packageManager.packageInstaller

    override fun commit(apk: File, packageName: String, onSession: (Int) -> Unit): Int {
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(packageName)
            setSize(apk.length())
            setInstallReason(android.content.pm.PackageManager.INSTALL_REASON_USER)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                setPackageSource(PackageInstaller.PACKAGE_SOURCE_STORE)
            }
        }
        val sessionId = installer.createSession(params)
        onSession(sessionId)
        try {
            installer.openSession(sessionId).use { session ->
                apk.inputStream().use { input ->
                    session.openWrite("base.apk", 0, apk.length()).use { output ->
                        input.copyTo(output)
                        session.fsync(output)
                    }
                }
                session.commit(statusReceiver(sessionId, packageName).intentSender)
            }
        } catch (e: Exception) {
            runCatching { installer.abandonSession(sessionId) }
            throw InstallException("Android refused the install: ${e.message ?: e.javaClass.simpleName}", e)
        }
        return sessionId
    }

    override fun canInstall(): Boolean = appContext.packageManager.canRequestPackageInstalls()

    private fun statusReceiver(sessionId: Int, packageName: String): PendingIntent {
        val intent = Intent(appContext, InstallResultReceiver::class.java)
            .setAction(InstallResultReceiver.ACTION)
            .putExtra(InstallResultReceiver.EXTRA_TARGET_PACKAGE, packageName)
        // Mutable: the installer fills in the status extras.
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or
            (if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) PendingIntent.FLAG_MUTABLE else 0)
        return PendingIntent.getBroadcast(appContext, sessionId, intent, flags)
    }
}
