package me.hojgaard.ulysses.mast

import android.app.AlarmManager
import android.app.PendingIntent
import android.app.admin.DeviceAdminReceiver
import android.app.admin.DevicePolicyManager
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.UserManager
import android.util.Log
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import java.time.ZonedDateTime
import java.util.concurrent.TimeUnit

private const val TAG = "UlyssesMast"

/** Device-owner receiver. `adb shell dpm set-device-owner me.hojgaard.ulysses.mast/.MastAdminReceiver` */
class MastAdminReceiver : DeviceAdminReceiver() {
    override fun onEnabled(context: Context, intent: Intent) = Mast.refresh(context, sync = false)
}

/** Re-evaluate after boot, app update, and at the exact alarm for the next change. */
class EventReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val pending = goAsync()
        Thread {
            try {
                Mast.refresh(context, sync = intent.action != ACTION_TICK)
            } finally {
                pending.finish()
            }
        }.start()
    }

    companion object {
        const val ACTION_TICK = "me.hojgaard.ulysses.mast.TICK"
    }
}

/** Every 15 minutes: sync if the router is reachable, then re-evaluate. */
class SyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        Mast.refresh(applicationContext, sync = true)
        return Result.success()
    }
}

object Mast {
    fun admin(context: Context) = ComponentName(context, MastAdminReceiver::class.java)

    fun dpm(context: Context) = context.getSystemService(DevicePolicyManager::class.java)

    fun isDeviceOwner(context: Context) = dpm(context).isDeviceOwnerApp(context.packageName)

    fun decide(context: Context): Decision = Policy.decide(Store(context).state, ZonedDateTime.now())

    /** Sync (optional), decide, enforce, and schedule the next exact alarm. */
    @Synchronized
    fun refresh(context: Context, sync: Boolean) {
        val store = Store(context)
        if (sync && store.paired) {
            when (val r = Sync.fetch(store)) {
                is Sync.Result.Ok -> {
                    store.payload = r.payload
                    store.lastSync = System.currentTimeMillis() / 1000
                    store.lastError = null
                }
                is Sync.Result.Failed -> store.lastError = r.reason
            }
        }
        val decision = Policy.decide(store.state, ZonedDateTime.now())
        enforce(context, store.paired, decision.block)
        scheduleNext(context, store)
        schedulePeriodic(context)
        Log.i(TAG, "decision=${decision.reason} block=${decision.block} lastSync=${store.lastSync} error=${store.lastError}")
    }

    /**
     * Tethering restriction follows the decision. Android itself stops running
     * hotspot/USB/Bluetooth tethering when it is added and greys out the
     * quick-settings tile. While paired, the clock is locked to network time so
     * moving it into an allowed window is no way out.
     */
    fun enforce(context: Context, paired: Boolean, block: Boolean) {
        if (!isDeviceOwner(context)) return
        val dpm = dpm(context)
        val admin = admin(context)
        // While blocked, adb is off too (no `cmd wifi start-softap`, no fiddling);
        // outside the blocked hours it works, so the app can still be updated.
        // (Debug builds keep adb, so the emulator test can watch the restrictions.)
        val restrictions = listOf(UserManager.DISALLOW_CONFIG_TETHERING) +
            if (BuildConfig.DEBUG) emptyList() else listOf(UserManager.DISALLOW_DEBUGGING_FEATURES)
        for (r in restrictions) {
            if (block) dpm.addUserRestriction(admin, r) else dpm.clearUserRestriction(admin, r)
        }
        if (paired) {
            dpm.setAutoTimeEnabled(admin, true)
            dpm.setAutoTimeZoneEnabled(admin, true)
            dpm.addUserRestriction(admin, UserManager.DISALLOW_CONFIG_DATE_TIME)
        } else {
            dpm.clearUserRestriction(admin, UserManager.DISALLOW_CONFIG_DATE_TIME)
        }
    }

    private fun scheduleNext(context: Context, store: Store) {
        val next = Policy.nextChange(store.state, ZonedDateTime.now()) ?: return
        val am = context.getSystemService(AlarmManager::class.java)
        val pi = PendingIntent.getBroadcast(
            context, 0,
            Intent(context, EventReceiver::class.java).setAction(EventReceiver.ACTION_TICK),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        // A second late is fine; a minute late would leave a window to start a hotspot
        am.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, next.toInstant().toEpochMilli() + 1000, pi)
    }

    private fun schedulePeriodic(context: Context) {
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            "mast-sync", ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES).build(),
        )
    }

    /** Leaving is only possible when the router says there is no pact. */
    fun canRelease(context: Context): Boolean {
        val state = Store(context).state ?: return true
        return !Policy.pactActive(state, System.currentTimeMillis() / 1000)
    }

    /** Lift all restrictions and stop being device owner, so the app can be uninstalled. */
    @Suppress("DEPRECATION")
    fun release(context: Context) {
        Store(context).unpair()
        enforce(context, paired = false, block = false)
        if (isDeviceOwner(context)) dpm(context).clearDeviceOwnerApp(context.packageName)
    }
}
