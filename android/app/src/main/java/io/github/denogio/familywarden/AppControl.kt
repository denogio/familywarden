package io.github.denogio.familywarden

import android.Manifest
import android.app.AppOpsManager
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.app.admin.DevicePolicyManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import android.os.PowerManager
import android.os.Process
import android.os.SystemClock
import io.github.denogio.familywarden.ui.AppUsageReport
import android.util.Log
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * Applies the router-signed app policy (wire v3, `MastState.apps`) on this phone:
 * count visible use, suspend what the policy blocks, warn before limits, and acknowledge
 * the revision only once it is fully in force. Runs only while a trusted policy has `apps`.
 */
object AppControl {
    // ponytail: fixed poll while the screen is on; a limit can overshoot by up to this.
    // Nothing counts with the screen off, so nothing polls then. Measure battery on the Pixel.
    const val SCREEN_ON_INTERVAL_MS = 15_000L
    const val WARN_BEFORE_MS = 5 * 60_000L

    /** In-process only: after process death the UI waits for a new service observation. */
    @Volatile var usageReport: AppUsageReport? = null
        private set

    fun hasUsageAccess(context: Context): Boolean = context.getSystemService(AppOpsManager::class.java)
        .unsafeCheckOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName) == AppOpsManager.MODE_ALLOWED

    fun publish(report: AppUsageReport?) { usageReport = report }

    /** Acknowledge only what is fully in force: every action applied and usage trustworthy. */
    fun acknowledges(failures: Set<String>, usageReliable: Boolean) = failures.isEmpty() && usageReliable

    fun zone(state: MastState): ZoneId = state.timeZone?.let(ZoneId::of) ?: ZoneId.systemDefault()

    /** Limits with at most 5 minutes left today ([UsageCounter.TOTAL] = the daily total). */
    fun almostUsedUp(policy: LocalAppPolicy, used: Map<String, Long>): Set<String> = buildSet {
        fun check(key: String, limit: Long?) {
            val left = (limit ?: return) - (used[key] ?: 0)
            if (left in 1..WARN_BEFORE_MS) add(key)
        }
        check(UsageCounter.TOTAL, policy.dailyLimitMillis)
        policy.limitsMillis.forEach { (pkg, limit) -> check(pkg, limit) }
    }

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
            val dpm = Mast.dpm(context)
            val admin = Mast.admin(context)
            // Body of Android's pause dialog; its title ("Blocked by work policy") is fixed.
            dpm.setShortSupportMessage(admin, context.getString(R.string.fw_apps_support))
            if (Build.VERSION.SDK_INT >= 33) dpm.setPermissionGrantState(admin, context.packageName,
                Manifest.permission.POST_NOTIFICATIONS, DevicePolicyManager.PERMISSION_GRANT_STATE_GRANTED)
            context.startForegroundService(Intent(context, AppControlService::class.java))
        } else {
            stop(context, state?.let(::zone) ?: ZoneId.systemDefault())
        }
    }

    /** No (more) app policy, or removal: stop and lift only FamilyWarden's own suspensions. */
    fun stop(context: Context, zone: ZoneId) {
        usageReport = null
        scheduleGrantExpiry(context, null, null)
        context.stopService(Intent(context, AppControlService::class.java))
        if (Mast.isDeviceOwner(context)) synchronized(AppEnforcer::class.java) {
            stopped = true
            val r = AppEnforcer(context).reconcile(LocalAppPolicy(zone), emptyMap(), 0, ZonedDateTime.now(zone), false)
            if (r.failures.isNotEmpty()) Log.w(TAG, "could not lift suspension of ${r.failures.size} package(s)")
            Mast.dpm(context).setShortSupportMessage(Mast.admin(context), null)
        } else stopped = true
        val store = Store(context)
        if (store.appsApplied != null) store.appsApplied = null
        if (store.appsStatus != null) store.appsStatus = null
    }

    /** Native exact alarm closes an expired window even when the screen stays off. */
    fun scheduleGrantExpiry(context: Context, state: MastState?, clock: PolicyClock?): Boolean = runCatching {
        val am = context.getSystemService(android.app.AlarmManager::class.java)
        val pi = PendingIntent.getBroadcast(context, 77,
            Intent(context, EventReceiver::class.java).setAction(EventReceiver.ACTION_TICK),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val elapsed = SystemClock.elapsedRealtime()
        val boot = android.provider.Settings.Global.getInt(context.contentResolver,
            android.provider.Settings.Global.BOOT_COUNT, -1)
        val now = clock?.now(elapsed, boot)
        val end = if (now == null) null else state?.apps?.grants?.map { it.notAfter * 1000 }?.filter { it > now }?.minOrNull()
        if (end == null) am.cancel(pi)
        else am.setExactAndAllowWhileIdle(android.app.AlarmManager.ELAPSED_REALTIME_WAKEUP, elapsed + end - requireNotNull(now), pi)
        true
    }.onFailure { Log.w(TAG, "grant expiry alarm unavailable", it) }.getOrDefault(false)

    /**
     * Every accepted sync stores a fresh request nonce together with its clock, so an
     * unchanged nonce after all reads proves one consistent policy snapshot. A changed
     * one means a concurrent sync replaced the cache mid-read (e.g. at process start).
     */
    internal fun replacedDuringRead(readNonce: String?, currentPayload: String?): Boolean =
        readNonce != null && runCatching { org.json.JSONObject(requireNotNull(currentPayload)).getString("nonce") }
            .getOrNull() != readNonce

    private const val TAG = "UlyssesMast"
}

class AppControlService : Service() {
    private lateinit var worker: HandlerThread
    private lateinit var handler: Handler
    private var monitor: UsageMonitor? = null
    private var monitorZone: ZoneId? = null
    private val labelCache = mutableMapOf<String, String>()
    private val launchableCache = mutableMapOf<String, Boolean>()
    private val tick = Runnable { tick() }
    private var replacedReads = 0
    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            intent.data?.schemeSpecificPart?.let { labelCache.remove(it); launchableCache.remove(it) }
            handler.post(tick)
        }
    }

    override fun onCreate() {
        super.onCreate()
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(CHANNEL, getString(R.string.fw_apps_channel), NotificationManager.IMPORTANCE_LOW))
        nm.createNotificationChannel(NotificationChannel(ALERTS, getString(R.string.fw_apps_alerts_channel), NotificationManager.IMPORTANCE_DEFAULT))
        startForeground(NOTIFICATION, Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_lighthouse_mono).setContentTitle(getString(R.string.fw_apps_notification))
            .setOngoing(true).build())
        worker = HandlerThread("app-control").apply { start() }
        handler = Handler(worker.looper)
        // Protected system broadcasts only: other apps cannot send these.
        register(IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED); addAction(Intent.ACTION_PACKAGE_REPLACED)
            addAction(Intent.ACTION_PACKAGE_REMOVED); addAction(Intent.ACTION_PACKAGE_CHANGED); addDataScheme("package")
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
            val today = LocalDate.now(zone)
            val basePolicy = apps.toLocal(zone, today)
            val enforcer = AppEnforcer(this)
            // A changed router timezone needs a journal migration; until then limits hold.
            if (monitorZone != zone) { monitor = null; monitorZone = zone }
            if (monitor == null) monitor = runCatching { UsageMonitor(this, zone) }
                .onFailure { Log.w(TAG, "usage journal unavailable: ${it.message}") }.getOrNull()
            val installed = packageManager.getInstalledApplications(0)
            val nonLaunchableSystem = installed.filter {
                it.flags and (android.content.pm.ApplicationInfo.FLAG_SYSTEM or android.content.pm.ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0 &&
                    !launchableCache.getOrPut(it.packageName) { packageManager.getLaunchIntentForPackage(it.packageName) != null }
            }.map { it.packageName }.toSet()
            val essential = enforcer.essentialPackages() + nonLaunchableSystem
            val input = runCatching {
                val payload = org.json.JSONObject(requireNotNull(store.payload))
                check(payload.getString("nonce") == state.requestNonce) { "policy cache changed during enforcement read" }
                val binding = requireNotNull(state.binding)
                val pin = Usign.parsePublicKey(requireNotNull(store.publicKey)).fingerprint
                val users = installed.filter {
                    it.flags and (android.content.pm.ApplicationInfo.FLAG_SYSTEM or android.content.pm.ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) == 0
                }.map { it.packageName }.toSet()
                GrantInput("$pin|${store.phoneKey.fingerprint}|${binding.deviceId}|${binding.profileId}", binding.deviceId,
                    payload.getJSONObject("apps").toString(), store.policyClock(state), users,
                    state.shared?.day.orEmpty(), state.shared?.millis.orEmpty())
            }.onFailure { Log.w(TAG, "grant input unavailable", it) }.getOrNull()
            if (AppControl.replacedDuringRead(state.requestNonce, store.payload)) {
                // Enforcing a mix of two policies could drop an active grant and push the
                // child out of an approved app. Keep what is in force; re-read at once.
                if (++replacedReads <= 3) {
                    Log.i(TAG, "policy replaced during enforcement read; retrying")
                    handler.post(tick)
                    return
                }
            } else replacedReads = 0
            val sample = monitor?.let { m ->
                runCatching { m.sample(essential + basePolicy.alwaysAllowed, input) }
                    .onFailure {
                        // Reload the last atomic checkpoint after a failed write, never
                        // continue with an advanced RAM cursor or unpersisted spending.
                        monitor = null
                        Log.w(TAG, "usage sample failed", it)
                    }.getOrNull()
            }
            val expiryReady = AppControl.scheduleGrantExpiry(this, state, input?.clock)
            val policy = basePolicy.copy(grantRemaining = sample?.grantRemaining.orEmpty(),
                grantReliable = sample?.grantReliable == true && expiryReady)
            val reliable = sample?.reliable == true
            // Shared budget (D4): other phones' usage today only adds to local counters.
            val others = state.shared?.takeIf { sample != null && it.day == sample.day }?.millis.orEmpty()
            val used = (sample?.millis ?: emptyMap()).let { local ->
                local + others.mapValues { (k, v) -> (local[k] ?: 0) + v }
            }
            val result = synchronized(AppEnforcer::class.java) {
                if (AppControl.stopped) return
                enforcer.reconcile(policy, used, used[UsageCounter.TOTAL] ?: 0, ZonedDateTime.now(zone), reliable)
            }
            notify(today.toString(), AppControl.almostUsedUp(policy, used), result.newlyPaused)
            // For the next usage report: apps waiting for a parent's approval (bounded, labelled).
            val waiting = result.waitingApproval.sorted().take(32).joinToString("\n") { "$it\t${UsageUpload.label(label(it)) ?: ""}" }
            val waitPrefs = getSharedPreferences("usage-upload", MODE_PRIVATE)
            if (waitPrefs.getString("new_apps", null) != waiting) waitPrefs.edit().putString("new_apps", waiting).apply()
            val status = when {
                result.failures.isNotEmpty() -> "suspend_failed"
                sample == null -> "usage_unavailable"
                apps.grants.isNotEmpty() && !policy.grantReliable -> "grant_unavailable"
                else -> sample.status
            }
            val historyApps = monitor?.historyApps().orEmpty()
            val labels = (policy.limitsMillis.keys + policy.blocked + policy.alwaysAllowed + policy.approved +
                historyApps.values.flatMap { it.keys }).associateWith(::label)
            AppControl.publish(AppUsageReport(apps.rev, zone.id, today, SystemClock.elapsedRealtime(),
                reliable, used.toMap(), labels, monitor?.historyTotals() ?: emptyMap(), essential,
                sample?.grantRemaining.orEmpty(), policy.grantReliable && result.failures.isEmpty(), installed.map { it.packageName }.toSet(),
                others[UsageCounter.TOTAL] ?: 0, historyApps))
            if (store.appsStatus != status) store.appsStatus = status
            if (AppControl.acknowledges(result.failures, reliable && expiryReady) && store.appsApplied != apps.rev) store.appsApplied = apps.rev
        } catch (e: Exception) {
            AppControl.publish(null)
            Log.e(TAG, "app control tick failed", e)
        } finally {
            if (getSystemService(PowerManager::class.java).isInteractive)
                handler.postDelayed(tick, AppControl.SCREEN_ON_INTERVAL_MS)
        }
    }

    /** "5 minutes left" once per limit and day; "paused" when a limit pauses an app. */
    private fun notify(day: String, almost: Set<String>, paused: Map<String, AppAction>) {
        val prefs = getSharedPreferences("app-alerts", MODE_PRIVATE)
        val sent = prefs.getStringSet("sent", emptySet())!!.filter { it.startsWith("$day|") }.toMutableSet()
        fun once(key: String, title: String) {
            if (!sent.add("$day|$key")) return
            post(key.hashCode(), title)
        }
        almost.forEach { key ->
            once("warn|$key", if (key == UsageCounter.TOTAL) getString(R.string.fw_apps_total_warning)
                else getString(R.string.fw_apps_app_warning, label(key)))
        }
        if (paused.values.any { it == AppAction.DAILY_LIMIT }) once("paused|total", getString(R.string.fw_apps_total_paused))
        paused.filterValues { it == AppAction.APP_LIMIT }.keys.forEach { pkg ->
            once("paused|$pkg", getString(R.string.fw_apps_app_paused, label(pkg)))
        }
        prefs.edit().putStringSet("sent", sent).apply()
    }

    private fun post(id: Int, title: String) {
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        runCatching {
            getSystemService(NotificationManager::class.java).notify(id, Notification.Builder(this, ALERTS)
                .setSmallIcon(R.drawable.ic_lighthouse_mono).setContentTitle(title)
                .setContentIntent(open).setAutoCancel(true).build())
        }
    }

    private fun label(pkg: String): String = labelCache.getOrPut(pkg) {
        runCatching { packageManager.getApplicationLabel(packageManager.getApplicationInfo(pkg, 0)).toString() }.getOrDefault(pkg)
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        AppControl.publish(null)
        unregisterReceiver(receiver)
        handler.removeCallbacksAndMessages(null)
        worker.quitSafely()
        super.onDestroy()
    }

    private companion object {
        const val CHANNEL = "app-control"
        const val ALERTS = "app-limits"
        const val NOTIFICATION = 75
        const val TAG = "UlyssesMast"
    }
}
