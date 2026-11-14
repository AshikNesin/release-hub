package groot.mobile.updates

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import groot.kit.log.Log
import groot.mobile.data.UpdatePrefs
import java.util.concurrent.TimeUnit

/** Keeps the periodic update check in step with the settings: scheduled while connected, cancelled otherwise. */
class UpdateScheduler(context: Context) {
    private val appContext = context.applicationContext

    fun apply(connected: Boolean, prefs: UpdatePrefs) {
        val workManager = try {
            WorkManager.getInstance(appContext)
        } catch (e: IllegalStateException) {
            Log.w(TAG, e) { "WorkManager unavailable" }
            return
        }
        if (!connected) {
            workManager.cancelUniqueWork(WORK_NAME)
            return
        }
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(if (prefs.unmeteredOnly) NetworkType.UNMETERED else NetworkType.CONNECTED)
            .setRequiresBatteryNotLow(true)
            .setRequiresStorageNotLow(true)
            .build()
        val request = PeriodicWorkRequestBuilder<UpdateWorker>(prefs.checkIntervalHours.toLong(), TimeUnit.HOURS)
            .setConstraints(constraints)
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, BACKOFF_MINUTES, TimeUnit.MINUTES)
            .build()
        workManager.enqueueUniquePeriodicWork(WORK_NAME, ExistingPeriodicWorkPolicy.UPDATE, request)
    }

    private companion object {
        const val TAG = "updates"
        const val WORK_NAME = "update-check"
        const val BACKOFF_MINUTES = 15L
    }
}
