package io.github.denogio.familywarden

import java.net.HttpURLConnection
import java.net.URL

/**
 * Talks to familywarden's sail-page uhttpd (port 7700, open even during a
 * lockdown). Plain HTTP on the LAN; integrity comes from the usign signature.
 */
object Sync {
    const val PORT = 7700

    class PairingInfo(val host: String, val publicKey: String, val fingerprint: String, val displayHost: String = host)

    /** Parse the pairing link from LuCI's QR code: ulyssesmast://pair?host=...&key=... */
    fun parsePairingUri(uri: String): PairingInfo {
        val u = java.net.URI(uri.trim())
        require(u.scheme in setOf("familywarden", "ulyssesmast") && u.host == "pair") { "not a FamilyWarden pairing code" }
        val params = (u.rawQuery ?: "").split("&").mapNotNull {
            val i = it.indexOf('=')
            if (i < 0) null else it.substring(0, i) to java.net.URLDecoder.decode(it.substring(i + 1), "UTF-8")
        }.toMap()
        val displayHost = params["host"] ?: throw IllegalArgumentException("no host")
        base(displayHost)
        val host = params["address"]?.also {
            require(isNumericEndpoint(it)) { "control address must be numeric" }
        } ?: displayHost
        val key = params["key"] ?: throw IllegalArgumentException("no key")
        return PairingInfo(host, key, Usign.parsePublicKey(key).fingerprint, displayHost)
    }

    /** Fetch the router's public key for pairing (trust on first use, compare fingerprints). */
    fun fetchPairingInfo(host: String): PairingInfo {
        val lines = get("${base(host)}/cgi-bin/mast?pubkey").lines().filter { it.isNotBlank() }
        require(lines.size >= 2) { "unexpected reply" }
        val key = Usign.parsePublicKey(lines[0])
        require(key.fingerprint == lines[1].trim()) { "fingerprint mismatch" }
        return PairingInfo(numericEndpoint(host), lines[0].trim(), key.fingerprint, host)
    }

    sealed class Result {
        data class Ok(val payload: String, val state: MastState) : Result()
        data class Failed(val reason: String) : Result()
    }

    /**
     * Fetch, verify and parse. Rejects anything not signed by the pinned key,
     * and replies older than the last accepted one (replayed "no pact").
     */
    fun fetch(store: Store, model: String = ""): Result {
        return fetch(store.host, store.publicKey, store.deviceId, store.phoneKey, model, store.state, store.provisioned,
            store.appsApplied, store.wireSeen)
    }

    /**
     * The request names this phone and a fresh nonce, signed with the phone's
     * own key ([PhoneKey]): the router only answers an approved id+key pair with
     * a binding. [model] is shown to the administrator next to the fingerprint.
     *
     * Both signatures are sent: sig over the v2 message (an older router answers
     * v2) and sig3 over the v3 message, which also binds [appliedRev], the app
     * policy revision this phone actually enforces. Once a v3 reply was accepted
     * ([minWire]), an older reply is a downgrade and refused.
     */
    internal fun fetch(host: String?, key: String?, deviceId: String, phoneKey: PhoneKey, model: String = "",
        previous: MastState? = null, requireDevicePolicy: Boolean = false,
        appliedRev: String? = null, minWire: Int = 0): Result {
        if (host == null || key == null) return Result.Failed("not paired")
        val pub = try { Usign.parsePublicKey(key) } catch (_: IllegalArgumentException) {
            return Result.Failed("bad paired key")
        }
        val nonce = newNonce()
        val requestSignature = phoneKey.sign(requestMessage(deviceId, nonce))
        val applied = appliedRev?.takeIf { it.matches(Regex("[0-9a-f]{16}")) } ?: "-"
        val v3Signature = phoneKey.sign(requestMessageV3(deviceId, nonce, applied))
        val name = model.map { if (it.isLetterOrDigit() && it.code < 128 || it in "._-") it else '-' }.joinToString("").take(40)
        val body = try {
            get("${base(host)}/cgi-bin/mast?device=$deviceId&nonce=$nonce&key=${enc(phoneKey.publicKey)}&sig=${enc(requestSignature)}" +
                "&sig3=${enc(v3Signature)}&applied=$applied&name=$name")
        } catch (e: ReplyRejected) {
            return Result.Failed(e.reason)
        } catch (e: Exception) {
            return Result.Failed("router unreachable")
        }
        val newline = body.indexOf('\n')
        if (newline < 0) return Result.Failed("malformed reply")
        val signature = body.substring(0, newline)
        val payload = body.substring(newline + 1) // Verify the exact signed bytes, including any whitespace.
        if (!Usign.verify(pub, signature, payload.toByteArray(Charsets.UTF_8))) return Result.Failed("bad signature")
        val state = try {
            Payload.parse(payload)
        } catch (e: Exception) {
            return Result.Failed("unreadable payload")
        }
        if (state.wireVersion >= 2 && !responseMatches(state, deviceId, nonce)) {
            return Result.Failed("response mismatch")
        }
        // A router rollback (or a replayed old router) must not silently drop app policy.
        if (state.wireVersion < minWire) return Result.Failed("downgraded reply")
        // Never downgrade an enrolled device to global v1 state after a router rollback,
        // and keep its last policy if the router no longer recognises it.
        if (requireDevicePolicy && state.wireVersion >= 2 && state.pending) return Result.Failed("device not approved")
        if (requireDevicePolicy && !state.appliesTo(deviceId)) return Result.Failed("unsupported device policy")
        if (previous != null && state.serverTime < previous.serverTime) return Result.Failed("stale reply")
        return Result.Ok(payload, state)
    }

    /** "host" or "host:port" (e.g. an emulator reaching a test router on another port). */
    internal fun base(host: String): String {
        val uri = java.net.URI("http://$host")
        require(uri.host != null && uri.rawUserInfo == null && uri.rawQuery == null &&
            uri.rawFragment == null && uri.rawPath.isNullOrEmpty() &&
            (uri.port == -1 || uri.port in 1..65535)) { "invalid router address" }
        return if (uri.port == -1) "http://$host:$PORT" else "http://$host"
    }

    internal fun isNumericEndpoint(endpoint: String): Boolean {
        return runCatching {
            base(endpoint)
            val host = java.net.URI("http://$endpoint").host.removePrefix("[").removeSuffix("]")
            if (':' in host) {
                host.all { it in "0123456789abcdefABCDEF:." } &&
                    java.net.InetAddress.getByName(host) is java.net.Inet6Address
            } else {
                val parts = host.split('.')
                parts.size == 4 && parts.all { p ->
                    p.isNotEmpty() && p.length <= 3 && p.all { it in '0'..'9' } && p.toInt() in 0..255
                }
            }
        }.getOrDefault(false)
    }

    /** Resolve ONCE during manual pairing, then pin the numeric endpoint and router key. */
    internal fun numericEndpoint(endpoint: String): String {
        base(endpoint)
        if (isNumericEndpoint(endpoint)) return endpoint
        val uri = java.net.URI("http://$endpoint")
        val addresses = java.net.InetAddress.getAllByName(uri.host)
        val address = addresses.firstOrNull { it is java.net.Inet4Address } ?: addresses.first()
        val ip = if (address is java.net.Inet6Address) "[${address.hostAddress}]" else address.hostAddress
        return ip + if (uri.port == -1) "" else ":${uri.port}"
    }

    internal fun newNonce(): String = ByteArray(32).also {
        java.security.SecureRandom().nextBytes(it)
    }.joinToString("") { "%02x".format(it) }

    /** Exactly the request this phone made: its device id and its fresh nonce. */
    internal fun responseMatches(state: MastState, deviceId: String, nonce: String): Boolean =
        state.device == deviceId && state.requestNonce == nonce &&
            (state.pending || state.binding?.deviceId == deviceId)

    /** What the phone signs; the router's cli_mast2 checks exactly these bytes. */
    internal fun requestMessage(deviceId: String, nonce: String): ByteArray =
        "familywarden-sync-v2\n$deviceId\n$nonce\n".toByteArray(Charsets.UTF_8)

    /** v3: also binds the applied app policy revision ("-" = none) to this request. */
    internal fun requestMessageV3(deviceId: String, nonce: String, applied: String): ByteArray =
        "familywarden-sync-v3\n$deviceId\n$nonce\n$applied\n".toByteArray(Charsets.UTF_8)

    private fun enc(b64: String) = b64.replace("+", "%2B").replace("/", "%2F").replace("=", "%3D")

    private class ReplyRejected(val reason: String) : Exception(reason)

    private fun get(url: String): String {
        val c = URL(url).openConnection() as HttpURLConnection
        c.connectTimeout = 5000
        c.readTimeout = 5000
        c.useCaches = false
        c.instanceFollowRedirects = false // Captive portals are not alternative control endpoints.
        try {
            if (c.responseCode != 200) throw ReplyRejected("unexpected HTTP reply")
            return c.inputStream.bufferedReader(Charsets.UTF_8).use { reader ->
                val out = StringBuilder()
                val buffer = CharArray(2048)
                while (true) {
                    val n = reader.read(buffer)
                    if (n < 0) break
                    if (out.length + n > 65536) throw ReplyRejected("reply too large")
                    out.append(buffer, 0, n)
                }
                out.toString()
            }
        } finally {
            c.disconnect()
        }
    }
}
