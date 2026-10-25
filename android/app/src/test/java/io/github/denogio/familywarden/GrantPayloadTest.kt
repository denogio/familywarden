package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class GrantPayloadTest {
    private fun fixture(): JSONObject {
        val raw = javaClass.classLoader!!.getResourceAsStream("v3-payload.json")!!.readBytes().toString(Charsets.UTF_8)
        return JSONObject(raw).apply {
            put("requires", JSONArray().put("apps-v1").put("grants-v1").put("screen-time-v1"))
            getJSONObject("apps").put("grants", JSONArray().put(JSONObject().put("grant_id", "a".repeat(32))
                .put("device_id", "fw-fixture").put("target", "com.roblox.client").put("minutes", 20)
                .put("not_before", getLong("server_time")).put("not_after", getLong("server_time") + 7200)))
        }
    }
    private fun rejected(o: JSONObject) {
        try { Payload.parse(o.toString()); fail("unsafe grant policy accepted") } catch (_: Exception) { }
    }

    @Test fun completeRouterPolicyCarriesADeviceBoundGrant() {
        val state = Payload.parse(fixture().toString())
        assertEquals("fw-fixture", state.apps!!.grants.single().deviceId)
        assertEquals(20, state.apps.grants.single().minutes)
        assertEquals("com.roblox.client", state.apps.grants.single().target)
    }

    @Test fun grantFeatureMustBeDeclaredAndCannotBeTrustedOnAnOlderWire() {
        rejected(fixture().put("requires", JSONArray().put("apps-v1").put("screen-time-v1")))
        rejected(fixture().apply { remove("requires") })
        assertNull(Payload.parse(fixture().put("v", 2).toString()).apps)
    }

    @Test fun signatureAuthorityIsNotAuthorityForAnotherDeviceOrAPact() {
        rejected(fixture().apply { getJSONObject("apps").getJSONArray("grants").getJSONObject(0).put("device_id", "fw-other") })
        rejected(fixture().apply {
            put("captain", "fixture")
            getJSONObject("binding").put("captain", true)
            getJSONObject("pact").put("active", true).put("type", "ongoing")
        })
    }

    @Test fun lifetimeMustBeActiveAtTheSignedServerTimestamp() {
        rejected(fixture().apply { getJSONObject("apps").getJSONArray("grants").getJSONObject(0)
            .put("not_before", getLong("server_time") + 1).put("not_after", getLong("server_time") + 7201) })
        rejected(fixture().apply { getJSONObject("apps").getJSONArray("grants").getJSONObject(0)
            .put("not_before", getLong("server_time") - 7200).put("not_after", getLong("server_time")) })
    }
}
