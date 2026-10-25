package io.github.denogio.familywarden

import android.content.Context
import android.os.SystemClock
import android.util.Log
import org.json.JSONObject
import java.time.Instant
import java.time.LocalDate

/** A parent's view of one child's day, as signed by the router. Display only. */
data class FamilyUsage(val profile: ProfileRef, val day: String, val totalSeconds: Int,
    val devices: List<DeviceUsage>, val apps: List<AppSeconds>)
data class DeviceUsage(val name: String, val totalSeconds: Int, val incomplete: Boolean)
data class AppSeconds(val pkg: String, val label: String, val seconds: Int)

/** Other phones' usage for [day] (router-local), in milliseconds keyed like UsageCounter days. */
data class SharedUsage(val day: String, val millis: Map<String, Long>) {
    companion object {
        private const val MAX = 16L * 25 * 3600
        fun parse(o: JSONObject): SharedUsage {
            val day = o.getString("day").also { require(LocalDate.parse(it).toString() == it) }
            val apps = o.getJSONObject("apps")
            require(apps.length() <= 64) { "too many shared apps" }
            val millis = apps.keys().asSequence().associateWith { pkg ->
                require(validPackage(pkg)) { "invalid package" }
                RequestProtocol.integer(apps, pkg).also { require(it in 0..MAX) } * 1000
            } + (UsageCounter.TOTAL to RequestProtocol.integer(o, "total").also { require(it in 0..MAX) } * 1000)
            return SharedUsage(day, millis)
        }
    }
}

/**
 * Cumulative usage reports (protocol §5). Information for parents, never authority:
 * nothing on any phone is unlocked by them. Cumulative, so a lost report is simply
 * superseded by the next one; no outbox.
 */
object UsageUpload {
    private const val MAX_DAY_SECONDS = 25 * 3600
    private const val MIN_INTERVAL_MS = 60_000L

    /** Exact JSON for one day. [millis] is the checkpoint's day map (apps plus [UsageCounter.TOTAL]). */
    fun report(device: String, profile: String, day: String, seq: Long, millis: Map<String, Long>,
        labels: Map<String, String>, grantMillis: Map<String, Long>, incomplete: Boolean): String {
        require(seq >= 1 && LocalDate.parse(day).toString() == day)
        val apps = millis.filterKeys { it != UsageCounter.TOTAL && validPackage(it) }
            .mapValues { (_, ms) -> (ms / 1000).coerceIn(0L, MAX_DAY_SECONDS.toLong()).toInt() }
            .filterValues { it > 0 }.entries.sortedWith(compareBy({ -it.value }, { it.key })).take(256)
        val o = JSONObject().put("type", "usage").put("v", 1).put("device", device).put("profile", profile)
            .put("day", day).put("seq", seq)
            .put("total", ((millis[UsageCounter.TOTAL] ?: 0) / 1000).coerceIn(0L, MAX_DAY_SECONDS.toLong()).toInt())
            .put("apps", JSONObject().apply { apps.forEach { put(it.key, it.value) } })
            .put("labels", JSONObject().apply {
                apps.forEach { (pkg, _) -> label(labels[pkg])?.let { put(pkg, it) } }
            })
            .put("incomplete", incomplete)
        if (grantMillis.isNotEmpty()) o.put("grants_used", JSONObject().apply {
            grantMillis.entries.sortedBy { it.key }.take(32).forEach { (id, ms) ->
                require(id.matches(Regex("[0-9a-f]{32}")))
                put(id, (ms / 1000).coerceIn(0L, 240L * 60).toInt())
            }
        })
        return o.toString()
    }

    /** Untrusted display text from PackageManager: bounded, no control characters. */
    internal fun label(raw: String?): String? {
        val s = raw?.trim()?.filterNot { Character.isISOControl(it) } ?: return null
        if (s.isEmpty()) return null
        return s.substring(0, s.offsetByCodePoints(0, minOf(64, s.codePointCount(0, s.length))))
    }

    /** Strict and scoped: only profiles this phone may approve for, bounded sizes. */
    fun parse(arr: org.json.JSONArray, scope: List<ProfileRef>): List<FamilyUsage> {
        require(arr.length() <= 16) { "too many usage entries" }
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            val ref = scope.firstOrNull { it.id == o.getString("profile_id") }
                ?: throw IllegalArgumentException("usage outside parent scope")
            val day = o.getString("day").also { require(LocalDate.parse(it).toString() == it) }
            val devices = o.getJSONArray("devices").let { d ->
                require(d.length() <= 16)
                (0 until d.length()).map { j ->
                    val x = d.getJSONObject(j)
                    DeviceUsage(x.getString("name").also { require(it.codePointCount(0, it.length) in 1..64) },
                        seconds(x, "total"), x.getBoolean("incomplete"))
                }
            }
            val apps = o.getJSONArray("apps").let { a ->
                require(a.length() <= 20)
                (0 until a.length()).map { j ->
                    val x = a.getJSONObject(j)
                    AppSeconds(x.getString("package").also { require(validPackage(it)) },
                        label(x.optString("label", "")) ?: "", seconds(x, "seconds"))
                }
            }
            FamilyUsage(ref, day, seconds(o, "total"), devices, apps)
        }.also { require(it.map { u -> u.profile.id }.toSet().size == it.size) { "duplicate usage profile" } }
    }

    private fun seconds(o: JSONObject, key: String): Int =
        RequestProtocol.integer(o, key).also { require(it in 0..16L * MAX_DAY_SECONDS) }.toInt()

    /** Best effort after a successful sync; the next sync repeats it. */
    fun send(context: Context, store: Store, state: MastState) {
        val apps = state.apps ?: return
        val binding = state.binding ?: return
        val host = store.host ?: return
        val routerKey = store.publicKey ?: return
        val prefs = context.getSharedPreferences("usage-upload", Context.MODE_PRIVATE)
        val elapsed = SystemClock.elapsedRealtime()
        val last = prefs.getLong("sent_elapsed", -1)
        if (last in 0..elapsed && elapsed - last < MIN_INTERVAL_MS) return
        val checkpoint = UsageJournal(context).load() ?: return
        val zone = AppControl.zone(state)
        if (checkpoint.zone != zone.id || checkpoint.sequence < 1) return
        val server = store.policyClock(state)?.now(elapsed, android.provider.Settings.Global.getInt(
            context.contentResolver, android.provider.Settings.Global.BOOT_COUNT, -1)) ?: state.serverTime * 1000
        val today = Instant.ofEpochMilli(server).atZone(zone).toLocalDate()
        val pin = Usign.parsePublicKey(routerKey).fingerprint
        val scope = "$pin|${store.phoneKey.fingerprint}|${binding.deviceId}|${binding.profileId}"
        val pm = context.packageManager
        for (day in listOf(today.minusDays(1), today).map(LocalDate::toString)) {
            val millis = checkpoint.counter.days[day] ?: continue
            val labels = millis.keys.filter { it != UsageCounter.TOTAL }.associateWith { pkg ->
                runCatching { pm.getApplicationLabel(pm.getApplicationInfo(pkg, 0)).toString() }.getOrDefault("")
            }
            // Router ledgers prune 24 h after a grant ends; report well inside that window.
            val grants = if (day == today.toString())
                checkpoint.grants.reported(scope, binding.deviceId, server - 12 * 3_600_000L) else emptyMap()
            val raw = report(binding.deviceId, binding.profileId, day, checkpoint.sequence, millis, labels, grants,
                day <= checkpoint.uncertainThroughDay)
            val result = runCatching {
                AppRequestClient.post(host, RequestUpload("usage", raw, routerKey, binding.deviceId), store.phoneKey, state.serverTime)
            }
            Log.i("UlyssesMast", "usage $day: ${result.getOrNull()?.let { it.state ?: it.error } ?: result.exceptionOrNull()?.message}")
            // Pinned-key fingerprint, never the binding's app list, is logged.
            if (result.getOrNull()?.error == "app_control_off") break
        }
        prefs.edit().putLong("sent_elapsed", elapsed).apply()
    }
}
