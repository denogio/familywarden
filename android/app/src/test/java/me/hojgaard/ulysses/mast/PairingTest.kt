package me.hojgaard.ulysses.mast

import org.junit.Assert.assertEquals
import org.junit.Test

class PairingTest {
    private val key = javaClass.classLoader!!.getResourceAsStream("router.pub")!!.readBytes().let { String(it).trim() }
    private val fingerprint = javaClass.classLoader!!.getResourceAsStream("router.fingerprint")!!.readBytes().let { String(it).trim() }

    // Same encoding as the router's ubus "mast" (+ / = percent-encoded)
    private fun encoded(k: String) = k.replace("+", "%2B").replace("/", "%2F").replace("=", "%3D")

    @Test fun parsesLuciQr() {
        val info = Sync.parsePairingUri("ulyssesmast://pair?host=192.168.1.1&key=${encoded(key)}")
        assertEquals("192.168.1.1", info.host)
        assertEquals(key, info.publicKey)
        assertEquals(fingerprint, info.fingerprint)
    }

    @Test fun acceptsHostnames() =
        assertEquals("rivendell.lan", Sync.parsePairingUri("ulyssesmast://pair?host=rivendell.lan&key=${encoded(key)}").host)

    @Test(expected = IllegalArgumentException::class)
    fun rejectsOtherSchemes() {
        Sync.parsePairingUri("https://pair?host=192.168.1.1&key=${encoded(key)}")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsInjectedHost() {
        Sync.parsePairingUri("ulyssesmast://pair?host=evil%2Fx%3Fy&key=${encoded(key)}")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsBadKey() {
        Sync.parsePairingUri("ulyssesmast://pair?host=192.168.1.1&key=AAAA")
    }
}
