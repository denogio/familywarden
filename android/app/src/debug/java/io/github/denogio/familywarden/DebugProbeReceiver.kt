package io.github.denogio.familywarden

import android.app.AppOpsManager
import android.app.admin.DevicePolicyManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Process
import android.os.SystemClock
import android.util.Log

/**
 * Debug builds only: P2 capability experiments (plans/app-control-todo.md), driven by adb.
 *
 *   am broadcast -a io.github.denogio.familywarden.PROBE_SUSPEND --es packages a,b --ez suspended true
 *   am broadcast -a io.github.denogio.familywarden.PROBE_UNINSTALL_BLOCK --es package a --ez blocked true
 *   am broadcast -a io.github.denogio.familywarden.PROBE_USAGE --el since <epoch ms>
 *   am broadcast -a io.github.denogio.familywarden.PROBE_AUTOSUSPEND --ez enabled true
 *   am broadcast -a io.github.denogio.familywarden.PROBE_HIDE --es package a --ez hidden true
 *   am broadcast -a io.github.denogio.familywarden.PROBE_RESTRICT --es restriction no_install_apps --ez set true
 *   am broadcast -a io.github.denogio.familywarden.PROBE_SUPPORT --es message "..."
 *   am broadcast -a io.github.denogio.familywarden.PROBE_PLAY --es allowed_accounts "a@x,b@y"   ("" blocks all; omit to clear)
 *
 * Results go to logcat tag "FwProbe". PACKAGE_ADDED also arrives here: with
 * autosuspend enabled a newly installed app is suspended at once, to measure the
 * install-to-suspend window.
 */
class DebugProbeReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val dpm = context.getSystemService(DevicePolicyManager::class.java)
        val admin = ComponentName(context, AdminReceiver::class.java)
        val prefs = context.getSharedPreferences("probe", Context.MODE_PRIVATE)
        try {
            when (intent.action) {
                "io.github.denogio.familywarden.PROBE_SUSPEND" -> {
                    val packages = intent.getStringExtra("packages").orEmpty().split(',').filter { it.isNotBlank() }
                    val failed = dpm.setPackagesSuspended(admin, packages.toTypedArray(), intent.getBooleanExtra("suspended", true))
                    Log.i(TAG, "suspend ${packages} suspended=${intent.getBooleanExtra("suspended", true)} failed=${failed.toList()}")
                    packages.forEach { Log.i(TAG, "isPackageSuspended $it=${runCatching { dpm.isPackageSuspended(admin, it) }.getOrElse { e -> e.toString() }}") }
                }
                "io.github.denogio.familywarden.PROBE_UNINSTALL_BLOCK" -> {
                    val pkg = intent.getStringExtra("package").orEmpty()
                    dpm.setUninstallBlocked(admin, pkg, intent.getBooleanExtra("blocked", true))
                    Log.i(TAG, "uninstallBlocked $pkg=${dpm.isUninstallBlocked(admin, pkg)}")
                }
                "io.github.denogio.familywarden.PROBE_HIDE" -> {
                    val pkg = intent.getStringExtra("package").orEmpty()
                    val ok = dpm.setApplicationHidden(admin, pkg, intent.getBooleanExtra("hidden", true))
                    Log.i(TAG, "hide $pkg result=$ok isHidden=${dpm.isApplicationHidden(admin, pkg)}")
                }
                "io.github.denogio.familywarden.PROBE_RESTRICT" -> {
                    val r = intent.getStringExtra("restriction").orEmpty()
                    if (intent.getBooleanExtra("set", true)) dpm.addUserRestriction(admin, r) else dpm.clearUserRestriction(admin, r)
                    Log.i(TAG, "restriction $r=${dpm.getUserRestrictions(admin).getBoolean(r)}")
                }
                "io.github.denogio.familywarden.PROBE_SUPPORT" -> {
                    dpm.setShortSupportMessage(admin, intent.getStringExtra("message"))
                    Log.i(TAG, "shortSupportMessage=${dpm.getShortSupportMessage(admin)}")
                }
                "io.github.denogio.familywarden.PROBE_PLAY" -> {
                    val bundle = android.os.Bundle(dpm.getApplicationRestrictions(admin, "com.android.vending"))
                    if (intent.hasExtra("allowed_accounts")) bundle.putString("allowed_accounts", intent.getStringExtra("allowed_accounts").orEmpty())
                    else bundle.remove("allowed_accounts")
                    dpm.setApplicationRestrictions(admin, "com.android.vending", bundle)
                    val now = dpm.getApplicationRestrictions(admin, "com.android.vending")
                    val state = if (!now.containsKey("allowed_accounts")) "unset" else if (now.getString("allowed_accounts").isNullOrEmpty()) "deny-all" else "allowlist (redacted)"
                    Log.i(TAG, "play allowed_accounts=$state")
                }
                "io.github.denogio.familywarden.PROBE_PACKAGE_WATCH" -> {
                    val enabled = intent.getBooleanExtra("enabled", false)
                    check(prefs.edit().putBoolean("package_watch", enabled).commit())
                    val service = Intent(context, PackageWatchProbe::class.java)
                    if (enabled) context.startForegroundService(service) else context.stopService(service)
                }
                Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED -> {
                    if (prefs.getBoolean("package_watch", false))
                        context.startForegroundService(Intent(context, PackageWatchProbe::class.java))
                }
                "io.github.denogio.familywarden.PROBE_AUTOSUSPEND" ->
                    prefs.edit().putBoolean("autosuspend", intent.getBooleanExtra("enabled", false)).apply()
                "io.github.denogio.familywarden.PROBE_USAGE" -> dumpUsage(context, intent.getLongExtra("since", System.currentTimeMillis() - 3_600_000))
                Intent.ACTION_PACKAGE_ADDED -> {
                    val pkg = intent.data?.schemeSpecificPart ?: return
                    val replacing = intent.getBooleanExtra(Intent.EXTRA_REPLACING, false)
                    Log.i(TAG, "PACKAGE_ADDED $pkg replacing=$replacing uptime=${SystemClock.uptimeMillis()}")
                    if (prefs.getBoolean("autosuspend", false) && !replacing) {
                        val failed = dpm.setPackagesSuspended(admin, arrayOf(pkg), true)
                        Log.i(TAG, "autosuspend $pkg failed=${failed.toList()} uptime=${SystemClock.uptimeMillis()}")
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "probe ${intent.action} failed", e)
        }
    }

    private fun dumpUsage(context: Context, since: Long) {
        val ops = context.getSystemService(AppOpsManager::class.java)
        val mode = ops.unsafeCheckOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName)
        Log.i(TAG, "usage access mode=$mode (0=allowed)")
        val events = context.getSystemService(UsageStatsManager::class.java).queryEvents(since, System.currentTimeMillis())
        if (events == null) {
            Log.i(TAG, "queryEvents returned null (user locked?)")
            return
        }
        val e = UsageEvents.Event()
        var n = 0
        while (events.getNextEvent(e)) {
            Log.i(TAG, "event t=${e.timeStamp} type=${e.eventType} pkg=${e.packageName} cls=${e.className}")
            n++
        }
        Log.i(TAG, "usage events: $n")
    }

    private companion object {
        const val TAG = "FwProbe"
    }
}
