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
import android.util.Log
import java.time.ZoneId
import java.time.ZonedDateTime

/** Test-only activation of the real collector/enforcer; no unsigned activation in release.
 * Only our synthetic package is managed. One-second polling is for latency experiments,
 * not a measured/approved battery configuration for a physical phone.
 */
class AppControlProbe : Service() {
    private lateinit var worker: HandlerThread
    private lateinit var handler: Handler
    private var monitor: UsageMonitor? = null
    @Volatile private var running = true
    private val zone = ZoneId.of("Europe/Copenhagen")
    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.data?.schemeSpecificPart == PackageWatchProbe.FIXTURE)
                handler.post { tick(false) }
        }
    }

    override fun onCreate() {
        super.onCreate()
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel("app-control-probe", "App control experiment", NotificationManager.IMPORTANCE_LOW))
        startForeground(74, Notification.Builder(this, "app-control-probe")
            .setSmallIcon(R.drawable.ic_lighthouse_mono).setContentTitle("FamilyWarden usage experiment").build())
        worker = HandlerThread("app-control-probe").apply { start() }
        handler = Handler(worker.looper)
        val filter = IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED); addAction(Intent.ACTION_PACKAGE_REMOVED)
            addAction(Intent.ACTION_PACKAGE_REPLACED); addDataScheme("package")
        }
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(receiver, filter, RECEIVER_EXPORTED)
        else registerReceiver(receiver, filter)
        handler.post { tick(true) }
    }

    private fun tick(reschedule: Boolean) {
        if (!running) return
        try {
            val prefs = getSharedPreferences("probe", MODE_PRIVATE)
            if (!prefs.getBoolean("app_control", false)) return
            if (monitor == null) monitor = UsageMonitor(this, zone)
            val sample = monitor!!.sample()
            val pkg = PackageWatchProbe.FIXTURE
            val policy = LocalAppPolicy(zone, limitsMillis = mapOf(pkg to prefs.getInt("app_control_seconds", 30) * 1000L),
                blocked = if (prefs.getBoolean("app_control_block", false)) setOf(pkg) else emptySet())
            val failures = if (running) AppEnforcer(this).reconcile(policy, sample.millis, ZonedDateTime.now(zone), sample.reliable) else emptySet()
            Log.i("FwAppControlProbe", "day=${sample.day} fixtureMs=${sample.millis[pkg] ?: 0} seq=${sample.sequence} status=${sample.status} reliable=${sample.reliable} gaps=${sample.gaps} failures=${failures.size}")
        } catch (e: Exception) {
            // No reset-to-zero on corruption/overflow/storage errors; existing blocks stay.
            Log.e("FwAppControlProbe", "collector/enforcement failed", e)
        } finally {
            if (running && reschedule) handler.postDelayed({ tick(true) }, 1000)
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int) = START_STICKY
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onDestroy() {
        running = false
        unregisterReceiver(receiver)
        handler.removeCallbacksAndMessages(null)
        handler.post {
            try {
                if (!getSharedPreferences("probe", MODE_PRIVATE).getBoolean("app_control", false))
                    AppEnforcer(this).reconcile(LocalAppPolicy(zone), emptyMap(), ZonedDateTime.now(zone), false)
            } finally { worker.quitSafely() }
        }
        super.onDestroy()
    }
}
