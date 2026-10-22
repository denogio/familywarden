package io.github.denogio.familywarden

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * Applies the router-signed app policy (wire v3, `MastState.apps`) on this phone:
 * count visible use, suspend what the policy blocks, and acknowledge the revision
 * only once it is fully in force. Runs only while a trusted policy has `apps`.
 */
object AppControl {
    // ponytail: fixed poll while the screen is on; a limit can overshoot by up to this.
    // Nothing counts with the screen off, so nothing polls then. Measure battery on the Pixel.
    const val SCREEN_ON_INTERVAL_MS = 15_000L

    /** Acknowledge only what is fully in force: every action applied and usage trustworthy. */
    fun acknowledges(failures: Set<String>, usageReliable: Boolean) = failures.isEmpty() && usageReliable

    fun zone(state: MastState): ZoneId = state.timeZone?.let(ZoneId::of) ?: ZoneId.systemDefault()

    /**
     * Set by [stop] before lifting. A tick already queued must not suspend again after
     * the lift (e.g. right before removal gives up Device Owner): ticks check this under
     * the same lock as [AppEnforcer.reconcile].
     */
    @Volatile var stopped = false

    /** Match the service to the trusted policy. Only call with a verified state (or none, unprovisioned). */
    fun sync(context: Context, state: MastState?) {
        if (state?.apps != null && Mast.isDeviceOwner(context)) {
            stopped = false
            context.startForegroundService(Intent(context, AppControlService::class.java))
        } else {
            stop(context, state?.let(::zone) ?: ZoneId.systemDefault())
        }
    }

    /** No (more) app policy, or removal: stop and lift only FamilyWarden's own suspensions. */
    fun stop(context: Context, zone: ZoneId) {
        context.stopService(Intent(context, AppControlService::class.java))
        if (Mast.isDeviceOwner(context)) synchronized(AppEnforcer::class.java) {
            stopped = true
            val failed = AppEnforcer(context).reconcile(LocalAppPolicy(zone), emptyMap(), ZonedDateTime.now(zone), false)
            if (failed.isNotEmpty()) Log.w(TAG, "could not lift suspension of ${failed.size} package(s)")
        } else stopped = true
        val store = Store(context)
        if (store.appsApplied != null) store.appsApplied = null
        if (store.appsStatus != null) store.appsStatus = null
    }

    private const val TAG = "UlyssesMast"
}

class AppControlService : Service() {
    private lateinit var worker: HandlerThread
    private lateinit var handler: Handler
    private var monitor: UsageMonitor? = null
    private val tick = Runnable { tick() }
    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) { handler.post(tick) }
    }

    override fun onCreate() {
        super.onCreate()
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel(CHANNEL, getString(R.string.fw_apps_channel), NotificationManager.IMPORTANCE_LOW))
        startForeground(NOTIFICATION, Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_lighthouse_mono).setContentTitle(getString(R.string.fw_apps_notification))
            .setOngoing(true).build())
        worker = HandlerThread("app-control").apply { start() }
        handler = Handler(worker.looper)
        // Protected system broadcasts only: other apps cannot send these.
        register(IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED); addAction(Intent.ACTION_PACKAGE_REPLACED)
            addAction(Intent.ACTION_PACKAGE_REMOVED); addDataScheme("package")
        })
        register(IntentFilter().apply {
            addAction(Intent.ACTION_SCREEN_ON); addAction(Intent.ACTION_SCREEN_OFF); addAction(Intent.ACTION_USER_PRESENT)
            addAction(Intent.ACTION_TIME_CHANGED); addAction(Intent.ACTION_DATE_CHANGED)
        })
    }

    private fun register(filter: IntentFilter) {
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(receiver, filter, null, handler, RECEIVER_EXPORTED)
        else registerReceiver(receiver, filter, null, handler)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        handler.post(tick)
        return START_STICKY
    }

    private fun tick() {
        handler.removeCallbacks(tick)
        try {
            val store = Store(this)
            val state = store.effectiveState
            val apps = state?.apps
            if (state == null) return  // damaged/unverified cache: keep what is in force
            if (apps == null || !Mast.isDeviceOwner(this)) { AppControl.stop(this, AppControl.zone(state)); stopSelf(); return }
            val zone = AppControl.zone(state)
            // A changed router timezone needs a journal migration; until then limits hold.
            if (monitor == null) monitor = runCatching { UsageMonitor(this, zone) }
                .onFailure { Log.w(TAG, "usage journal unavailable: ${it.message}") }.getOrNull()
            val sample = monitor?.let { m -> runCatching { m.sample() }.onFailure { Log.w(TAG, "usage sample failed", it) }.getOrNull() }
            val reliable = sample?.reliable == true
            val failures = synchronized(AppEnforcer::class.java) {
                if (AppControl.stopped) return
                AppEnforcer(this).reconcile(apps.toLocal(zone), sample?.millis ?: emptyMap(), ZonedDateTime.now(zone), reliable)
            }
            val status = when {
                failures.isNotEmpty() -> "suspend_failed"
                sample == null -> "usage_unavailable"
                else -> sample.status
            }
            if (store.appsStatus != status) store.appsStatus = status
            if (AppControl.acknowledges(failures, reliable) && store.appsApplied != apps.rev) store.appsApplied = apps.rev
        } catch (e: Exception) {
            Log.e(TAG, "app control tick failed", e)
        } finally {
            if (getSystemService(PowerManager::class.java).isInteractive)
                handler.postDelayed(tick, AppControl.SCREEN_ON_INTERVAL_MS)
        }
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        unregisterReceiver(receiver)
        handler.removeCallbacksAndMessages(null)
        worker.quitSafely()
        super.onDestroy()
    }

    private companion object {
        const val CHANNEL = "app-control"
        const val NOTIFICATION = 75
        const val TAG = "UlyssesMast"
    }
}
