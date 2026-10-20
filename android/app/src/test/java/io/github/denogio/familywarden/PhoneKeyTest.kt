package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test

class PhoneKeyTest {
    @Test fun usignRequestRoundTripAndTampering() {
        val phone = PhoneKey.generate()
        val bytes = Sync.requestMessage("fw-example", "a".repeat(64))
        assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), phone.sign(bytes), bytes))
        assertFalse(Usign.verify(Usign.parsePublicKey(phone.publicKey), phone.sign(bytes), bytes + 0))
        val restored = PhoneKey.decode(phone.encode())
        assertEquals(phone.publicKey, restored.publicKey)
        assertEquals(phone.fingerprint, restored.fingerprint)
        assertEquals(phone.sign(bytes), restored.sign(bytes))
    }

    @Test fun fingerprintAuthenticatesFullKeyNotJustCallerChosenKeyId() {
        val a = PhoneKey.generate()
        val b = PhoneKey.generate()
        val sameId = PhoneKey.decode(a.encode().substringBefore(':') + ":" + b.encode().substringAfter(':'))
        assertEquals(Usign.parsePublicKey(a.publicKey).fingerprint, Usign.parsePublicKey(sameId.publicKey).fingerprint)
        assertNotEquals(a.fingerprint, sameId.fingerprint)
        assertTrue(a.fingerprint.matches(Regex("[0-9a-f]{64}")))
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsOversizedSeedInsteadOfIgnoringTrailingBytes() {
        val encoded = PhoneKey.generate().encode()
        val bytes = java.util.Base64.getDecoder().decode(encoded.substringAfter(':'))
        PhoneKey.decode(encoded.substringBefore(':') + ":" + java.util.Base64.getEncoder().encodeToString(bytes + 0))
    }
}
