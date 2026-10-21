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
import android.os.IBinder
import android.os.SystemClock
import android.util.Log

/** Disposable P2 experiment: only ever suspends our synthetic test package. */
class PackageWatchProbe : Service() {
    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            val pkg = intent.data?.schemeSpecificPart ?: return
            if (pkg != FIXTURE) return
            Log.i("FwPackageProbe", "event=${intent.action} replacing=${intent.getBooleanExtra(Intent.EXTRA_REPLACING, false)} uptime=${SystemClock.uptimeMillis()}")
            if (intent.action != Intent.ACTION_PACKAGE_REMOVED) suspendFixture()
        }
    }

    private fun suspendFixture() {
        if (!packageManager.getInstalledApplications(0).any { it.packageName == FIXTURE }) return
        val failed = Mast.dpm(this).setPackagesSuspended(Mast.admin(this), arrayOf(FIXTURE), true)
        Log.i("FwPackageProbe", "suspended failed=${failed.toList()} uptime=${SystemClock.uptimeMillis()}")
    }

    override fun onCreate() {
        super.onCreate()
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel("package-probe", "Package event experiment", NotificationManager.IMPORTANCE_LOW))
        startForeground(73, Notification.Builder(this, "package-probe")
            .setSmallIcon(R.drawable.ic_lighthouse_mono).setContentTitle("FamilyWarden package experiment").build())
        val filter = IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED)
            addAction(Intent.ACTION_PACKAGE_REMOVED)
            addAction(Intent.ACTION_PACKAGE_REPLACED)
            addDataScheme("package")
        }
        // These are protected system actions; other apps cannot forge package events.
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(receiver, filter, RECEIVER_EXPORTED)
        else registerReceiver(receiver, filter)
        suspendFixture() // Includes missed events / reboot recovery.
        Log.i("FwPackageProbe", "watching uptime=${SystemClock.uptimeMillis()}")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int) = START_STICKY
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onDestroy() { unregisterReceiver(receiver); super.onDestroy() }

    companion object { const val FIXTURE = "io.github.denogio.familywarden.fixture" }
}
