package io.github.denogio.familywarden

import android.app.Activity
import android.app.ActivityManager
import android.app.ActivityOptions
import android.app.admin.DevicePolicyManager
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.widget.TextView

/** Reversible emulator-only research. Never called from release or at boot. */
object LockTaskProbe {
    fun set(context: Context, enabled: Boolean) {
        check(Build.HARDWARE in setOf("ranchu", "goldfish")) { "emulator only" }
        check(Mast.isDeviceOwner(context)) { "Device Owner required" }
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        val prefs = context.getSharedPreferences("lock-task-probe", Context.MODE_PRIVATE)
        val bootPrefs = context.createDeviceProtectedStorageContext().getSharedPreferences("lock-task-probe", Context.MODE_PRIVATE)
        if (!enabled) {
            check(bootPrefs.edit().clear().commit())
            if (!prefs.getBoolean("active", false)) return
            if (context.getSystemService(ActivityManager::class.java).lockTaskModeState == ActivityManager.LOCK_TASK_MODE_NONE) restore(context)
            else {
                val activity = if (prefs.getBoolean("home_probe", false)) LockTaskHomeProbe::class.java else LockTaskProbeActivity::class.java
                context.startActivity(Intent(context, activity).putExtra("stop_probe", true)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP))
            }
            return
        }
        check(!prefs.getBoolean("active", false)) { "already active; disable first" }
        check(context.getSystemService(ActivityManager::class.java).lockTaskModeState == ActivityManager.LOCK_TASK_MODE_NONE) { "existing lock task must not be disturbed" }
        val home = context.packageManager.resolveActivity(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME), 0)?.activityInfo
            ?: error("no default launcher")
        val launcher = home.packageName
        check(prefs.edit().putBoolean("active", true)
            .putBoolean("home_probe", home.name == LockTaskHomeProbe::class.java.name)
            .putInt("features", dpm.getLockTaskFeatures(admin))
            .putStringSet("packages", dpm.getLockTaskPackages(admin).toSet()).commit())
        try {
            dpm.setLockTaskFeatures(admin, DevicePolicyManager.LOCK_TASK_FEATURE_HOME or
                DevicePolicyManager.LOCK_TASK_FEATURE_OVERVIEW or DevicePolicyManager.LOCK_TASK_FEATURE_NOTIFICATIONS or
                DevicePolicyManager.LOCK_TASK_FEATURE_SYSTEM_INFO or DevicePolicyManager.LOCK_TASK_FEATURE_KEYGUARD or
                DevicePolicyManager.LOCK_TASK_FEATURE_GLOBAL_ACTIONS or DevicePolicyManager.LOCK_TASK_FEATURE_BLOCK_ACTIVITY_START_IN_TASK)
            // Play remains installed/enabled but is deliberately NOT activity-allowlisted.
            dpm.setLockTaskPackages(admin, setOf(context.packageName, launcher, "com.google.android.gm", "com.google.android.youtube", "com.google.android.apps.docs", "com.google.android.apps.maps").toTypedArray())
            check(bootPrefs.edit().putBoolean("home_boot", home.name == LockTaskHomeProbe::class.java.name).commit())
            val options = ActivityOptions.makeBasic().apply { setLockTaskEnabled(true) }
            val activity = if (home.name == LockTaskHomeProbe::class.java.name)
                Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME).setClass(context, LockTaskHomeProbe::class.java)
            else Intent(context, LockTaskProbeActivity::class.java)
            context.startActivity(activity.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK), options.toBundle())
            Log.i("FwLockTaskProbe", "configured")
        } catch (e: Exception) {
            set(context, false)
            throw e
        }
    }

    fun restore(context: Context) {
        check(context.getSystemService(ActivityManager::class.java).lockTaskModeState == ActivityManager.LOCK_TASK_MODE_NONE) { "exit locked root before restoring policy" }
        val prefs = context.getSharedPreferences("lock-task-probe", Context.MODE_PRIVATE)
        if (!prefs.getBoolean("active", false)) return
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        check(context.createDeviceProtectedStorageContext().getSharedPreferences("lock-task-probe", Context.MODE_PRIVATE).edit().clear().commit())
        dpm.setLockTaskPackages(admin, emptyArray())
        dpm.setLockTaskFeatures(admin, prefs.getInt("features", DevicePolicyManager.LOCK_TASK_FEATURE_GLOBAL_ACTIONS))
        dpm.setLockTaskPackages(admin, prefs.getStringSet("packages", emptySet())!!.toTypedArray())
        check(prefs.edit().clear().commit())
        Log.i("FwLockTaskProbe", "restored")
    }
}

open class LockTaskProbeActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        check(Build.HARDWARE in setOf("ranchu", "goldfish")) { "emulator only" }
        setContentView(TextView(this).apply { text = "FamilyWarden emulator-only lock-task experiment" })
        handle(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent) {
        if (intent.getBooleanExtra("stop_probe", false)) {
            stopLockTask()
            window.decorView.postDelayed({
                if (getSystemService(ActivityManager::class.java).lockTaskModeState == ActivityManager.LOCK_TASK_MODE_NONE) {
                    LockTaskProbe.restore(this)
                    finish()
                } else Log.e("FwLockTaskProbe", "locked root did not exit; leave snapshot for emulator recovery")
            }, 500)
        } else if (intent.getBooleanExtra("try_fixture", false)) {
            Log.i("FwLockTaskProbe", "attempting non-allowlisted activity inside allowlisted task")
            try {
                startActivity(Intent().setClassName(PackageWatchProbe.FIXTURE, "${PackageWatchProbe.FIXTURE}.ProbeActivity"))
            } catch (_: SecurityException) {
                Log.i("FwLockTaskProbe", "in-task activity rejected")
            }
        }
    }
}
