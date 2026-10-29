package io.github.denogio.familywarden

import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject
import java.security.MessageDigest
import java.util.Base64

/** Regenerable, bounded icon batches; never consume inventory metadata space. */
object InventoryIconUpload {
    const val MAX_ICON_BYTES = 40 * 1024
    private const val RETRY_MS = 6 * 3_600_000L

    internal fun batches(device: String, profile: String, inventorySeq: Long, seq: Long,
                         items: Collection<InventoryUpload.Item>): List<String> {
        val metadata = JSONObject(InventoryUpload.report(device, profile, inventorySeq, items, false)).getJSONArray("apps")
        val included = (0 until metadata.length()).map { metadata.getJSONObject(it).getString("package") }.toSet()
        val sorted = items.filter { it.pkg in included && it.icon != null }.distinctBy { it.pkg }.sortedBy { it.pkg }
        fun head() = JSONObject().put("type", "inventory_icons").put("v", 1).put("device", device).put("profile", profile)
            .put("inventory_seq", inventorySeq).put("seq", seq).put("icons", JSONArray())
        val result = mutableListOf<String>()
        var batch = head()
        for (item in sorted) {
            val icon = item.icon ?: continue
            if (icon.length > 4 * ((MAX_ICON_BYTES + 2) / 3)) continue
            val bytes = runCatching { Base64.getDecoder().decode(icon) }.getOrNull() ?: continue
            if (bytes.isEmpty() || bytes.size > MAX_ICON_BYTES) continue
            val value = JSONObject().put("package", item.pkg).put("icon", icon)
            var icons = batch.getJSONArray("icons")
            icons.put(value)
            if (icons.length() > 32 || batch.toString().toByteArray().size > InventoryUpload.MAX_BYTES) {
                icons.remove(icons.length() - 1)
                if (icons.length() > 0) result.add(batch.toString())
                batch = head()
                icons = batch.getJSONArray("icons")
                icons.put(value)
            }
            check(batch.toString().toByteArray().size <= InventoryUpload.MAX_BYTES)
        }
        if (batch.getJSONArray("icons").length() > 0) result.add(batch.toString())
        return result
    }

    internal fun send(prefs: SharedPreferences, scope: String, now: Long, items: Collection<InventoryUpload.Item>,
                      device: String, profile: String, post: (String) -> UploadReply) {
        val inventorySeq = prefs.getLong("inventory_seq", 0)
        if (inventorySeq < 1) return
        // Pack once with maximum-width sequences: both the digest and restart
        // progress stay stable as actual transport sequences grow.
        val batches = batches(device, profile, 253402300799L, 253402300799L, items)
        val hash = MessageDigest.getInstance("SHA-256")
        for (raw in batches) { hash.update(raw.toByteArray()); hash.update(10.toByte()) }
        val digest = scope + "|" + hash.digest().joinToString("") { "%02x".format(it) }
        if (prefs.getString("icons_digest", null) != digest) {
            check(prefs.edit().putString("icons_digest", digest).putInt("icons_next", 0).remove("icons_retry_at").remove("icons_completed_at").commit())
        }
        val retryAt = prefs.getLong("icons_retry_at", 0)
        if (retryAt - now in 1..RETRY_MS) return
        var next = prefs.getInt("icons_next", 0)
        if (next >= batches.size && now - prefs.getLong("icons_completed_at", 0) !in 0 until RETRY_MS) {
            next = 0
            check(prefs.edit().putInt("icons_next", 0).commit())
        }
        repeat(32) {
            if (next >= batches.size) {
                if (prefs.getLong("icons_completed_at", 0) == 0L) check(prefs.edit().putLong("icons_completed_at", now).commit())
                return
            }
            val seq = prefs.getLong("icons_seq", 0) + 1
            check(prefs.edit().putLong("icons_seq", seq).commit())
            val raw = JSONObject(batches[next]).put("seq", seq).put("inventory_seq", inventorySeq).toString()
            val accepted = runCatching { post(raw).state in setOf("stored", "unchanged") }.getOrDefault(false)
            if (!accepted) {
                // Includes unsigned unknown-op errors on old routers: no downgrade or
                // unsupported claim. Retry after upgrade, independently of inventory.
                check(prefs.edit().putLong("icons_retry_at", now + RETRY_MS).commit())
                return
            }
            next++
            val edit = prefs.edit().putInt("icons_next", next).remove("icons_retry_at")
            if (next == batches.size) edit.putLong("icons_completed_at", now)
            check(edit.commit())
        }
    }
}
