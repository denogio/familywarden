package io.github.denogio.familywarden

import android.app.AlarmManager
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.provider.AlarmClock
import android.telecom.TelecomManager
import android.view.inputmethod.InputMethodManager
import java.time.ZoneId
import java.time.ZonedDateTime

/** Today's local form of the signed v3 app policy (`AppsPolicy.toLocal`); no grants or shared budget yet. */
data class LocalAppPolicy(
    val zone: ZoneId,
    val limitsMillis: Map<String, Long> = emptyMap(),
    val blocked: Set<String> = emptySet(),
    val approved: Set<String> = emptySet(),
    val suspendNewUserApps: Boolean = false,
    val downtime: List<Window> = emptyList(),
    /** Total screen time today including today's bonus; null = no daily limit (D9). */
    val dailyLimitMillis: Long? = null,
    /** Family Link "Unlimited time": never counted, usable after the limit and in downtime. */
    val alwaysAllowed: Set<String> = emptySet(),
) {
    init {
        require(limitsMillis.size <= 256 && blocked.size <= 256 && approved.size <= 512 && alwaysAllowed.size <= 256)
        require(limitsMillis.values.all { it in 0..2 * 86_400_000L })
        require(dailyLimitMillis == null || dailyLimitMillis in 0..2 * 86_400_000L)
    }
}

/** What to do with one app now, and why (the reason drives notifications). */
enum class AppAction(val block: Boolean) {
    ALLOW(false), HOLD(false),
    BLOCKED(true), NOT_APPROVED(true), DOWNTIME(true), DAILY_LIMIT(true), APP_LIMIT(true),
}

/**
 * Family Link order: essential and always-allowed apps work; blocked and unapproved
 * apps never; downtime and the daily limit pause every other app, then per-app limits.
 * HOLD keeps the current state when usage is unknown: it never lifts a pause, and a
 * limit that is already known to be used up still blocks.
 */
fun appAction(policy: LocalAppPolicy, pkg: String, essential: Boolean, userApp: Boolean,
    usedMillis: Long, totalUsedMillis: Long, now: ZonedDateTime, usageReliable: Boolean): AppAction {
    if (essential) return AppAction.ALLOW
    if (pkg in policy.blocked) return AppAction.BLOCKED
    if (pkg in policy.alwaysAllowed) return AppAction.ALLOW
    if (policy.suspendNewUserApps && userApp && pkg !in policy.approved) return AppAction.NOT_APPROVED
    if (policy.downtime.any { Policy.inWindow(it, now.withZoneSameInstant(policy.zone)) }) return AppAction.DOWNTIME
    val daily = policy.dailyLimitMillis
    if (daily != null && totalUsedMillis >= daily) return AppAction.DAILY_LIMIT
    val limit = policy.limitsMillis[pkg]
    if (limit != null && usedMillis >= limit) return AppAction.APP_LIMIT
    if (!usageReliable && (daily != null || limit != null)) return AppAction.HOLD
    return AppAction.ALLOW
}

/** Applies only our tracked suspensions; never blindly unsuspends every package. */
class AppEnforcer(private val context: Context) {
    private val prefs = context.getSharedPreferences("app-enforcement", Context.MODE_PRIVATE)

    /** [failures]: could not (un)pause. [newlyPaused]: paused by this call, with the reason. */
    data class Result(val failures: Set<String>, val newlyPaused: Map<String, AppAction>)

    /** Serialized: the service tick and lifting on policy removal must not interleave. */
    fun reconcile(policy: LocalAppPolicy, used: Map<String, Long>, totalUsedMillis: Long, now: ZonedDateTime,
        usageReliable: Boolean): Result = synchronized(AppEnforcer::class.java) {
        check(Mast.isDeviceOwner(context)) { "device owner required" }
        val pm = context.packageManager
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        val owned = prefs.getStringSet("suspended", emptySet())!!.toMutableSet()
        val safe = essentialPackages()
        val failures = mutableSetOf<String>()
        val paused = mutableMapOf<String, AppAction>()
        // Downtime and the daily limit concern every app, so look at all of them.
        for (info in pm.getInstalledApplications(0)) {
            val pkg = info.packageName
            val userApp = info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) == 0
            // Non-launchable system components are never targets, even if misconfigured.
            val essential = pkg in safe || (!userApp && pm.getLaunchIntentForPackage(pkg) == null)
            val action = appAction(policy, pkg, essential, userApp, used[pkg] ?: 0, totalUsedMillis, now, usageReliable)
            if (action == AppAction.HOLD) continue
            val block = action.block
            if (!block && pkg !in owned) continue
            if (block && pkg in owned && dpm.isPackageSuspended(admin, pkg)) continue
            // Persist intent before suspension, so a crash cannot orphan our restriction.
            val alreadyOwned = pkg in owned
            if (block && !alreadyOwned) {
                owned.add(pkg)
                check(prefs.edit().putStringSet("suspended", owned.toSet()).commit()) { "suspension persistence failed" }
            }
            val failed = dpm.setPackagesSuspended(admin, arrayOf(pkg), block).isNotEmpty()
            if (failed) {
                failures.add(pkg)
                if (block && !alreadyOwned) owned.remove(pkg)
            } else if (block) paused[pkg] = action
            else owned.remove(pkg)
            check(prefs.edit().putStringSet("suspended", owned.toSet()).commit()) { "suspension persistence failed" }
        }
        Result(failures, paused)
    }

    /**
     * Never paused, never counted: calls (incl. emergency), launcher, Settings, keyboard,
     * active admins, FamilyWarden, and the clock app (a paused app cannot play the alarm).
     * Messaging is not built in: as in Family Link it works in downtime only if always allowed.
     */
    fun essentialPackages(): Set<String> {
        val pm = context.packageManager
        fun resolve(intent: Intent): String? = pm.resolveActivity(intent, 0)?.activityInfo?.packageName
        return buildSet {
            add(context.packageName)
            resolve(Intent(android.provider.Settings.ACTION_SETTINGS))?.let(::add)
            pm.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME), 0)
                .forEach { add(it.activityInfo.packageName) }
            resolve(Intent(Intent.ACTION_DIAL, android.net.Uri.parse("tel:112")))?.let(::add)
            add("android"); add("com.android.systemui")
            context.getSystemService(TelecomManager::class.java).defaultDialerPackage?.let(::add)
            resolve(Intent(AlarmClock.ACTION_SHOW_ALARMS))?.let(::add)
            resolve(Intent(AlarmClock.ACTION_SET_ALARM))?.let(::add)
            context.getSystemService(AlarmManager::class.java).nextAlarmClock?.showIntent?.creatorPackage?.let(::add)
            context.getSystemService(InputMethodManager::class.java).inputMethodList.forEach { add(it.packageName) }
            Mast.dpm(context).activeAdmins?.forEach { add(it.packageName) }
        }
    }
}
