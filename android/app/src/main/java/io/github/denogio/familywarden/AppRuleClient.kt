package io.github.denogio.familywarden

import android.content.Context
import org.json.JSONObject

/** Editable local intent. It is never policy or an acknowledgement. */
data class RuleDraft(val mode: String, val minutes: String = "30") {
    fun quota(): Int = if (mode == "limit") requireNotNull(minutes.toIntOrNull()).also { require(it in 0..1440) } else 0
}

/** Exact signed bytes survive process death; one unresolved operation across all children. */
data class RuleOperation(val raw: String, val signature: String, val router: String, val phone: String,
    val state: String = "unknown", val error: String? = null) {
    val message: JSONObject get() = JSONObject(raw)
    val profile: String get() = message.getString("profile")
    val pkg: String get() = message.getString("package")
    val unresolved: Boolean get() = state in setOf("unknown", "retryable")
    fun body(): ByteArray = (signature + "\n" + raw).toByteArray(Charsets.UTF_8)
    fun encode(): String = JSONObject().put("raw", raw).put("signature", signature).put("router", router)
        .put("phone", phone).put("state", state).put("error", error).toString()
    companion object {
        fun decode(raw: String): RuleOperation {
            require(raw.toByteArray().size < 32768)
            val o = JSONObject(raw)
            return RuleOperation(o.getString("raw"), o.getString("signature"), o.getString("router"),
                o.getString("phone"), o.getString("state"), o.optString("error").takeIf { it.isNotEmpty() && it != "null" })
        }
    }
}

object AppRuleClient {
    internal fun query(device: String, profile: String, pkg: String, rev: String, draft: RuleDraft, now: Long, nonce: String): String {
        require(validPackage(pkg) && rev.length in 1..128 && draft.mode in setOf("allowed", "limit", "always", "blocked"))
        require(device.matches(Regex("[A-Za-z0-9-]{1,80}")) && profile.matches(Regex("[A-Za-z0-9_-]{1,64}")))
        require(nonce.matches(Regex("[0-9a-f]{64}")) && now in 1..253402300199L)
        return JSONObject().put("type", "rule").put("v", 1).put("device", device).put("profile", profile).put("package", pkg)
            .put("mode", draft.mode).put("minutes", draft.quota()).put("apps_rev", rev).put("created", now)
            .put("expires", now + 600).put("nonce", nonce).toString()
    }

    fun save(context: Context, data: InstalledApps, app: InstalledApp, draft: RuleDraft): RuleOperation = synchronized(Mast) {
        val store = Store(context)
        check(store.ruleOperation?.unresolved != true) { "unresolved rule operation" }
        val fresh = fresh(store)
        check(fresh.state.binding?.approves?.any { it.id == data.profile.id } == true) { "scope changed" }
        val raw = query(store.deviceId, data.profile.id, app.pkg, data.appsRev, draft, fresh.state.serverTime, Sync.newNonce())
        val signature = store.phoneKey.sign("familywarden-rule-v1\n".toByteArray() + raw.toByteArray(Charsets.UTF_8))
        val op = RuleOperation(raw, signature, requireNotNull(store.publicKey), store.phoneKey.publicKey)
        store.ruleOperation = op // durable before the first attempt; never inferred from a UI callback
        deliver(store, op, false, fresh.state.serverTime)
    }

    /** A retry is allowed only after a fresh, signed, exact-operation status check. */
    fun check(context: Context, retry: Boolean = false): RuleOperation = synchronized(Mast) {
        val store = Store(context)
        val op = requireNotNull(store.ruleOperation)
        check(op.router == store.publicKey && op.phone == store.phoneKey.publicKey && op.message.getString("device") == store.deviceId) {
            "operation belongs to another identity"
        }
        try {
            val fresh = fresh(store)
            reconcile(op, retry) { original, status -> deliver(store, original, status, fresh.state.serverTime) }
        } catch (_: Exception) {
            op.copy(state = "unknown").also { store.ruleOperation = it }
        }
    }

    internal fun reconcile(op: RuleOperation, retry: Boolean, exchange: (RuleOperation, Boolean) -> RuleOperation): RuleOperation {
        val checked = exchange(op, true)
        return if (retry && checked.state == "retryable") exchange(op, false) else checked
    }

    private fun fresh(store: Store): Sync.Result.Ok {
        val reply = Sync.fetch(store)
        check(reply is Sync.Result.Ok && reply.state.appliesTo(store.deviceId)) { "verified router required" }
        store.accept(reply)
        return reply
    }

    private fun deliver(store: Store, op: RuleOperation, status: Boolean, minimumTime: Long): RuleOperation {
        val result = try {
            val endpoint = if (status) "rule_status" else "rule"
            parse(AppRequestClient.exchange(requireNotNull(store.host), endpoint, op.body()), op, status, minimumTime)
        } catch (_: Exception) { op.copy(state = "unknown") }
        store.ruleOperation = result
        return result
    }

    internal fun parse(body: ByteArray, op: RuleOperation, status: Boolean, minimumTime: Long): RuleOperation {
        val endpoint = if (status) "rule_status" else "rule"
        val o = RequestProtocol.signedReply(body, op.router, op.message.getString("device"), endpoint, op.raw, minimumTime)
        if (o.has("error")) {
            val code = o.getString("error")
            require(code in setOf("malformed", "not_parent", "app_control_off", "configuration_required", "confirmation_pending",
                "invalid_rule", "unavailable", "outcome_unknown", "replay", "operation_capacity"))
            // A status refusal cannot prove that an earlier attempt did not commit.
            return op.copy(state = if (status || code == "outcome_unknown") "unknown" else "refused", error = code)
        }
        require(o.getString("profile_id") == op.profile && o.getString("package") == op.pkg && o.getString("apps_rev").length in 1..128)
        val state = o.getString("state")
        require(state in (if (status) setOf("matched", "retryable", "conflict", "expired") else setOf("saved", "matched", "conflict", "expired")))
        return op.copy(state = state, error = null)
    }
}

/** Preflight failures cannot claim that a new signed operation was retained. */
internal fun ruleSaveNotSent(before: RuleOperation?, after: RuleOperation?): Boolean =
    after == null || after.raw == before?.raw
