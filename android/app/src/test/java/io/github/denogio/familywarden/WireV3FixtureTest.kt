package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test

/**
 * A real v3 reply produced by the router's shell plugin (jshn + usign), so the
 * Kotlin side is checked against the actual bytes, not a hand-written JSON.
 * Regenerate after wire changes; see the test-resource README in plans/app-control-protocol.md.
 */
class WireV3FixtureTest {
    private fun text(name: String) = javaClass.classLoader!!.getResourceAsStream(name)!!.readBytes().toString(Charsets.UTF_8)

    @Test fun routerSignedV3ReplyVerifiesAndParses() {
        val payload = text("v3-payload.json")
        val router = Usign.parsePublicKey(text("v3-router.pub").trim())
        assertTrue(Usign.verify(router, text("v3-payload.sig").trim(), payload.toByteArray(Charsets.UTF_8)))
        assertEquals("apps-v1", org.json.JSONObject(payload).getJSONArray("requires").getString(0))
        assertEquals("screen-time-v1", org.json.JSONObject(payload).getJSONArray("requires").getString(1))
        val s = Payload.parse(payload)
        assertTrue(s.appliesTo("fw-fixture"))
        assertTrue(Sync.responseMatches(s, "fw-fixture", "c".repeat(64)))
        val apps = s.apps!!
        assertTrue(apps.suspendNew)
        assertEquals(mapOf("com.roblox.client" to 45), apps.limitsMinutes)
        assertEquals(setOf("com.zhiliaoapp.musically"), apps.blocked)
        assertEquals(setOf("com.roblox.client", "com.spotify.music"), apps.approved)
        assertEquals(1, apps.downtime.size)
    }

    @Test fun routerSignedCapabilityRefusalIsVerifiedButNeverBecomesPolicy() {
        val payload = text("v3-refused.json")
        assertTrue(Usign.verify(Usign.parsePublicKey(text("v3-router.pub").trim()),
            text("v3-refused.sig").trim(), payload.toByteArray(Charsets.UTF_8)))
        try { Payload.parse(payload); fail("capability refusal became policy") }
        catch (_: PolicyCapabilities.Unsupported) { }
    }

    @Test fun capabilityMessageHasItsOwnDomainAndExactRouterByteLayout() {
        assertEquals("familywarden-sync-caps-v1\nfw-fixture\n${"c".repeat(64)}\n-\nscreen-time-v1\n",
            Sync.requestCapabilitiesMessage("fw-fixture", "c".repeat(64), "-", "screen-time-v1").toString(Charsets.UTF_8))
    }

    @Test fun v3RequestMessageIsByteForByteWhatTheRouterVerifies() {
        // cli_mast2/_phone_sig_ok: printf 'familywarden-sync-v3\n%s\n%s\n%s\n'
        assertEquals("familywarden-sync-v3\nfw-fixture\n${"c".repeat(64)}\n-\n",
            Sync.requestMessageV3("fw-fixture", "c".repeat(64), "-").toString(Charsets.UTF_8))
    }
}
