package me.hojgaard.ulysses.mast

import android.content.Context
import java.util.UUID

/** Pairing and the last verified payload, kept until the next successful sync. */
class Store(context: Context) {
    private val prefs = context.getSharedPreferences("mast", Context.MODE_PRIVATE)

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
        get() = prefs.getString("device_id", null) ?: ("mast-" + UUID.randomUUID().toString().take(8)).also {
            prefs.edit().putString("device_id", it).apply()
        }

    val paired: Boolean get() = host != null && publicKey != null

    val state: MastState?
        get() = if (!paired) null else payload?.let { runCatching { Payload.parse(it) }.getOrNull() }
            ?: MastState(0, "", false, "", 0, 0, Rules(false, "allow", "none", emptyList()))

    fun unpair() {
        prefs.edit().remove("host").remove("pubkey").remove("payload").remove("last_sync").remove("last_error").apply()
    }
}
