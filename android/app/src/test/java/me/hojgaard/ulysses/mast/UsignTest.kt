package me.hojgaard.ulysses.mast

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Fixtures in src/test/resources were made with OpenWrt 25.12's real usign. */
class UsignTest {
    private fun res(name: String) = javaClass.classLoader!!.getResourceAsStream(name)!!.readBytes()
    private fun text(name: String) = String(res(name)).trim()

    private val pub = Usign.parsePublicKey(text("router.pub"))
    private val payload = res("payload.json")
    private val sig = text("payload.sig")

    @Test fun fingerprintMatchesUsignF() = assertEquals(text("router.fingerprint"), pub.fingerprint)

    @Test fun verifiesRealUsignSignature() = assertTrue(Usign.verify(pub, sig, payload))

    @Test fun rejectsTamperedPayload() {
        val forged = String(payload).replace("\"active\": true", "\"active\": false").toByteArray()
        assertFalse(Usign.verify(pub, sig, forged))
    }

    @Test fun rejectsOtherKey() = assertFalse(Usign.verify(Usign.parsePublicKey(text("other.pub")), sig, payload))

    @Test fun rejectsGarbageSignature() {
        assertFalse(Usign.verify(pub, "not base64!!", payload))
        assertFalse(Usign.verify(pub, "RWQ=", payload))
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsNonUsignKey() {
        Usign.parsePublicKey("AAAA")
    }

    @Test fun signedPayloadParses() {
        val s = Payload.parse(String(payload))
        assertEquals("dennis", s.captain)
        assertTrue(s.pactActive)
        assertEquals("ongoing", s.pactType)
        assertEquals(7, s.rules.windows.single().days.size)
    }
}
