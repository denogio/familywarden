package io.github.denogio.familywarden

import org.json.JSONObject
import java.time.LocalTime
import java.time.ZoneId

/** v1 is readable for diagnostics only; v2 is explicitly scoped and request-bound. */
object Payload {
    fun parse(json: String): MastState {
        val o = JSONObject(json)
        val version = o.optInt("v")
        require(version == 1 || version == 2) { "unsupported payload version" }
        val binding = if (version == 2) o.getJSONObject("binding").let { b ->
            DeviceBinding(
                b.getString("device_id"), b.getString("profile_id"), b.getString("profile_name"),
                b.getBoolean("captain"), b.getBoolean("administrator"), b.getBoolean("can_self_remove"),
            ).also {
                require(it.deviceId.matches(Regex("[A-Za-z0-9-]{1,80}"))) { "invalid device ID" }
                require(it.profileId.matches(Regex("[A-Za-z0-9_-]{1,64}"))) { "invalid profile ID" }
                require(it.profileName.length in 1..128) { "invalid profile name" }
            }
        } else null
        val dnsObject = if (version == 2 && o.has("dns") && !o.isNull("dns")) o.getJSONObject("dns") else null
        val dns = dnsObject?.let { d ->
            val names = d.getJSONArray("protections")
            require(names.length() <= Protection.entries.size) { "too many DNS protections" }
            val hostname = d.getString("hostname")
            require(validHostname(hostname)) { "invalid DNS hostname" }
            DnsPolicy(hostname.lowercase(), (0 until names.length()).map {
                Protection.valueOf(names.getString(it).uppercase())
            }.toSet())
        }
        val nonce = if (version == 2) o.getString("nonce").also {
            require(it.matches(Regex("[0-9a-f]{64}"))) { "invalid response nonce" }
        } else null
        val zone = if (version == 2) o.getString("timezone").also { ZoneId.of(it) } else null
        val pact = if (version == 2) o.getJSONObject("pact") else o.optJSONObject("pact") ?: JSONObject()
        val r = if (version == 2) o.getJSONObject("rules") else o.optJSONObject("rules") ?: JSONObject()
        val windows = if (version == 2) r.getJSONArray("windows") else r.optJSONArray("windows")
        require((windows?.length() ?: 0) <= 128) { "too many schedule windows" }
        val state = MastState(
            serverTime = o.getLong("server_time"),
            captain = o.optString("captain"),
            pactActive = if (version == 2) pact.getBoolean("active") else pact.optBoolean("active"),
            pactType = if (version == 2) pact.getString("type") else pact.optString("type"),
            pactUntil = if (version == 2) pact.getLong("until") else pact.optLong("until"),
            releaseAt = if (version == 2) pact.getLong("release_at") else pact.optLong("release_at"),
            rules = Rules(
                enabled = r.optString("enabled") == "1",
                policy = r.optString("policy", "allow"),
                override = r.optString("override", "none"),
                windows = (0 until (windows?.length() ?: 0)).mapNotNull { i ->
                    val w = windows!!.getJSONObject(i)
                    try {
                        val days = w.getString("days")
                        val start = w.getString("start")
                        val stop = w.getString("stop")
                        if (version == 2) {
                            require(days.isNotBlank() && days.trim().split(Regex("\\s+")).all {
                                Policy.parseDays(it).size == 1
                            }) { "invalid schedule days" }
                            require(start.matches(Regex("[0-9]{2}:[0-9]{2}")) &&
                                stop.matches(Regex("[0-9]{2}:[0-9]{2}"))) { "invalid schedule time" }
                        }
                        Window(Policy.parseDays(days), LocalTime.parse(start), LocalTime.parse(stop))
                    } catch (e: Exception) {
                        if (version == 2) throw e else null
                    }
                },
            ),
            binding = binding, dns = dns, requestNonce = nonce, wireVersion = version, timeZone = zone,
        )
        // Bounded epoch seconds (through year 9999), not milliseconds/overflowing alarm times.
        require(listOf(state.serverTime, state.pactUntil, state.releaseAt).all {
            it in 0..253402300799L
        }) { "invalid epoch" }
        if (version == 2) {
            require(r.getString("enabled") in setOf("0", "1")) { "invalid enabled flag" }
            require(r.getString("policy") in setOf("allow", "block", "scheduled")) { "invalid policy" }
            require(r.getString("override") in setOf("none", "allow", "block")) { "invalid override" }
            require(state.pactType in setOf("", "ongoing", "until")) { "invalid pact type" }
            if (binding?.captain == true) require(state.captain == binding.profileId) { "captain mismatch" }
            if (state.pactActive) {
                require(binding?.captain == true && state.captain == binding.profileId) { "pact is not applicable to this device" }
                require(state.pactType in setOf("ongoing", "until")) { "invalid pact type" }
                require(state.pactType != "until" || state.pactUntil > 0) { "invalid pact end" }
            }
        }
        return state
    }
}
