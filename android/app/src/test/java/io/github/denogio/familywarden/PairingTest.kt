package io.github.denogio.familywarden

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

    @Test fun numericControlAddressDoesNotNeedLocalDns() {
        val info = Sync.parsePairingUri("familywarden://pair?host=router.invalid&address=10.77.0.1&key=${encoded(key)}")
        assertEquals("10.77.0.1", info.host)
        assertEquals("router.invalid", info.displayHost)
        assertEquals(fingerprint, info.fingerprint)
        assertEquals("http://10.77.0.1:7700", Sync.base(info.host))
    }

    @Test fun numericAddressKeepsExplicitControlPort() {
        val info = Sync.parsePairingUri("ulyssesmast://pair?host=router.lan&address=192.0.2.1:7777&key=${encoded(key)}")
        assertEquals("http://192.0.2.1:7777", Sync.base(info.host))
    }

    @Test fun acceptsNumericIpv6WithoutDnsResolution() {
        assertEquals(true, Sync.isNumericEndpoint("[fd00::1]:7700"))
        assertEquals("http://[fd00::1]:7700", Sync.base("[fd00::1]"))
    }

    @Test fun numericValidationRejectsNamesAndMalformedAddresses() {
        for (value in listOf("router.lan", "999.0.0.1", "127.1", "a/b", "10.0.0.1:0", "user@10.0.0.1")) {
            assertEquals(value, false, Sync.isNumericEndpoint(value))
        }
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsDnsNameAsNumericFallback() {
        Sync.parsePairingUri("familywarden://pair?host=router.lan&address=other.lan&key=${encoded(key)}")
    }

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
