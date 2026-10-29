package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest

/** Router-policy authority, valid only for the requesting phone. Never a standing limit. */
data class AppGrant(val id: String, val deviceId: String, val target: String, val minutes: Int,
    val notBefore: Long, val notAfter: Long) {
    init {
        require(id.matches(Regex("[0-9a-f]{32}")) && deviceId.matches(Regex("[A-Za-z0-9-]{1,80}")))
        require(target == "total" || validPackage(target))
        require(minutes in 1..240 && notBefore in 1..253402293599L && notAfter - notBefore == 7200L)
    }
}

data class ChildRequest(val id: String, val target: String, val minutes: Int, val state: String,
    val expires: Long, val granted: Int? = null, val notAfter: Long? = null,
    val answeredBy: String? = null)

/** A paused new app waiting for a parent in this phone's scope (display; router rechecks). */
data class NewAppRequest(val profile: ProfileRef, val pkg: String, val label: String, val seen: Long)

data class ParentRequest(val id: String, val hash: String, val profile: ProfileRef, val target: String,
    val minutes: Int, val received: Long, val expires: Long, val label: String = "", val note: String = "")

/** Only status, never policy/removal authority. The policy arrives through normal verified sync. */
data class UploadReply(val state: String?, val error: String?, val granted: Int? = null,
    val notAfter: Long? = null, val homeInternet: Boolean? = null)

/** Kept byte-for-byte across retries, including the original creation timestamp and request id. */
data class RequestUpload(val op: String, val raw: String, val routerKey: String, val device: String) {
    init {
        require(op in setOf("request", "decision", "usage", "app", "inventory", "inventory_icons"))
        require(raw.toByteArray(Charsets.UTF_8).size < (if (op == "inventory" || op == "inventory_icons") 64 * 1024 else 16 * 1024) - 101)
        val o = JSONObject(raw)
        require(o.getString("type") == (if (op == "app") "app_decision" else op) && o.getString("device") == device)
        if (op == "request" || op == "decision") require(o.getString("request_id").matches(RequestProtocol.requestId))
        Usign.parsePublicKey(routerKey)
    }
    fun encode(): String = JSONObject().put("op", op).put("raw", raw)
        .put("router", routerKey).put("device", device).toString()
    fun signedBody(key: PhoneKey): ByteArray {
        val bytes = raw.toByteArray(Charsets.UTF_8)
        return (key.sign("familywarden-$op-v1\n".toByteArray() + bytes) + "\n").toByteArray() + bytes
    }
    companion object {
        fun decode(raw: String): RequestUpload {
            require(raw.toByteArray().size <= 32 * 1024)
            val o = JSONObject(raw)
            return RequestUpload(o.getString("op"), o.getString("raw"), o.getString("router"), o.getString("device"))
        }
    }
}

/** Bounds match the Go LAN contract, not the old speculative standalone-grant draft. */
object RequestProtocol {
    internal val requestId = Regex("[A-Za-z0-9-]{8,64}")
    private val hash = Regex("[0-9a-f]{64}")
    private val device = Regex("[A-Za-z0-9-]{1,80}")
    private const val MAX_EPOCH = 253402300799L
    private val states = setOf("pending", "approved", "denied", "expired")
    private val errors = setOf("malformed", "wrong_profile", "unknown_package", "invalid_minutes", "request_expired",
        "decision_expired", "app_control_off", "pact_forbids", "not_parent", "not_enrolled", "unknown_request",
        "replay", "request_open", "unavailable", "already_answered", "usage_regressed", "day_out_of_range", "unknown_grant", "unknown_app")

    private fun string(o: JSONObject, key: String): String = (o.get(key) as? String)
        ?: throw IllegalArgumentException("invalid $key")
    internal fun integer(o: JSONObject, key: String): Long {
        val n = o.get(key)
        require(n is Int || n is Long) { "invalid $key" }
        return (n as Number).toLong()
    }
    private fun epoch(o: JSONObject, key: String) = integer(o, key).also { require(it in 1..MAX_EPOCH) }
    private fun minutes(o: JSONObject, key: String) = integer(o, key).also { require(it in 1..240) }.toInt()
    private fun id(o: JSONObject) = string(o, "request_id").also { require(it.matches(requestId)) }
    private fun target(o: JSONObject) = string(o, "target").also { require(it == "total" || validPackage(it)) }
    private fun text(o: JSONObject, key: String, max: Int): String {
        if (!o.has(key)) return ""
        return string(o, key).also { s ->
            require(s.codePointCount(0, s.length) <= max && s.none { Character.isISOControl(it) })
        }
    }

    fun grants(a: JSONObject, deviceId: String): List<AppGrant> {
        if (!a.has("grants")) return emptyList()
        val arr = a.getJSONArray("grants")
        require(arr.length() <= 32) { "too many grants" }
        return (0 until arr.length()).map { i ->
            val g = arr.getJSONObject(i)
            AppGrant(string(g, "grant_id").also { require(it.matches(Regex("[0-9a-f]{32}"))) },
                string(g, "device_id").also { require(it.matches(device) && it == deviceId) },
                target(g), minutes(g, "minutes"), epoch(g, "not_before"), epoch(g, "not_after")).also {
                require(it.notAfter - it.notBefore == 7200L) { "invalid grant lifetime" }
            }
        }.also { require(it.map(AppGrant::id).toSet().size == it.size) { "duplicate grant" } }
    }

    fun child(o: JSONObject): ChildRequest {
        val state = string(o, "state").also { require(it in states) }
        val requested = minutes(o, "minutes")
        return ChildRequest(id(o), target(o), requested, state, epoch(o, "expires"),
            if (state == "approved") minutes(o, "granted").also { require(it <= requested) } else null,
            if (state == "approved") epoch(o, "not_after") else null,
            if (o.has("answered_by")) text(o, "answered_by", 128) else null)
    }

    fun parents(arr: JSONArray, scope: List<ProfileRef>): List<ParentRequest> {
        require(arr.length() <= 16) { "too many pending requests" }
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            val profile = string(o, "profile_id")
            require(scope.any { it.id == profile }) { "request outside parent scope" }
            ParentRequest(id(o), string(o, "request_sha256").also { require(it.matches(hash)) },
                ProfileRef(profile, text(o, "profile_name", 128).also { require(it.isNotBlank()) }),
                target(o), minutes(o, "minutes"), epoch(o, "received"), epoch(o, "expires"),
                text(o, "label", 64), text(o, "note", 140)).also {
                require(it.expires > it.received && it.expires <= it.received + 3600)
            }
        }.also {
            require(it.map(ParentRequest::id).toSet().size == it.size && it.map { r -> r.profile.id }.toSet().size == it.size)
        }
    }

    fun request(id: String, device: String, profile: String, target: String, minutes: Int,
        created: Long, label: String = "", note: String = ""): String {
        require(id.matches(requestId) && device.matches(this.device) && profile.matches(Regex("[A-Za-z0-9_-]{1,64}")))
        require((target == "total" || validPackage(target)) && minutes in 1..240 && created in 1..MAX_EPOCH)
        val o = JSONObject().put("type", "request").put("v", 1).put("request_id", id).put("device", device)
            .put("profile", profile).put("target", target).put("minutes", minutes).put("created", created)
        if (label.isNotEmpty()) o.put("label", label).also { text(it, "label", 64) }
        if (note.isNotEmpty()) o.put("note", note).also { text(it, "note", 140) }
        return o.toString()
    }

    /** One line of printable text, at most 140 characters (the router's own rule). */
    fun requestNote(input: String): String {
        val line = input.replace(Regex("\\s+"), " ").trim()
        val clipped = line.substring(0, line.offsetByCodePoints(0, minOf(140, line.codePointCount(0, line.length))))
        return clipped.takeIf { it.matches(Regex("[\\p{L}\\p{M}\\p{N}\\p{P}\\p{S} ]{1,140}")) } ?: ""
    }

    fun newApps(arr: JSONArray, scope: List<ProfileRef>): List<NewAppRequest> {
        require(arr.length() <= 64) { "too many new apps" }
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            val profile = string(o, "profile_id")
            require(scope.any { it.id == profile }) { "new app outside parent scope" }
            NewAppRequest(ProfileRef(profile, text(o, "profile_name", 128).also { require(it.isNotBlank()) }),
                string(o, "package").also { require(validPackage(it)) }, text(o, "label", 64), epoch(o, "seen"))
        }.also { require(it.map { a -> a.profile.id to a.pkg }.toSet().size == it.size) { "duplicate new app" } }
    }

    fun appDecision(app: NewAppRequest, device: String, approve: Boolean, created: Long): String {
        require(device.matches(this.device) && created in 1..MAX_EPOCH)
        return JSONObject().put("type", "app_decision").put("v", 1).put("device", device).put("profile", app.profile.id)
            .put("package", app.pkg).put("action", if (approve) "approve" else "deny").put("created", created).toString()
    }

    fun decision(request: ParentRequest, device: String, approve: Boolean, minutes: Int, created: Long): String {
        require(device.matches(this.device) && created in 1..MAX_EPOCH)
        require(if (approve) minutes in 1..request.minutes else minutes == 0)
        return JSONObject().put("type", "decision").put("v", 1).put("request_id", request.id)
            .put("request_sha256", request.hash).put("device", device).put("approve", approve)
            .put("minutes", minutes).put("created", created).toString()
    }

    fun digest(raw: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(raw)
        .joinToString("") { "%02x".format(it) }

    /** Verify a message-bound informational envelope; this never returns policy authority. */
    internal fun signedReply(body: ByteArray, routerKey: String, device: String, op: String,
        request: String, minimumServerTime: Long): JSONObject {
        require(body.size <= 65536 + 101)
        val newline = body.indexOf(10.toByte())
        require(newline == 100) { "malformed signed reply" }
        val signature = body.copyOfRange(0, newline).toString(Charsets.US_ASCII)
        val raw = body.copyOfRange(newline + 1, body.size)
        require(Usign.verify(Usign.parsePublicKey(routerKey), signature, raw)) { "bad reply signature" }
        val json = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
        val o = JSONObject(json)
        require(integer(o, "v") == 1L && string(o, "type") == "${op}_reply")
        require(string(o, "device") == device && epoch(o, "server_time") >= minimumServerTime)
        require(string(o, "message_sha256") == digest(request.toByteArray(Charsets.UTF_8))) { "reply mismatch" }
        return o
    }

    /** Authenticate exact bytes first; no upload reply can be accepted as a policy. */
    fun verifyReply(body: ByteArray, upload: RequestUpload, minimumServerTime: Long): UploadReply {
        val o = signedReply(body, upload.routerKey, upload.device, upload.op, upload.raw, minimumServerTime)
        if (o.has("error")) return UploadReply(null, string(o, "error").also { require(it in errors) })
        if (upload.op == "usage" || upload.op == "inventory" || upload.op == "inventory_icons") return UploadReply(string(o, "state").also { require(it in setOf("stored", "unchanged")) }, null)
        if (upload.op == "app") {
            val sent = JSONObject(upload.raw)
            require(string(o, "profile") == sent.getString("profile") && string(o, "package") == sent.getString("package"))
            val want = mapOf("approve" to "approved", "deny" to "denied", "revoke" to "revoked")[sent.getString("action")]
            return UploadReply(string(o, "state").also { require(it == want) }, null)
        }
        require(id(o) == JSONObject(upload.raw).getString("request_id"))
        val state = string(o, "state").also { require(it in states) }
        if (upload.op == "decision") {
            require(state in setOf("approved", "denied"))
            require(JSONObject(upload.raw).getBoolean("approve") == (state == "approved"))
        }
        val granted = if (state == "approved" && upload.op == "decision") minutes(o, "granted").also {
            require(it <= integer(JSONObject(upload.raw), "minutes"))
        } else null
        val end = if (granted != null) epoch(o, "not_after").also {
            require(it <= epoch(o, "server_time") + 7200)
        } else null
        val internet = if (o.has("home_internet")) (o.get("home_internet") as? Boolean
            ?: throw IllegalArgumentException("invalid internet status")) else null
        return UploadReply(state, null, granted, end, internet)
    }
}
