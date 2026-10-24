package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject

/** Feature names describe implemented semantics, never roles or permission grants. */
object PolicyCapabilities {
    // P7b: whole-phone downtime, daily union screen time, always-allowed apps and dated bonus.
    val supported: Set<String> = setOf("apps-v1", "screen-time-v1")
    val advertised: String = supported.sorted().joinToString(",")

    class Unsupported : IllegalArgumentException("unsupported capabilities")

    /** An ack from an older implementation is not proof that new enforcement semantics ran. */
    fun acknowledgedRevision(revision: String?, capabilities: String?): String? = revision?.takeIf {
        capabilities == advertised && it.matches(Regex("[0-9a-f]{16}"))
    }

    /** Called only after signature verification in Sync; also strict for cached/pending payloads. */
    fun check(payload: JSONObject) {
        if (payload.has("requires")) {
            val raw = payload.get("requires")
            require(raw is JSONArray && raw.length() <= 16) { "invalid required capabilities" }
            val names = (0 until raw.length()).map { index ->
                val name = raw.get(index)
                require(name is String && name.matches(Regex("[a-z][a-z0-9-]{0,31}"))) { "invalid required capability" }
                name
            }
            require(names.toSet().size == names.size) { "duplicate required capability" }
            if (!supported.containsAll(names)) throw Unsupported()
        }
        if (payload.has("error")) {
            if (payload.get("error") == "unsupported_capabilities") throw Unsupported()
            throw IllegalArgumentException("policy error")
        }
    }
}
