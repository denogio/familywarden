package io.github.denogio.familywarden

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.time.LocalDate

/** A missing day's usage is null, not a fabricated zero. Never policy authority. */
data class FamilyHistoryDay(val profile: ProfileRef, val day: String, val usage: FamilyUsage?, val serverTime: Long)

class HistoryRefused(val code: String) : IllegalStateException("history refused: $code")

/** Read-only LAN API preparation: no outbox, cache write, acknowledgement or mutation. */
object FamilyHistoryClient {
    fun history(context: Context, profileId: String, day: LocalDate): FamilyHistoryDay = synchronized(Mast) {
        val store = Store(context)
        val fresh = Sync.fetch(store)
        check(fresh is Sync.Result.Ok && fresh.state.appliesTo(store.deviceId)) { "verified router required" }
        val binding = requireNotNull(fresh.state.binding)
        val profile = if (binding.profileId == profileId) ProfileRef(profileId, binding.profileName)
            else requireNotNull(binding.approves.singleOrNull { it.id == profileId }) { "profile outside current scope" }
        // Sync.fetch verifies generation/downgrade against the saved policy. Neither
        // this policy preview nor the historical reply changes enforcement state.
        read(requireNotNull(store.host), requireNotNull(store.publicKey), store.phoneKey,
            store.deviceId, profile, day, fresh.state.serverTime)
    }

    internal fun query(device: String, profile: String, day: LocalDate, created: Long, nonce: String): String {
        require(device.matches(Regex("[A-Za-z0-9-]{1,80}")) && profile.matches(Regex("[A-Za-z0-9_-]{1,64}")))
        require(created in 1..253402300799L && nonce.matches(Regex("[0-9a-f]{64}")) && day.year in 1..9999)
        return JSONObject().put("type", "history").put("v", 1).put("device", device).put("profile", profile)
            .put("day", day.toString()).put("created", created).put("nonce", nonce).toString()
    }

    internal fun read(host: String, routerKey: String, phoneKey: PhoneKey, device: String,
        profile: ProfileRef, day: LocalDate, serverTime: Long, nonce: String = Sync.newNonce()): FamilyHistoryDay {
        val raw = query(device, profile.id, day, serverTime, nonce)
        val signature = phoneKey.sign("familywarden-history-v1\n".toByteArray() + raw.toByteArray(Charsets.UTF_8))
        val body = (signature + "\n" + raw).toByteArray(Charsets.UTF_8)
        return parse(AppRequestClient.exchange(host, "history", body), routerKey, device, profile, day, raw, serverTime)
    }

    internal fun parse(body: ByteArray, routerKey: String, device: String, profile: ProfileRef,
        day: LocalDate, raw: String, minimumServerTime: Long): FamilyHistoryDay {
        val o = RequestProtocol.signedReply(body, routerKey, device, "history", raw, minimumServerTime)
        if (o.has("error")) {
            val code = o.get("error") as? String
            require(code in setOf("malformed", "not_parent", "request_expired", "app_control_off", "day_out_of_range", "unavailable"))
            throw HistoryRefused(requireNotNull(code))
        }
        require(o.getString("profile_id") == profile.id && o.getString("day") == day.toString())
        val reported = o.get("reported") as? Boolean ?: throw IllegalArgumentException("invalid history coverage")
        val usage = UsageUpload.parse(JSONArray().put(o), listOf(profile)).single()
        require(if (reported) usage.devices.isNotEmpty() else
            usage.totalSeconds == 0 && usage.devices.isEmpty() && usage.apps.isEmpty())
        return FamilyHistoryDay(profile, day.toString(), usage.takeIf { reported }, RequestProtocol.integer(o, "server_time"))
    }
}
