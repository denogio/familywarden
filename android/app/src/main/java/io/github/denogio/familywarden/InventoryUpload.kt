package io.github.denogio.familywarden

import android.content.Context
import android.content.pm.ApplicationInfo
import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
import java.security.MessageDigest

/**
 * This phone's installed apps, so a parent can choose from names instead of package
 * IDs (plans/android-installed-apps.md I1). Observation only: the router never
 * loosens, approves or acknowledges anything from it. Best effort, no outbox.
 */
object InventoryUpload {
    const val MAX_APPS = 512
    /** Router transport limit is 64 KiB including the signature line; keep headroom. */
    const val MAX_BYTES = 60 * 1024
    private const val REFRESH_MS = 6 * 3_600_000L

    data class Item(val pkg: String, val label: String, val system: Boolean, val essential: Boolean)

    /**
     * Sorted, bounded snapshot. `installed` counts every valid app found; when fewer
     * fit, the router keeps it as partial instead of pretending it is complete.
     */
    fun report(device: String, profile: String, seq: Long, items: Collection<Item>): String {
        require(seq >= 1)
        val sorted = items.filter { validPackage(it.pkg) }.distinctBy { it.pkg }.sortedBy { it.pkg }
        val head = JSONObject().put("type", "inventory").put("v", 1).put("device", device).put("profile", profile)
            .put("seq", seq).put("installed", sorted.size)
        val apps = JSONArray()
        var bytes = head.toString().toByteArray().size + 16
        for (item in sorted) {
            if (apps.length() >= MAX_APPS) break
            val o = JSONObject().put("package", item.pkg).put("label", UsageUpload.label(item.label) ?: "")
                .put("system", item.system).put("essential", item.essential)
            val size = o.toString().toByteArray().size + 1
            if (bytes + size > MAX_BYTES) break
            apps.put(o)
            bytes += size
        }
        return head.put("apps", apps).toString()
    }

    /** User apps, launchable system apps and essential roles; not hidden system components. */
    fun items(context: Context): List<Item> {
        val pm = context.packageManager
        val essential = AppEnforcer(context).essentialPackages()
        return pm.getInstalledApplications(0).mapNotNull { info ->
            val system = info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0
            val pkg = info.packageName
            if (system && pkg !in essential && pm.getLaunchIntentForPackage(pkg) == null) return@mapNotNull null
            Item(pkg, runCatching { pm.getApplicationLabel(info).toString() }.getOrDefault(""), system, pkg in essential)
        }
    }

    /** After a successful sync: on change, else at most every few hours for freshness. */
    fun send(context: Context, store: Store, state: MastState) {
        val binding = state.binding ?: return
        val host = store.host ?: return
        val routerKey = store.publicKey ?: return
        if (state.apps == null) return
        val prefs = context.getSharedPreferences("inventory-upload", Context.MODE_PRIVATE)
        val items = items(context)
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(items.sortedBy { it.pkg }.joinToString("\n").toByteArray()).joinToString("") { "%02x".format(it) }
        val now = System.currentTimeMillis()
        val scope = "${binding.deviceId}|${binding.profileId}|$routerKey"
        if (prefs.getString("sent", null) == "$scope|$digest" && now - prefs.getLong("sent_at", 0) in 0 until REFRESH_MS) return
        // Durable before sending: a lost reply must never let an old seq be reused.
        val seq = prefs.getLong("seq", 0) + 1
        check(prefs.edit().putLong("seq", seq).commit()) { "inventory sequence not saved" }
        val raw = report(binding.deviceId, binding.profileId, seq, items)
        val reply = AppRequestClient.post(host, RequestUpload("inventory", raw, routerKey, binding.deviceId),
            store.phoneKey, state.serverTime)
        Log.i("UlyssesMast", "inventory: ${reply.state ?: reply.error}")
        if (reply.state != null) prefs.edit().putString("sent", "$scope|$digest").putLong("sent_at", now).apply()
    }
}
