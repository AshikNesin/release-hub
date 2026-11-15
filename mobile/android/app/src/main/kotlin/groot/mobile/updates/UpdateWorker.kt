package groot.mobile.updates

import android.content.Context
import android.os.Build
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import groot.kit.log.Log
import groot.mobile.GrootMobileApp
import groot.mobile.data.releasehub.ReleaseHubException

/** The periodic background check scheduled by [UpdateScheduler]. Exits early when there's no release hub. */
class UpdateWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val container = (applicationContext as GrootMobileApp).container
        return try {
            val summary = container.updateRunner.run()
            Log.i(TAG) { "checked ${summary.checked}, installed ${summary.installed.size}, pending ${summary.pending.size}" }
            Result.success()
        } catch (e: ReleaseHubException) {
            Log.w(TAG, e) { "check failed: ${e.message}" }
            // A rejected token won't fix itself; anything else (network) is worth a retry.
            if (e.unauthorized || runAttemptCount >= MAX_ATTEMPTS) Result.failure() else Result.retry()
        } finally {
            if (isStopped && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) Log.i(TAG) { "stopped, reason $stopReason" }
        }
    }

    private companion object {
        const val TAG = "updates"
        const val MAX_ATTEMPTS = 3
    }
}
