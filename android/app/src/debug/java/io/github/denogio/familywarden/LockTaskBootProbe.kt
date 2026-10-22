package io.github.denogio.familywarden

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log

/** Research only: enter native Home lock-task before credential unlock, not BOOT_COMPLETED.
 * Reads only a device-protected experiment flag and system policy, never
 * credential-encrypted preferences or account/pairing keys.
 */
class LockTaskBootProbe : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (Build.HARDWARE !in setOf("ranchu", "goldfish")) return
        val enabled = context.createDeviceProtectedStorageContext().getSharedPreferences("lock-task-probe", Context.MODE_PRIVATE).getBoolean("home_boot", false)
        val owner = Mast.isDeviceOwner(context)
        val permitted = Mast.dpm(context).isLockTaskPermitted(context.packageName)
        Log.i("FwLockTaskProbe", "Direct Boot receiver enabled=$enabled owner=$owner permitted=$permitted")
        if (!enabled || !owner || !permitted) return
        context.startActivity(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME)
            .setClass(context, LockTaskHomeProbe::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        Log.i("FwLockTaskProbe", "Direct Boot Home requested before credential unlock")
    }
}
