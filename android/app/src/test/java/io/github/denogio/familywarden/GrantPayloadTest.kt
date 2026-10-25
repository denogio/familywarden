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

    private fun shared() = fixture().put("requires", JSONArray().put("apps-v1").put("grants-v1").put("screen-time-v1").put("shared-v1"))
        .put("shared", JSONObject().put("day", "2026-10-21").put("total", 900).put("apps", JSONObject().put("com.roblox.client", 600)))

    @Test fun otherPhonesUsageIsParsedOnlyWhenDeclaredAndBounded() {
        val s = Payload.parse(shared().toString()).shared!!
        assertEquals("2026-10-21", s.day)
        assertEquals(600_000L, s.millis["com.roblox.client"])
        assertEquals(900_000L, s.millis[UsageCounter.TOTAL])
        rejected(shared().put("requires", JSONArray().put("apps-v1").put("grants-v1").put("screen-time-v1")))
        rejected(shared().apply { getJSONObject("shared").put("total", -1) })
        rejected(shared().apply { getJSONObject("shared").put("total", "900") })
        rejected(shared().apply { getJSONObject("shared").getJSONObject("apps").put("../x", 1) })
        rejected(shared().apply { getJSONObject("shared").put("apps", JSONObject((0..64).associate { "com.example.a$it" to 1 })) })
        assertNull(Payload.parse(shared().apply { remove("apps"); getJSONArray("requires") }.put("requires",
            JSONArray().put("shared-v1")).apply { remove("shared") }.toString()).shared)
    }

    @Test fun lifetimeMustBeActiveAtTheSignedServerTimestamp() {
        rejected(fixture().apply { getJSONObject("apps").getJSONArray("grants").getJSONObject(0)
            .put("not_before", getLong("server_time") + 1).put("not_after", getLong("server_time") + 7201) })
        rejected(fixture().apply { getJSONObject("apps").getJSONArray("grants").getJSONObject(0)
            .put("not_before", getLong("server_time") - 7200).put("not_after", getLong("server_time")) })
    }
}
