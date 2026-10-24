package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class DevicePolicyTest {
    private val nonce = "a".repeat(64)
    private fun payload(): JSONObject = JSONObject("""{
      "v":2,"server_time":1800000000,"nonce":"$nonce","timezone":"Europe/Copenhagen",
      "device":"fw-example","enrollment":"approved",
      "binding":{"device_id":"fw-example","profile_id":"captain","profile_name":"Alex",
        "captain":true,"administrator":true,"can_self_remove":true},
      "captain":"captain","pact":{"active":false,"type":"","until":0,"release_at":0},
      "rules":{"enabled":"1","policy":"scheduled","override":"none","windows":[]},
      "dns":{"hostname":"alex.dns.example.net","protections":["filter_lists","safe_search"]}
    }""")
    private fun parsed(o: JSONObject = payload()) = Payload.parse(o.toString())

    @Test(expected = IllegalArgumentException::class)
    fun unknownRequiredFeatureCannotBeSilentlyIgnored() {
        parsed(payload().put("v", 3).put("requires", org.json.JSONArray().put("future-parent-approval")))
    }

    @Test fun understoodAndAbsentRequirementsKeepLegacyCompatibility() {
        assertTrue(parsed(payload().put("v", 3).put("requires", org.json.JSONArray().put("screen-time-v1"))).appliesTo("fw-example"))
        assertTrue(parsed().appliesTo("fw-example"))
    }

    @Test fun requirementsAreStrictBoundedUniqueStringsEvenOnPendingReplies() {
        val invalid = listOf<Any>(JSONObject.NULL, "screen-time-v1", org.json.JSONArray().put(42),
            org.json.JSONArray().put("screen-time-v1").put("screen-time-v1"),
            org.json.JSONArray().put("SCREEN-TIME-V1"), org.json.JSONArray((1..17).map { "feature-$it" }))
        invalid.forEach { value ->
            val o = payload().put("v", 3).put("enrollment", "pending").put("requires", value)
            try { parsed(o); fail("accepted invalid requirements: $value") } catch (_: IllegalArgumentException) { }
        }
    }

    @Test(expected = IllegalArgumentException::class)
    fun signedCapabilityRefusalIsNotAnApprovedOrPendingPolicy() {
        parsed(payload().put("v", 3).put("error", "unsupported_capabilities"))
    }

    private val parentKey = PhoneKey.generate().publicKey
    private fun scoped(): JSONObject = payload().put("v", 3).apply {
        getJSONObject("binding").put("approves", org.json.JSONArray()
            .put(JSONObject().put("profile_id", "kid").put("profile_name", "Kid")))
        put("parents", org.json.JSONArray()
            .put(JSONObject().put("device_id", "fw-parent").put("name", "Taylor phone").put("pubkey", parentKey)))
    }

    @Test fun parsesSignedParentScopeAndParentKeys() {
        val s = parsed(scoped())
        assertEquals(listOf(ProfileRef("kid", "Kid")), s.binding!!.approves)
        assertEquals(listOf(ParentKey("fw-parent", "Taylor phone", parentKey)), s.parents)
    }

    @Test fun parentFieldsAreOptionalAndNeverTrustedFromOlderVersions() {
        assertTrue(parsed().binding!!.approves.isEmpty())
        val old = scoped().put("v", 2)
        assertTrue(parsed(old).binding!!.approves.isEmpty())
        assertTrue(parsed(old).parents.isEmpty())
    }

    @Test fun parentScopeAndKeysAreStrict() {
        fun bad(change: (JSONObject) -> Unit) {
            val o = scoped().also(change)
            try { parsed(o); fail("accepted: $o") } catch (_: Exception) { }
        }
        bad { it.getJSONObject("binding").getJSONArray("approves").put(JSONObject().put("profile_id", "captain").put("profile_name", "Me")) }
        bad { it.getJSONObject("binding").getJSONArray("approves").put(JSONObject().put("profile_id", "kid").put("profile_name", "Kid")) }
        bad { it.getJSONObject("binding").getJSONArray("approves").put(JSONObject().put("profile_id", "../x").put("profile_name", "X")) }
        bad { it.getJSONArray("parents").getJSONObject(0).put("pubkey", "not-a-key") }
        bad { it.getJSONArray("parents").put(JSONObject().put("device_id", "fw-parent").put("name", "Again").put("pubkey", parentKey)) }
        bad { it.getJSONArray("parents").put(JSONObject().put("device_id", "fw-example").put("name", "Self").put("pubkey", parentKey)) }
        bad { o -> o.put("parents", org.json.JSONArray((1..17).map { JSONObject().put("device_id", "fw-p$it").put("name", "P").put("pubkey", parentKey) })) }
        // A pact-bound captain trusts no parent keys, whatever the router sends.
        bad { it.put("pact", JSONObject().put("active", true).put("type", "ongoing").put("until", 0).put("release_at", 0)) }
    }

    @Test fun acknowledgementsFromOlderImplementationsMustBeReappliedNotReused() {
        val revision = "0123456789abcdef"
        assertNull(PolicyCapabilities.acknowledgedRevision(revision, null))
        assertNull(PolicyCapabilities.acknowledgedRevision(revision, "apps-v1"))
        assertNull(PolicyCapabilities.acknowledgedRevision(null, PolicyCapabilities.advertised))
        assertNull(PolicyCapabilities.acknowledgedRevision("bad", PolicyCapabilities.advertised))
        assertEquals(revision, PolicyCapabilities.acknowledgedRevision(revision, PolicyCapabilities.advertised))
    }

    @Test fun pendingReplyIsBoundToRequestButNeverGrantsPolicyOrRemoval() {
        val o = JSONObject().put("v", 2).put("server_time", 1800000000)
            .put("device", "fw-example").put("nonce", nonce).put("enrollment", "pending")
        val state = parsed(o)
        assertTrue(state.pending)
        assertTrue(Sync.responseMatches(state, "fw-example", nonce))
        assertFalse(Sync.responseMatches(state, "fw-other", nonce))
        assertFalse(Sync.responseMatches(state, "fw-example", "b".repeat(64)))
        assertNull(state.binding)
        assertNull(state.dns)
        assertFalse(state.canSelfRemove("fw-example", state.serverTime))
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsBindingForDifferentResponseDevice() { parsed(payload().put("device", "fw-other")) }

    @Test fun parsesScopedPolicyAndRoles() {
        val s = parsed()
        assertTrue(s.appliesTo("fw-example"))
        assertFalse(s.appliesTo("another-phone"))
        assertEquals("Europe/Copenhagen", s.timeZone)
        assertEquals(setOf(Protection.FILTER_LISTS, Protection.SAFE_SEARCH), s.dns!!.protections)
    }

    @Test fun nonceAndDeviceBothMustMatch() {
        val s = parsed()
        assertTrue(Sync.responseMatches(s, "fw-example", nonce))
        assertFalse(Sync.responseMatches(s, "fw-example", "b".repeat(64)))
        assertFalse(Sync.responseMatches(s, "another-phone", nonce))
    }

    @Test fun randomRequestNoncesHaveExpectedShape() {
        val a = Sync.newNonce(); val b = Sync.newNonce()
        assertTrue(a.matches(Regex("[a-f0-9]{64}")))
        assertNotEquals(a, b)
    }

    @Test fun removalNeedsExplicitAuthorityAndNoPact() {
        val s = parsed()
        assertTrue(s.canSelfRemove("fw-example", 1800000000))
        assertFalse(s.canSelfRemove("other", 1800000000))
        assertFalse(s.copy(binding = s.binding!!.copy(administrator = false, selfRemoval = false)).canSelfRemove("fw-example", 1800000000))
        assertTrue(s.copy(binding = s.binding!!.copy(administrator = false, selfRemoval = true)).canSelfRemove("fw-example", 1800000000))
        assertFalse(s.copy(binding = s.binding!!.copy(selfRemoval = false)).canSelfRemove("fw-example", 1800000000))
        assertFalse(s.copy(pactActive = true, pactType = "ongoing").canSelfRemove("fw-example", 1800000000))
    }

    @Test fun fastPhoneClockCannotAuthorizeFreshFixedPactRemoval() {
        val base = parsed()
        val state = base.copy(pactActive = true, pactType = "until", pactUntil = base.serverTime + 3600)
        // Local UI expiry is not authority: the newly signed server time is still bound.
        assertTrue(state.canSelfRemove("fw-example", base.serverTime + 7200))
        assertFalse(Sync.Result.Ok("test", state).authorizesSelfRemoval("fw-example"))
    }

    @Test fun legacyGlobalPayloadDoesNotGrantDeviceAuthority() {
        val s = parsed(payload().put("v", 1))
        assertNull(s.binding)
        assertNull(s.dns)
        assertFalse(s.canSelfRemove("fw-example", 1800000000))
    }

    @Test fun validatesDnsHostnames() {
        for (name in listOf("alex.dns.example.net", "a-b.example.org", "DNS.EXAMPLE.NET")) assertTrue(validHostname(name))
        for (name in listOf("10.77.0.11", "https://dns.example.net", "dns.example.net:853", "bad_name.example.net", "-bad.example.net", "bad..example.net", "dns.example.net.", "localhost")) assertFalse(name, validHostname(name))
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsUnscopedCaptainPact() {
        val o = payload()
        o.getJSONObject("binding").put("captain", false)
        o.getJSONObject("pact").put("active", true).put("type", "ongoing")
        parsed(o)
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsAnotherProfilesPact() {
        val o = payload().put("captain", "other-profile")
        o.getJSONObject("pact").put("active", true).put("type", "ongoing")
        parsed(o)
    }

    @Test(expected = Exception::class)
    fun rejectsMissingPactRatherThanDefaultingToNoPact() { parsed(payload().apply { remove("pact") }) }

    @Test(expected = Exception::class)
    fun rejectsUnknownProtectionRatherThanPretendingApplied() {
        val o = payload()
        o.getJSONObject("dns").getJSONArray("protections").put("invented_filter")
        parsed(o)
    }

    @Test(expected = Exception::class)
    fun rejectsMalformedDnsRatherThanTreatingItAsUnconfigured() { parsed(payload().put("dns", "invalid")) }

    @Test(expected = Exception::class)
    fun rejectsInvalidTimezone() { parsed(payload().put("timezone", "not/a/zone")) }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsUnknownWeekdayRatherThanSilentlyDroppingIt() {
        val o = payload()
        o.getJSONObject("rules").getJSONArray("windows").put(JSONObject()
            .put("days", "mon never").put("start", "07:00").put("stop", "20:00"))
        parsed(o)
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsSecondPrecisionUnsupportedByScheduleContract() {
        val o = payload()
        o.getJSONObject("rules").getJSONArray("windows").put(JSONObject()
            .put("days", "mon").put("start", "07:00:30").put("stop", "20:00"))
        parsed(o)
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsUnicodeHostnameBeforeCaseNormalization() {
        val o = payload()
        o.getJSONObject("dns").put("hostname", "Kid.dns.example.net")
        parsed(o)
    }

    @Test fun rejectsOverflowingOrMillisecondEpochs() {
        for (field in listOf("server_time", "until", "release_at")) {
            val o = payload()
            if (field == "server_time") o.put(field, 1800000000000L)
            else o.getJSONObject("pact").put(field, Long.MAX_VALUE)
            assertTrue(field, runCatching { parsed(o) }.exceptionOrNull() is IllegalArgumentException)
        }
    }

    private fun v3(): JSONObject = payload().put("v", 3).put("apps", JSONObject("""{
      "rev":"3d3515b169db9112","suspend_new":true,
      "limits":[{"package":"com.roblox.client","minutes":30}],
      "blocked":["com.zhiliaoapp.musically"],
      "approved":["com.roblox.client","com.spotify.music"],
      "downtime":[{"days":"mon tue wed thu","start":"21:00","stop":"07:00"}]
    }"""))

    @Test fun v3IsV2PlusSignedAppPolicy() {
        val s = parsed(v3())
        assertEquals(3, s.wireVersion)
        assertTrue(s.appliesTo("fw-example"))
        assertTrue(s.canSelfRemove("fw-example", 1800000000))
        val apps = s.apps!!
        assertEquals("3d3515b169db9112", apps.rev)
        assertEquals(mapOf("com.roblox.client" to 30), apps.limitsMinutes)
        assertEquals(setOf("com.zhiliaoapp.musically"), apps.blocked)
        assertEquals(1, apps.downtime.size)
        val local = apps.toLocal(java.time.ZoneId.of("Europe/Copenhagen"), java.time.LocalDate.parse("2026-10-21"))
        assertEquals(1_800_000L, local.limitsMillis["com.roblox.client"])
        assertTrue(local.suspendNewUserApps)
    }

    @Test fun v3WithoutAppsMeansNoAppControl() { assertNull(parsed(v3().apply { remove("apps") }).apps) }

    @Test fun appsInAnOlderPayloadAreNeverTrusted() { assertNull(parsed(v3().put("v", 2)).apps) }

    @Test fun pendingV3ReplyCarriesNoPolicy() {
        val o = JSONObject().put("v", 3).put("server_time", 1800000000)
            .put("device", "fw-example").put("nonce", nonce).put("enrollment", "pending")
            .put("apps", v3().getJSONObject("apps"))
        val s = parsed(o)
        assertTrue(s.pending)
        assertNull(s.apps)
        assertFalse(s.appliesTo("fw-example"))
    }

    @Test fun contradictoryOrOversizedAppPolicyIsRejectedNotRepaired() {
        val bad = listOf<(JSONObject) -> Unit>(
            { it.put("rev", "not-hex") },
            { it.getJSONArray("approved").put("com.zhiliaoapp.musically") },          // blocked and approved
            { it.getJSONArray("limits").put(JSONObject().put("package", "com.other.app").put("minutes", 5)) },
            { it.getJSONArray("limits").getJSONObject(0).put("minutes", 1441) },
            { it.getJSONArray("limits").put(JSONObject().put("package", "com.roblox.client").put("minutes", 5)) },
            { it.getJSONArray("blocked").put("../etc") },
            { it.getJSONArray("approved").put("com.spotify.music") },                   // duplicate
            { it.getJSONArray("downtime").getJSONObject(0).put("days", "someday") },
            { it.getJSONArray("downtime").getJSONObject(0).put("start", "9:00") },
            { it.remove("suspend_new") },
            { a -> val arr = a.getJSONArray("blocked"); repeat(257) { arr.put("com.example.app$it") } },
        )
        bad.forEachIndexed { i, mutate ->
            val o = v3()
            mutate(o.getJSONObject("apps"))
            assertTrue("case $i", runCatching { parsed(o) }.isFailure)
        }
    }

    @Test fun parsesScreenTimeAlwaysAllowedAndBonus() {
        val o = v3()
        o.getJSONObject("apps").put("screen", JSONObject().put("mon", 120).put("sat", 180))
            .put("always_allowed", org.json.JSONArray().put("com.example.school"))
            .put("bonus", JSONObject("""{"day":"2026-10-24","total":30,"apps":[{"package":"com.roblox.client","minutes":15}]}"""))
        val a = parsed(o).apps!!
        assertEquals(mapOf(java.time.DayOfWeek.MONDAY to 120, java.time.DayOfWeek.SATURDAY to 180), a.screenMinutes)
        assertEquals(setOf("com.example.school"), a.alwaysAllowed)
        assertEquals(java.time.LocalDate.parse("2026-10-24"), a.bonusDay)
        assertEquals(30, a.bonusTotalMinutes)
        assertEquals(mapOf("com.roblox.client" to 15), a.bonusAppMinutes)
    }

    @Test fun invalidScreenTimeOrBonusIsRejectedNotRepaired() {
        val bad = listOf<(JSONObject) -> Unit>(
            { it.put("screen", JSONObject().put("monday", 60)) },
            { it.put("screen", JSONObject().put("xyz", 60)) },
            { it.put("screen", JSONObject().put("mon", 1441)) },
            { it.put("always_allowed", org.json.JSONArray().put("../x")) },
            { it.put("bonus", JSONObject().put("day", "not-a-date").put("total", 5)) },
            { it.put("bonus", JSONObject().put("day", "2026-10-24").put("total", -5)) },
            { it.put("bonus", JSONObject("""{"day":"2026-10-24","apps":[{"package":"com.a.b","minutes":0}]}""")) },
        )
        bad.forEachIndexed { i, mutate ->
            val o = v3()
            mutate(o.getJSONObject("apps"))
            assertTrue("case $i", runCatching { parsed(o) }.isFailure)
        }
    }

    @Test fun validatesPackageNames() {
        for (p in listOf("com.roblox.client", "a.b", "com.example.app_2")) assertTrue(p, validPackage(p))
        for (p in listOf("android", "com..x", ".com.x", "com.x.", "1com.x", "com.1x", "com/x", "c".repeat(256) + ".x")) assertFalse(p, validPackage(p))
    }

    @Test fun normalizesValidAsciiHostname() {
        val o = payload()
        o.getJSONObject("dns").put("hostname", "ALEX.DNS.EXAMPLE.NET")
        assertEquals("alex.dns.example.net", parsed(o).dns!!.hostname)
    }
}
