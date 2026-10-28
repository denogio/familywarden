package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class UsageUploadTest {
    private val day = "2027-01-15"
    private fun report(millis: Map<String, Long>, grants: Map<String, Long> = emptyMap(), labels: Map<String, String> = emptyMap()) =
        JSONObject(UsageUpload.report("fw-alex", "alex", day, 7, millis, labels, grants, false))

    @Test fun reportMatchesTheRouterContractAndCountsWholeSeconds() {
        val o = report(mapOf(UsageCounter.TOTAL to 900_999, "com.example.video" to 600_500, "com.example.game" to 300_000),
            mapOf("a".repeat(32) to 61_000L), mapOf("com.example.video" to "Video"))
        // Exactly the fields the Go strict decoder knows (type v device profile day seq total apps labels grants_used incomplete).
        assertEquals(setOf("type", "v", "device", "profile", "day", "seq", "total", "apps", "labels", "grants_used", "incomplete"),
            o.keys().asSequence().toSet())
        assertEquals(900, o.getInt("total"))
        assertEquals(600, o.getJSONObject("apps").getInt("com.example.video"))
        assertFalse(o.getJSONObject("apps").has(UsageCounter.TOTAL))
        assertEquals(61, o.getJSONObject("grants_used").getInt("a".repeat(32)))
        assertEquals("Video", o.getJSONObject("labels").getString("com.example.video"))
        assertFalse(o.getJSONObject("labels").has("com.example.game"))
    }

    @Test fun flooringKeepsCumulativeReportsMonotonic() {
        var previous = -1
        for (ms in listOf(0L, 999L, 1_000L, 1_999L, 59_999L, 60_000L)) {
            val now = report(mapOf(UsageCounter.TOTAL to ms)).getInt("total")
            assertTrue(now >= previous)
            previous = now
        }
    }

    @Test fun reportIsBoundedAndDropsZeroOrInvalidEntries() {
        val many = (0 until 400).associate { "com.example.a$it" to (it + 1) * 1_000L } +
            mapOf("not a package" to 5_000L, "com.example.zero" to 999L, UsageCounter.TOTAL to Long.MAX_VALUE)
        val o = report(many)
        assertEquals(256, o.getJSONObject("apps").length())
        assertEquals(25 * 3600, o.getInt("total"))
        assertTrue(o.getJSONObject("apps").has("com.example.a399"))
        assertFalse(o.getJSONObject("apps").has("com.example.zero"))
        try { report(emptyMap(), mapOf("../grant" to 1L)); fail("invalid grant id") } catch (_: IllegalArgumentException) { }
        try { UsageUpload.report("fw-alex", "alex", "2027-1-5", 1, emptyMap(), emptyMap(), emptyMap(), false); fail() }
        catch (_: Exception) { }
    }

    @Test fun labelsAreUntrustedBoundedText() {
        assertEquals("Video", UsageUpload.label("  Vi\u0000deo\n"))
        assertNull(UsageUpload.label("\u0007"))
        assertEquals(64, UsageUpload.label("😀".repeat(100))!!.codePointCount(0, 128))
    }

    @Test fun parentUsageIsStrictlyScoped() {
        val alex = ProfileRef("alex", "Alex")
        fun entry(profile: String = "alex") = JSONObject().put("profile_id", profile).put("day", day).put("total", 900)
            .put("devices", JSONArray().put(JSONObject().put("name", "Alex's phone").put("total", 900).put("incomplete", true)))
            .put("apps", JSONArray().put(JSONObject().put("package", "com.example.video").put("label", "Video").put("seconds", 600)))
        val u = UsageUpload.parse(JSONArray().put(entry()), listOf(alex)).single()
        assertEquals(900, u.totalSeconds)
        assertTrue(u.devices.single().incomplete)
        assertEquals("Video", u.apps.single().label)
        for (bad in listOf(JSONArray().put(entry("sam")), JSONArray().put(entry()).put(entry()),
            JSONArray().put(entry().put("total", -1)), JSONArray().put(entry().put("total", "900")),
            JSONArray().put(entry().put("day", "today")))) {
            try { UsageUpload.parse(bad, listOf(alex)); fail("accepted $bad") } catch (_: Exception) { }
        }
    }

    @Test fun receiptMetadataIsOptionalForOldRoutersButStrictWhenPresent() {
        val profile = ProfileRef("alex", "Alex")
        fun entry(received: Any?) = JSONObject().put("profile_id", profile.id).put("day", day).put("total", 0)
            .put("devices", JSONArray().put(JSONObject().put("name", "Alex phone").put("total", 0).put("incomplete", false)
                .apply { if (received != null) put("received", received) })).put("apps", JSONArray())
        assertNull(UsageUpload.parse(JSONArray().put(entry(null)), listOf(profile)).single().devices.single().received)
        assertEquals(1800000000L, UsageUpload.parse(JSONArray().put(entry(1800000000L)), listOf(profile)).single().devices.single().received)
        for (bad in listOf(0, -1, 253402300800L, "1800000000", 1800000000.5, JSONObject.NULL)) {
            try { UsageUpload.parse(JSONArray().put(entry(bad)), listOf(profile)); fail("bad receipt accepted") }
            catch (_: Exception) { }
        }
    }

    @Test fun usageUploadUsesItsOwnSigningDomainAndReplyType() {
        val router = PhoneKey.generate()
        val phone = PhoneKey.generate()
        val raw = UsageUpload.report("fw-alex", "alex", day, 7, mapOf(UsageCounter.TOTAL to 60_000L), emptyMap(), emptyMap(), false)
        val upload = RequestUpload("usage", raw, router.publicKey, "fw-alex")
        val body = upload.signedBody(phone)
        assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), body.copyOfRange(0, 100).toString(Charsets.US_ASCII),
            "familywarden-usage-v1\n".toByteArray() + raw.toByteArray()))
        val reply = JSONObject().put("v", 1).put("type", "usage_reply").put("device", "fw-alex").put("server_time", 1800000000)
            .put("message_sha256", RequestProtocol.digest(raw.toByteArray())).put("day", day).put("seq", 7).put("state", "stored")
            .toString().toByteArray()
        val signed = (router.sign(reply) + "\n").toByteArray() + reply
        assertEquals("stored", RequestProtocol.verifyReply(signed, upload, 1800000000).state)
        val wrongType = JSONObject(String(reply)).put("type", "request_reply").toString().toByteArray()
        try { RequestProtocol.verifyReply((router.sign(wrongType) + "\n").toByteArray() + wrongType, upload, 1800000000); fail() }
        catch (_: Exception) { }
    }

    @Test fun waitingAppsAreReportedOnlyWhenGivenAndBounded() {
        assertFalse(report(emptyMap()).has("new_apps"))
        val many = (0 until 40).associate { "com.example.n$it" to "N$it\u0000" } + ("bad pkg" to "X")
        val o = JSONObject(UsageUpload.report("fw-alex", "alex", day, 7, emptyMap(), emptyMap(), emptyMap(), false, many))
        val apps = o.getJSONObject("new_apps")
        assertEquals(32, apps.length())
        assertFalse(apps.has("bad pkg"))
        assertEquals("N0", apps.getString("com.example.n0"))
    }
}
