package io.github.denogio.familywarden

import android.content.Context
import android.content.pm.ApplicationInfo
import android.util.Log
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Drawable
import java.io.ByteArrayOutputStream
import java.util.Base64
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
    const val MAX_ICON_BYTES = 2048
    const val MAX_ICONS_BYTES = 16 * 1024
    internal const val LEGACY_MS = 24 * 3_600_000L
    private const val REFRESH_MS = 6 * 3_600_000L

    data class Item(val pkg: String, val label: String, val system: Boolean, val essential: Boolean, val icon: String? = null)

    /**
     * Sorted, bounded snapshot. `installed` counts every valid app found; when fewer
     * fit, the router keeps it as partial instead of pretending it is complete.
     */
    fun report(device: String, profile: String, seq: Long, items: Collection<Item>, includeIcons: Boolean = true): String {
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
        head.put("apps", apps)
        // Metadata first: optional images must never displace a package from the list.
        var iconBytes = 0
        if (includeIcons) for (item in sorted.filter { !it.system } + sorted.filter { it.system }) {
            val icon = item.icon ?: continue
            if (icon.length > 2732) continue
            val size = runCatching { Base64.getDecoder().decode(icon).size }.getOrNull() ?: continue
            if (size > MAX_ICON_BYTES || iconBytes + size > MAX_ICONS_BYTES) continue
            val index = (0 until apps.length()).firstOrNull { apps.getJSONObject(it).getString("package") == item.pkg } ?: continue
            val app = apps.getJSONObject(index)
            app.put("icon", icon)
            if (head.toString().toByteArray().size > MAX_BYTES) app.remove("icon") else iconBytes += size
        }
        return head.toString()
    }

    /** User apps, launchable system apps and essential roles; not hidden system components. */
    fun items(context: Context): List<Item> {
        val pm = context.packageManager
        val essential = AppEnforcer(context).essentialPackages()
        return pm.getInstalledApplications(0).mapNotNull { info ->
            val system = info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0
            val pkg = info.packageName
            if (system && pkg !in essential && pm.getLaunchIntentForPackage(pkg) == null) return@mapNotNull null
            Item(pkg, runCatching { pm.getApplicationLabel(info).toString() }.getOrDefault(""), system, pkg in essential,
                runCatching { icon(pm.getApplicationIcon(info)) }.getOrNull())
        }
    }

    internal fun icon(drawable: Drawable): String? {
        val oldBounds = android.graphics.Rect(drawable.bounds)
        val bitmap = Bitmap.createBitmap(96, 96, Bitmap.Config.ARGB_8888)
        try {
            val canvas = Canvas(bitmap)
            canvas.drawColor(android.graphics.Color.rgb(245, 245, 245))
            if (drawable is android.graphics.drawable.AdaptiveIconDrawable) {
                // Preserve Android's 108/72 layer scale without its launcher mask.
                val inset = (96 * android.graphics.drawable.AdaptiveIconDrawable.getExtraInsetFraction()).toInt()
                for (layer in listOfNotNull(drawable.background, drawable.foreground)) {
                    val bounds = android.graphics.Rect(layer.bounds)
                    try { layer.setBounds(-inset, -inset, 96 + inset, 96 + inset); layer.draw(canvas) }
                    finally { layer.bounds = bounds }
                }
            } else {
                val width = drawable.intrinsicWidth.coerceAtLeast(1)
                val height = drawable.intrinsicHeight.coerceAtLeast(1)
                val scale = 96f / maxOf(width, height)
                val w = (width * scale).toInt().coerceAtLeast(1)
                val h = (height * scale).toInt().coerceAtLeast(1)
                drawable.setBounds((96-w)/2, (96-h)/2, (96+w)/2, (96+h)/2)
                drawable.draw(canvas)
            }
            val bytes = ByteArrayOutputStream().also { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }.toByteArray()
            return if (bytes.size <= InventoryIconUpload.MAX_ICON_BYTES) Base64.getEncoder().encodeToString(bytes) else null
        } finally { bitmap.recycle(); drawable.bounds = oldBounds }
    }

    internal fun legacyActive(scope: String, savedScope: String?, until: Long, now: Long): Boolean =
        scope == savedScope && until - now in 1..LEGACY_MS

    // post returns only authenticated, exact-message-bound answers; exceptions never downgrade.
    internal fun deliver(device: String, profile: String, items: Collection<Item>, includeIcons: Boolean,
                         nextSeq: () -> Long, post: (String) -> UploadReply, onLegacy: () -> Unit): UploadReply {
        val raw = report(device, profile, nextSeq(), items, includeIcons)
        val reply = post(raw)
        if (includeIcons && JSONObject(raw).getJSONArray("apps").let { apps ->
                (0 until apps.length()).any { apps.getJSONObject(it).has("icon") }
            } && reply.error == "malformed") {
            onLegacy()
            return post(report(device, profile, nextSeq(), items, includeIcons = false))
        }
        return reply
    }

    /** After a successful sync: on change, else at most every few hours for freshness. */
    fun send(context: Context, store: Store, state: MastState) {
        val binding = state.binding ?: return
        val host = store.host ?: return
        val routerKey = store.publicKey ?: return
        if (state.apps == null) return
        val prefs = context.getSharedPreferences("inventory-upload", Context.MODE_PRIVATE)
        val items = items(context)
        val now = System.currentTimeMillis()
        val scope = "$host|${binding.deviceId}|${binding.profileId}|$routerKey"
        val icons = !legacyActive(scope, prefs.getString("legacy_scope", null), prefs.getLong("legacy_until", 0), now)
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(report(binding.deviceId, binding.profileId, 1, items, icons).toByteArray()).joinToString("") { "%02x".format(it) }
        if (prefs.getString("sent", null) == "$scope|$digest" && now - prefs.getLong("sent_at", 0) in 0 until REFRESH_MS) {
            InventoryIconUpload.send(prefs, scope, now, items, binding.deviceId, binding.profileId) { raw ->
                AppRequestClient.post(host, RequestUpload("inventory_icons", raw, routerKey, binding.deviceId), store.phoneKey, state.serverTime)
            }
            return
        }
        // Durable before sending: a lost reply must never let an old seq be reused.
        var inventorySeq = 0L
        val reply = deliver(binding.deviceId, binding.profileId, items, icons, nextSeq = {
            val seq = prefs.getLong("seq", 0) + 1
            check(prefs.edit().putLong("seq", seq).commit()) { "inventory sequence not saved" }
            inventorySeq = seq
            seq
        }, post = { raw ->
            AppRequestClient.post(host, RequestUpload("inventory", raw, routerKey, binding.deviceId),
                store.phoneKey, state.serverTime)
        }, onLegacy = {
            check(prefs.edit().putString("legacy_scope", scope).putLong("legacy_until", now + LEGACY_MS).commit())
        })
        Log.i("UlyssesMast", "inventory: ${reply.state ?: reply.error}")
        if (reply.state != null) {
            val currentIcons = !legacyActive(scope, prefs.getString("legacy_scope", null), prefs.getLong("legacy_until", 0), now)
            val sentDigest = MessageDigest.getInstance("SHA-256")
                .digest(report(binding.deviceId, binding.profileId, 1, items, currentIcons).toByteArray())
                .joinToString("") { "%02x".format(it) }
            check(prefs.edit().putString("sent", "$scope|$sentDigest").putLong("sent_at", now)
                .putLong("inventory_seq", inventorySeq).commit())
            InventoryIconUpload.send(prefs, scope, now, items, binding.deviceId, binding.profileId) { raw ->
                AppRequestClient.post(host, RequestUpload("inventory_icons", raw, routerKey, binding.deviceId), store.phoneKey, state.serverTime)
            }
        }
    }
}
