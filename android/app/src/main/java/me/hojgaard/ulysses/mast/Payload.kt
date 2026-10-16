package me.hojgaard.ulysses.mast

import org.json.JSONObject
import java.time.LocalTime

/** Parses the payload JSON of cgi-bin/mast into a [MastState]. */
object Payload {
    fun parse(json: String): MastState {
        val o = JSONObject(json)
        require(o.optInt("v") == 1) { "unsupported payload version" }
        val pact = o.optJSONObject("pact") ?: JSONObject()
        val r = o.optJSONObject("rules") ?: JSONObject()
        val windows = r.optJSONArray("windows")
        return MastState(
            serverTime = o.getLong("server_time"),
            captain = o.optString("captain"),
            pactActive = pact.optBoolean("active"),
            pactType = pact.optString("type"),
            pactUntil = pact.optLong("until"),
            releaseAt = pact.optLong("release_at"),
            rules = Rules(
                enabled = r.optString("enabled") == "1",
                policy = r.optString("policy", "allow"),
                override = r.optString("override", "none"),
                windows = (0 until (windows?.length() ?: 0)).mapNotNull { i ->
                    val w = windows!!.getJSONObject(i)
                    try {
                        Window(Policy.parseDays(w.getString("days")), LocalTime.parse(w.getString("start")), LocalTime.parse(w.getString("stop")))
                    } catch (e: Exception) {
                        null  // the router ignores incomplete schedules too
                    }
                },
            ),
        )
    }
}
