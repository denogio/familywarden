package me.hojgaard.ulysses.mast

import java.net.HttpURLConnection
import java.net.URL

/**
 * Talks to familywarden's sail-page uhttpd (port 7700, open even during a
 * lockdown). Plain HTTP on the LAN; integrity comes from the usign signature.
 */
object Sync {
    const val PORT = 7700

    class PairingInfo(val host: String, val publicKey: String, val fingerprint: String)

    /** Parse the pairing link from LuCI's QR code: ulyssesmast://pair?host=...&key=... */
    fun parsePairingUri(uri: String): PairingInfo {
        val u = java.net.URI(uri.trim())
        require(u.scheme == "ulyssesmast" && u.host == "pair") { "not a Ulysses Mast pairing code" }
        val params = (u.rawQuery ?: "").split("&").mapNotNull {
            val i = it.indexOf('=')
            if (i < 0) null else it.substring(0, i) to java.net.URLDecoder.decode(it.substring(i + 1), "UTF-8")
        }.toMap()
        val host = params["host"]?.takeIf { it.matches(Regex("[A-Za-z0-9.:-]+")) } ?: throw IllegalArgumentException("no host")
        val key = params["key"] ?: throw IllegalArgumentException("no key")
        return PairingInfo(host, key, Usign.parsePublicKey(key).fingerprint)
    }

    /** Fetch the router's public key for pairing (trust on first use, compare fingerprints). */
    fun fetchPairingInfo(host: String): PairingInfo {
        val lines = get("${base(host)}/cgi-bin/mast?pubkey").lines().filter { it.isNotBlank() }
        require(lines.size >= 2) { "unexpected reply" }
        val key = Usign.parsePublicKey(lines[0])
        require(key.fingerprint == lines[1].trim()) { "fingerprint mismatch" }
        return PairingInfo(host, lines[0].trim(), key.fingerprint)
    }

    sealed class Result {
        data class Ok(val payload: String, val state: MastState) : Result()
        data class Failed(val reason: String) : Result()
    }

    /**
     * Fetch, verify and parse. Rejects anything not signed by the pinned key,
     * and replies older than the last accepted one (replayed "no pact").
     */
    fun fetch(store: Store): Result {
        val host = store.host ?: return Result.Failed("not paired")
        val pub = Usign.parsePublicKey(store.publicKey ?: return Result.Failed("not paired"))
        val body = try {
            get("${base(host)}/cgi-bin/mast?device=${store.deviceId}")
        } catch (e: Exception) {
            return Result.Failed("router unreachable")
        }
        val newline = body.indexOf('\n')
        if (newline < 0) return Result.Failed("malformed reply")
        val signature = body.substring(0, newline)
        val payload = body.substring(newline + 1).trimEnd('\n')
        if (!Usign.verify(pub, signature, payload.toByteArray(Charsets.UTF_8))) return Result.Failed("bad signature")
        val state = try {
            Payload.parse(payload)
        } catch (e: Exception) {
            return Result.Failed("unreadable payload")
        }
        val previous = store.state
        if (previous != null && state.serverTime < previous.serverTime) return Result.Failed("stale reply")
        return Result.Ok(payload, state)
    }

    /** "host" or "host:port" (e.g. an emulator reaching a test router on another port). */
    private fun base(host: String) = if (host.count { it == ':' } == 1) "http://$host" else "http://$host:$PORT"

    private fun get(url: String): String {
        val c = URL(url).openConnection() as HttpURLConnection
        c.connectTimeout = 5000
        c.readTimeout = 5000
        c.useCaches = false
        try {
            if (c.responseCode != 200) throw IllegalStateException("HTTP ${c.responseCode}")
            return c.inputStream.bufferedReader(Charsets.UTF_8).readText()
        } finally {
            c.disconnect()
        }
    }
}
