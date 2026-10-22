package io.github.denogio.familywarden

import android.content.Context
import java.util.UUID

/** Pairing and the last verified payload, kept until the next successful sync. */
class Store(context: Context) {
    private val prefs = context.getSharedPreferences("familywarden", Context.MODE_PRIVATE)

    var host: String?
        get() = prefs.getString("host", null)
        set(v) = prefs.edit().putString("host", v).apply()

    /** Router public key (usign, base64), pinned at pairing. */
    var publicKey: String?
        get() = prefs.getString("pubkey", null)
        set(v) = prefs.edit().putString("pubkey", v).apply()

    /** Raw payload JSON of the last verified sync. */
    var payload: String?
        get() = prefs.getString("payload", null)
        set(v) = prefs.edit().putString("payload", v).apply()

    /** Phone time of the last verified sync (epoch seconds). */
    var lastSync: Long
        get() = prefs.getLong("last_sync", 0)
        set(v) = prefs.edit().putLong("last_sync", v).apply()

    var lastError: String?
        get() = prefs.getString("last_error", null)
        set(v) = prefs.edit().putString("last_error", v).apply()

    /** Shown in LuCI as the device that last synced. */
    val deviceId: String
        get() = prefs.getString("device_id", null) ?: ("fw-" + UUID.randomUUID().toString().replace("-", "")).also {
            prefs.edit().putString("device_id", it).apply()
        }

    /** This phone's signing key: created once, kept across re-pairing. */
    val phoneKey: PhoneKey
        get() = synchronized(PhoneKey) {
            val encoded = prefs.getString("phone_key", null)
            // A damaged key is a recovery problem, not permission to silently re-key.
            if (encoded != null) PhoneKey.decode(encoded)
            else PhoneKey.generate().also {
                check(prefs.edit().putString("phone_key", it.encode()).commit()) { "key persistence failed" }
            }
        }

    /** Android DNS settings are ours to restore on removal (set once we applied them). */
    var dnsOwned: Boolean
        get() = prefs.getBoolean("dns_owned", false)
        set(v) { prefs.edit().putBoolean("dns_owned", v).commit() }

    val paired: Boolean get() = host != null && publicKey != null

    val state: MastState?
        get() = if (!paired) null else payload?.let { runCatching { Payload.parse(it) }.getOrNull() }
            ?: MastState(0, "", false, "", 0, 0, Rules(false, "allow", "none", emptyList()))

    /** Once provisioned, an unsupported/bad reply must not reopen setup removal. */
    var provisioned: Boolean
        get() = prefs.getBoolean("provisioned", false)
        set(v) = prefs.edit().putBoolean("provisioned", v).apply()

    var ulyssesEnabled: Boolean
        get() = prefs.getBoolean("ulysses", false)
        set(v) = prefs.edit().putBoolean("ulysses", v).apply()

    var dnsError: String?
        get() = prefs.getString("dns_error", null)
        set(v) = prefs.edit().putString("dns_error", v).apply()

    val effectiveState: MastState? get() = state?.takeIf { it.appliesTo(deviceId) }

    /** Signed v2/v3 reply says this phone still waits for an administrator's approval. */
    val awaitingApproval: Boolean get() = !provisioned && state?.let { it.wireVersion >= 2 && it.pending } == true

    /** Highest wire version accepted from the paired router: older replies are downgrades. */
    val wireSeen: Int get() = prefs.getInt("wire_seen", 0)

    /**
     * App policy revision this phone actually enforces, acknowledged to the router.
     * Set only by enforcement after applying it, never merely on fetching.
     */
    var appsApplied: String?
        get() = prefs.getString("apps_applied", null)
        set(v) { prefs.edit().putString("apps_applied", v).commit() }

    /** Policy, enrollment latch, and sync metadata are one atomic preference update. */
    internal fun accept(reply: Sync.Result.Ok) {
        prefs.edit().putString("payload", reply.payload)
            .putBoolean("provisioned", provisioned || reply.state.appliesTo(deviceId))
            .putInt("wire_seen", maxOf(wireSeen, reply.state.wireVersion))
            .putLong("last_sync", System.currentTimeMillis() / 1000).remove("last_error").apply()
    }

    /** Pairing is a fresh trust decision, also about which wire version this router speaks. */
    internal fun pairVerified(address: String, key: String, reply: Sync.Result.Ok) {
        prefs.edit().putString("host", address).putString("pubkey", key)
            .putString("payload", reply.payload)
            .putBoolean("provisioned", reply.state.appliesTo(deviceId))
            .putInt("wire_seen", reply.state.wireVersion).remove("apps_applied")
            .putBoolean("ulysses", false).putLong("last_sync", System.currentTimeMillis() / 1000)
            .remove("last_error").apply()
    }

    /** New address for the same pinned key, with the signed reply that proved it: one commit. */
    internal fun repairAddress(address: String, reply: Sync.Result.Ok) {
        prefs.edit().putString("host", address).putString("payload", reply.payload)
            .putInt("wire_seen", maxOf(wireSeen, reply.state.wireVersion))
            .putLong("last_sync", System.currentTimeMillis() / 1000).remove("last_error").apply()
    }

    fun unpair() {
        prefs.edit().remove("host").remove("pubkey").remove("payload").remove("last_sync")
            .remove("last_error").remove("ulysses").remove("dns_error").remove("provisioned")
            .remove("wire_seen").remove("apps_applied").apply()
    }
}
