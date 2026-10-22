package io.github.denogio.familywarden

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

/** Device-owner receiver. `adb shell dpm set-device-owner io.github.denogio.familywarden/.AdminReceiver` */
class AdminReceiver : DeviceAdminReceiver() {
    override fun onEnabled(context: Context, intent: Intent) {
        // Off the main thread: applying Private DNS waits on the network
        val pending = goAsync()
        Thread {
            try { Mast.refresh(context, sync = false) } finally { pending.finish() }
        }.start()
    }
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
        const val ACTION_TICK = "io.github.denogio.familywarden.TICK"
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
    fun admin(context: Context) = ComponentName(context, AdminReceiver::class.java)

    fun dpm(context: Context) = context.getSystemService(DevicePolicyManager::class.java)

    fun isDeviceOwner(context: Context) = dpm(context).isDeviceOwnerApp(context.packageName)

    fun now(state: MastState?) = ZonedDateTime.now(
        state?.timeZone?.let(java.time.ZoneId::of) ?: java.time.ZoneId.systemDefault())

    fun decide(context: Context): Decision = Store(context).effectiveState.let { Policy.decide(it, now(it)) }

    internal fun accept(store: Store, reply: Sync.Result.Ok) {
        store.accept(reply)
    }

    /** Sync (optional), decide, enforce, and schedule the next exact alarm. */
    @Synchronized
    fun refresh(context: Context, sync: Boolean) {
        val store = Store(context)
        if (sync && store.paired) {
            when (val r = Sync.fetch(store, android.os.Build.MODEL.orEmpty())) {
                is Sync.Result.Ok -> {
                    accept(store, r)
                }
                is Sync.Result.Failed -> store.lastError = r.reason
            }
        }
        val state = store.effectiveState
        val decision = Policy.decide(state, now(state))
        // A damaged enrolled cache is not authorization to clear existing restrictions.
        if (!store.provisioned || state != null) {
            enforce(context, state != null, decision.block)
            scheduleNext(context, store)
            AppControl.sync(context, state)
        }
        // DNS is always on while the verified policy has it, independent of Ulysses
        if (state != null) PrivateDns.apply(context, store, state.dns)
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
        val am = context.getSystemService(AlarmManager::class.java)
        val pi = PendingIntent.getBroadcast(
            context, 0,
            Intent(context, EventReceiver::class.java).setAction(EventReceiver.ACTION_TICK),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val state = store.effectiveState
        val next = Policy.nextChange(state, now(state))
        if (next == null) { am.cancel(pi); return }
        // A second late is fine; a minute late would leave a window to start a hotspot
        am.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, next.toInstant().toEpochMilli() + 1000, pi)
    }

    private fun schedulePeriodic(context: Context) {
        val store = Store(context)
        if (!isDeviceOwner(context) || (!store.paired && !store.provisioned)) {
            WorkManager.getInstance(context).cancelUniqueWork("mast-sync")
            return
        }
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            "mast-sync", ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES).build(),
        )
    }

    /**
     * Follow the router to [address] if it still holds the pinned key and answers this
     * phone's fresh request with its own signed device policy. Nothing else changes:
     * key pin, enrollment, pact and removal rules stay as they are.
     */
    @Synchronized
    fun repairAddress(context: Context, address: String): Boolean {
        val store = Store(context)
        if (!store.provisioned) return false
        val r = Sync.fetch(address, store.publicKey, store.deviceId, store.phoneKey, android.os.Build.MODEL.orEmpty(),
            store.state, requireDevicePolicy = true, appliedRev = store.appsApplied, minWire = store.wireSeen)
        if (r !is Sync.Result.Ok) return false
        store.repairAddress(address, r)
        refresh(context, sync = false)
        return true
    }

    /** Cached eligibility is for presenting an action, never final authorization. */
    fun canRelease(context: Context): Boolean {
        val store = Store(context)
        return !store.provisioned || store.effectiveState?.canSelfRemove(
            store.deviceId, System.currentTimeMillis() / 1000) == true
    }

    /** Enrolled devices require a new signed response bound to this request and device. */
    @Synchronized
    fun checkRelease(context: Context): Boolean {
        val store = Store(context)
        if (!store.provisioned) return true
        val result = Sync.fetch(store, android.os.Build.MODEL.orEmpty())
        if (result !is Sync.Result.Ok) {
            store.lastError = (result as Sync.Result.Failed).reason
            return false
        }
        accept(store, result)
        refresh(context, sync = false)
        // A wrongly advanced phone clock is not fresh server authorization to leave.
        return result.authorizesSelfRemoval(store.deviceId)
    }

    /** Final confirmation repeats authorization; callers cannot bypass it. */
    @Synchronized
    @Suppress("DEPRECATION")
    fun release(context: Context): Boolean {
        if (!checkRelease(context)) return false
        // Hand Private DNS back first; if Android refuses, stay owner (nothing half-done)
        if (!PrivateDns.restore(context, Store(context))) return false
        enforce(context, paired = false, block = false)
        // Lift our own app suspensions while we still are Device Owner.
        AppControl.stop(context, Store(context).effectiveState?.let(AppControl::zone) ?: java.time.ZoneId.systemDefault())
        if (isDeviceOwner(context)) dpm(context).clearDeviceOwnerApp(context.packageName)
        Store(context).unpair()
        scheduleNext(context, Store(context))
        WorkManager.getInstance(context).cancelUniqueWork("mast-sync")
        return true
    }
}
