package io.github.denogio.familywarden

import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import java.util.Base64

/**
 * Verifies OpenWrt usign (Ed25519) signatures, as made by familywarden with
 * `usign -S`. Formats (base64-decoded):
 *   public key: "Ed" | 8-byte key id | 32-byte key
 *   signature:  "Ed" | 8-byte key id | 64-byte signature over the raw message
 */
object Usign {
    class PublicKey(val keyId: ByteArray, val key: ByteArray) {
        /** Same text as `usign -F`, so it can be compared with LuCI. */
        val fingerprint: String get() = keyId.joinToString("") { "%02x".format(it) }
    }

    fun parsePublicKey(b64: String): PublicKey {
        val raw = Base64.getDecoder().decode(b64.trim())
        require(raw.size == 42 && raw[0] == 'E'.code.toByte() && raw[1] == 'd'.code.toByte()) { "not a usign Ed25519 public key" }
        return PublicKey(raw.copyOfRange(2, 10), raw.copyOfRange(10, 42))
    }

    fun verify(pub: PublicKey, signatureB64: String, message: ByteArray): Boolean {
        val raw = try {
            Base64.getDecoder().decode(signatureB64.trim())
        } catch (e: IllegalArgumentException) {
            return false
        }
        if (raw.size != 74 || raw[0] != 'E'.code.toByte() || raw[1] != 'd'.code.toByte()) return false
        if (!raw.copyOfRange(2, 10).contentEquals(pub.keyId)) return false
        val signer = Ed25519Signer()
        signer.init(false, Ed25519PublicKeyParameters(pub.key, 0))
        signer.update(message, 0, message.size)
        return signer.verifySignature(raw.copyOfRange(10, 74))
    }
}
