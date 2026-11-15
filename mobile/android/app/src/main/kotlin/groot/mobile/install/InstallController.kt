package groot.mobile.install

import android.content.Intent
import android.content.pm.PackageInstaller
import groot.kit.log.Log
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.catalog.CatalogRepository
import groot.mobile.data.releasehub.ReleaseHubManifest
import java.io.File
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

enum class InstallResult {
    Installed,

    /** The person cancelled Android's confirmation. */
    Cancelled,

    /** Android is waiting on the person (confirmation dialog or notification); the result comes later. */
    WaitingForUser,

    /** "Install unknown apps" isn't allowed for this app yet. */
    NeedsPermission,
    Failed,
}

/** How the controller reaches the person: confirmation screens when the app is open, notifications otherwise. */
interface InstallUi {
    fun isForeground(): Boolean

    fun startConfirmation(intent: Intent)

    fun notifyConfirmation(packageName: String, title: String, intent: Intent)

    fun notifyInstalled(packageName: String, title: String, versionName: String)

    fun notifyFailed(packageName: String, title: String, message: String)

    fun notifyNeedsPermission()
}

/**
 * Downloads and installs releases, one at a time. Install progress is published through [tracker];
 * PackageInstaller results come back through [onSessionResult] from [InstallResultReceiver].
 */
class InstallController(
    private val downloader: ApkDownloader,
    private val sessions: SessionInstaller,
    private val tracker: InstallTracker,
    private val catalog: CatalogRepository,
    private val ui: InstallUi,
    private val apkDir: File,
    private val waitMillis: Long = DEFAULT_WAIT_MS,
) {
    private class Pending(val packageName: String, val title: String, val background: Boolean) {
        val result = CompletableDeferred<InstallResult>()
    }

    private val mutex = Mutex()
    private val pending = ConcurrentHashMap<Int, Pending>()

    /**
     * Installs [manifest] (the newest release by default) for [entry]. [background] installs (auto-update)
     * report through notifications instead of screens. Returns once Android has a final answer, or
     * [InstallResult.WaitingForUser] if the person hasn't confirmed within the wait.
     */
    suspend fun install(entry: CatalogEntry, manifest: ReleaseHubManifest? = entry.latest, background: Boolean = false): InstallResult {
        val release = manifest ?: return InstallResult.Failed
        val packageName = entry.packageName
        if (tracker.get(packageName)?.busy == true) return InstallResult.WaitingForUser
        if (!sessions.canInstall()) {
            if (background) ui.notifyNeedsPermission()
            return InstallResult.NeedsPermission
        }
        tracker.set(packageName, InstallState.Queued)
        return mutex.withLock { installLocked(entry, release, background) }
    }

    private suspend fun installLocked(entry: CatalogEntry, release: ReleaseHubManifest, background: Boolean): InstallResult {
        val packageName = entry.packageName
        val apk = File(apkDir, "${entry.slug}-${release.versionCode}.apk")
        try {
            tracker.set(packageName, InstallState.Downloading(null))
            downloader.download(release, apk) { tracker.set(packageName, InstallState.Downloading(it)) }

            tracker.set(packageName, InstallState.Installing)
            val job = Pending(packageName, entry.title, background)
            val sessionId = sessions.commit(apk, packageName) { pending[it] = job }
            val result = withTimeoutOrNull(waitMillis) { job.result.await() } ?: InstallResult.WaitingForUser
            if (result != InstallResult.WaitingForUser) pending.remove(sessionId)
            if (result == InstallResult.Installed && background) ui.notifyInstalled(packageName, entry.title, release.versionName)
            return result
        } catch (e: CancellationException) {
            tracker.clear(packageName)
            throw e
        } catch (e: InstallException) {
            pending.values.removeAll { it.packageName == packageName && !it.result.isCompleted }
            Log.w("install", e) { "${entry.slug}: ${e.message}" }
            fail(packageName, entry.title, e.message ?: "Install failed", background)
            return InstallResult.Failed
        } finally {
            apk.delete()
        }
    }

    /** A PackageInstaller session result (from [InstallResultReceiver]). */
    fun onSessionResult(sessionId: Int, packageName: String, status: Int, message: String?, confirmIntent: Intent?) {
        val job = pending[sessionId]
        val title = job?.title ?: catalog.state.value.entries.firstOrNull { it.packageName == packageName }?.title ?: packageName
        when (status) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                tracker.set(packageName, InstallState.WaitingForUser)
                // A background run doesn't wait on the person: the notification carries the confirmation,
                // and the final result still lands here (the entry stays in `pending` for its title).
                if (job?.background == true) job.result.complete(InstallResult.WaitingForUser)
                if (confirmIntent == null) return
                confirmIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                if (ui.isForeground()) ui.startConfirmation(confirmIntent) else ui.notifyConfirmation(packageName, title, confirmIntent)
            }
            PackageInstaller.STATUS_SUCCESS -> {
                tracker.clear(packageName)
                catalog.refreshInstalled(packageName)
                finish(sessionId, InstallResult.Installed)
            }
            PackageInstaller.STATUS_FAILURE_ABORTED -> {
                tracker.clear(packageName)
                finish(sessionId, InstallResult.Cancelled)
            }
            else -> {
                fail(packageName, title, installFailureMessage(status, message), job?.background ?: !ui.isForeground())
                finish(sessionId, InstallResult.Failed)
            }
        }
    }

    /** Whether "Install unknown apps" is allowed for this app. */
    fun canInstall(): Boolean = sessions.canInstall()

    /** Forget a failure once the person has seen it. */
    fun dismissError(packageName: String) {
        if (tracker.get(packageName) is InstallState.Failed) tracker.clear(packageName)
    }

    private fun finish(sessionId: Int, result: InstallResult) {
        pending.remove(sessionId)?.result?.complete(result)
    }

    private fun fail(packageName: String, title: String, message: String, background: Boolean) {
        tracker.set(packageName, InstallState.Failed(message))
        if (background) ui.notifyFailed(packageName, title, message)
    }

    companion object {
        /** How long an install waits for the person to confirm before the queue moves on. */
        const val DEFAULT_WAIT_MS = 10 * 60 * 1000L
    }
}

/** A sentence for a PackageInstaller failure; Android's own message is kept for the details. */
fun installFailureMessage(status: Int, message: String?): String {
    val raw = message.orEmpty()
    return when {
        "DOWNGRADE" in raw -> "Android doesn't install an older version over a newer one. Uninstall first."
        "UPDATE_INCOMPATIBLE" in raw || status == PackageInstaller.STATUS_FAILURE_CONFLICT ->
            "The installed app is signed with a different key. Uninstall it, then install from the release hub."
        status == PackageInstaller.STATUS_FAILURE_INCOMPATIBLE -> "This APK isn't compatible with this device."
        status == PackageInstaller.STATUS_FAILURE_STORAGE -> "Not enough storage to install."
        status == PackageInstaller.STATUS_FAILURE_BLOCKED -> "The install was blocked on this device."
        status == PackageInstaller.STATUS_FAILURE_INVALID -> "The download isn't a valid APK."
        raw.isNotEmpty() -> "Install failed: $raw"
        else -> "Install failed."
    }
}
