package io.github.denogio.familywarden

import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.provider.Telephony
import android.telecom.TelecomManager
import android.view.inputmethod.InputMethodManager
import java.time.ZoneId
import java.time.ZonedDateTime

/** Local form of the signed v3 app policy (`AppsPolicy.toLocal`); no grants or shared budget yet. */
data class LocalAppPolicy(
    val zone: ZoneId,
    val limitsMillis: Map<String, Long> = emptyMap(),
    val blocked: Set<String> = emptySet(),
    val approved: Set<String> = emptySet(),
    val suspendNewUserApps: Boolean = false,
    val downtime: List<Window> = emptyList(),
) {
    init {
        require(limitsMillis.size <= 256 && blocked.size <= 256 && approved.size <= 512)
        require(limitsMillis.values.all { it in 0..86_400_000L })
    }
}

enum class AppAction { ALLOW, BLOCK, HOLD }

/** Essential safety wins even over an explicit block. HOLD never clears an old block
 * merely because usage permission or history is missing.
 */
fun appAction(policy: LocalAppPolicy, pkg: String, essential: Boolean, userApp: Boolean,
    usedMillis: Long, now: ZonedDateTime, usageReliable: Boolean): AppAction {
    if (essential) return AppAction.ALLOW
    if (pkg in policy.blocked) return AppAction.BLOCK
    if (policy.suspendNewUserApps && userApp && pkg !in policy.approved) return AppAction.BLOCK
    val managed = pkg in policy.limitsMillis || pkg in policy.approved
    if (!managed) return AppAction.ALLOW
    if (policy.downtime.any { Policy.inWindow(it, now.withZoneSameInstant(policy.zone)) }) return AppAction.BLOCK
    val limit = policy.limitsMillis[pkg] ?: return AppAction.ALLOW
    if (usedMillis >= limit) return AppAction.BLOCK
    return if (usageReliable) AppAction.ALLOW else AppAction.HOLD
}

/** Applies only our tracked suspensions; never blindly unsuspends every package. */
class AppEnforcer(private val context: Context) {
    private val prefs = context.getSharedPreferences("app-enforcement", Context.MODE_PRIVATE)

    /** Serialized: the service tick and lifting on policy removal must not interleave. */
    fun reconcile(policy: LocalAppPolicy, used: Map<String, Long>, now: ZonedDateTime,
        usageReliable: Boolean): Set<String> = synchronized(AppEnforcer::class.java) {
        check(Mast.isDeviceOwner(context)) { "device owner required" }
        val pm = context.packageManager
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        val owned = prefs.getStringSet("suspended", emptySet())!!.toMutableSet()
        val installed = pm.getInstalledApplications(0).associateBy { it.packageName }
        val safe = essentialPackages()
        val failures = mutableSetOf<String>()
        val targets = policy.blocked + policy.limitsMillis.keys + policy.approved + owned +
            if (policy.suspendNewUserApps) installed.keys else emptySet()
        for (pkg in targets) {
            val info = installed[pkg] ?: continue
            val userApp = info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) == 0
            // Non-launchable system components are never targets, even if misconfigured.
            val essential = pkg in safe || (!userApp && pm.getLaunchIntentForPackage(pkg) == null)
            val action = appAction(policy, pkg, essential, userApp, used[pkg] ?: 0, now, usageReliable)
            if (action == AppAction.HOLD) continue
            val block = action == AppAction.BLOCK
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
            } else if (!block) owned.remove(pkg)
            check(prefs.edit().putStringSet("suspended", owned.toSet()).commit()) { "suspension persistence failed" }
        }
        failures
    }

    private fun essentialPackages(): Set<String> {
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
            Telephony.Sms.getDefaultSmsPackage(context)?.let(::add)
            context.getSystemService(InputMethodManager::class.java).inputMethodList.forEach { add(it.packageName) }
            Mast.dpm(context).activeAdmins?.forEach { add(it.packageName) }
        }
    }
}
