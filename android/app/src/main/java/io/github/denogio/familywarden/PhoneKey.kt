package io.github.denogio.familywarden

import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import java.security.SecureRandom
import java.security.MessageDigest
import java.util.Base64

/**
 * This phone's own usign (Ed25519) identity. The router verifies its request
 * signatures with `usign -V`, and an administrator approves its fingerprint in
 * LuCI, so the formats are usign's (see [Usign]). Kept in app-private storage:
 * Android Keystore has no Ed25519 on API 30.
 */
class PhoneKey private constructor(private val keyId: ByteArray, private val key: Ed25519PrivateKeyParameters) {
    /** Base64 usign public key: "Ed" | key id | public key. */
    val publicKey: String
        get() = b64("Ed".toByteArray() + keyId + key.generatePublicKey().encoded)

    /** Full-key SHA-256, not usign's caller-chosen key ID (which can be copied). */
    val fingerprint: String get() = MessageDigest.getInstance("SHA-256")
        .digest(publicKey.toByteArray(Charsets.US_ASCII)).joinToString("") { "%02x".format(it) }

    /** Base64 usign signature: "Ed" | key id | signature. */
    fun sign(message: ByteArray): String {
        val signer = Ed25519Signer()
        signer.init(true, key)
        signer.update(message, 0, message.size)
        return b64("Ed".toByteArray() + keyId + signer.generateSignature())
    }

    fun encode(): String = keyId.joinToString("") { "%02x".format(it) } + ":" + b64(key.encoded)

    companion object {
        fun generate(random: SecureRandom = SecureRandom()) =
            PhoneKey(ByteArray(8).also(random::nextBytes), Ed25519PrivateKeyParameters(random))

        fun decode(stored: String): PhoneKey {
            val (id, seed) = stored.split(":", limit = 2)
            require(id.matches(Regex("[0-9a-f]{16}"))) { "invalid key id" }
            val keyId = id.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
            val bytes = Base64.getDecoder().decode(seed)
            require(bytes.size == 32) { "invalid private seed" }
            return PhoneKey(keyId, Ed25519PrivateKeyParameters(bytes, 0))
        }

        private fun b64(bytes: ByteArray) = Base64.getEncoder().encodeToString(bytes)
    }
}
