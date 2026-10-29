package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class FamilyAppsClientTest {
    private val router = PhoneKey.generate()
    private val profile = ProfileRef("alex", "Alex")
    private val now = 1800000000L

    private fun app(pkg: String, rule: String = "none", on: List<Int> = listOf(0), essential: List<Int> = emptyList()) =
        JSONObject().put("package", pkg).put("label", "App").put("on", JSONArray(on)).put("system", JSONArray())
            .put("essential", JSONArray(essential)).put("rule", rule).also { if (rule == "limit") it.put("minutes", 0) }

    private fun reply(raw: String, after: String, apps: List<JSONObject>, more: Boolean, snapshot: String = "a".repeat(32)) =
        JSONObject().put("v", 1).put("type", "installed_reply").put("device", "fw-parent").put("server_time", now)
            .put("message_sha256", RequestProtocol.digest(raw.toByteArray())).put("profile_id", "alex").put("after", after)
            .put("snapshot", snapshot).put("apps_rev", "0123456789abcdef").put("suspend_new", true).put("more", more)
            .put("sources", JSONArray().put(JSONObject().put("name", "Alex phone").put("reported", true)
                .put("received", now - 60).put("installed", 3).put("sent", 2))
                .put(JSONObject().put("name", "Alex tablet").put("reported", false)))
            .put("apps", JSONArray(apps))

    private fun signed(o: JSONObject): ByteArray = o.toString().toByteArray().let { (router.sign(it) + "\n").toByteArray() + it }

    @Test fun pagesAreJoinedAndProvenanceRulesAndMissingSourcesStayHonest() {
        val pages = mapOf("" to (listOf(app("com.android.dialer", essential = listOf(0)), app("com.example.a", "limit")) to true),
            "com.example.a" to (listOf(app("com.example.unreported", "blocked", on = emptyList())) to false))
        val asked = mutableListOf<String>()
        val result = FamilyAppsClient.read(router.publicKey, "fw-parent", profile, now) { after, nonce ->
            asked += after
            val raw = FamilyAppsClient.query("fw-parent", "alex", after, now, nonce)
            raw to signed(reply(raw, after, pages.getValue(after).first, pages.getValue(after).second))
        }
        assertEquals(listOf("", "com.example.a"), asked)
        assertEquals(listOf("com.android.dialer", "com.example.a", "com.example.unreported"), result.apps.map { it.pkg })
        assertEquals(AppRule.Limit(0), result.apps[1].rule) // zero quota is not a block
        assertEquals(setOf(0), result.apps[0].essential)
        assertTrue(result.apps[2].on.isEmpty())
        assertTrue(result.sources[0].partial)
        assertFalse(result.sources[1].reported)
        assertNull(result.sources[1].received)
    }

    @Test fun aChangedSnapshotRestartsOnceAndThenFailsInsteadOfMixingLists() {
        var calls = 0
        try {
            FamilyAppsClient.read(router.publicKey, "fw-parent", profile, now) { after, nonce ->
                calls++
                val raw = FamilyAppsClient.query("fw-parent", "alex", after, now, nonce)
                val apps = if (after.isEmpty()) listOf(app("com.example.a")) else listOf(app("com.example.b"))
                raw to signed(reply(raw, after, apps, after.isEmpty(), if (after.isEmpty()) "a".repeat(32) else "b".repeat(32)))
            }
            fail("mixed snapshot accepted")
        } catch (_: IllegalStateException) { }
        assertEquals(4, calls)
    }

    @Test fun unsafeRepliesAndRefusalsNeverBecomeAnEmptyList() {
        val raw = FamilyAppsClient.query("fw-parent", "alex", "", now, "c".repeat(64))
        fun parse(o: JSONObject) = FamilyAppsClient.parse(signed(o), router.publicKey, "fw-parent", profile, "", raw, now)
        for (code in listOf("not_parent", "unavailable", "app_control_off", "request_expired")) {
            try { parse(reply(raw, "", emptyList(), false).put("error", code)); fail() }
            catch (e: InstalledRefused) { assertEquals(code, e.code) }
        }
        val bad = listOf(
            reply(raw, "", listOf(app("com.example.b"), app("com.example.a")), false),     // unsorted
            reply(raw, "", listOf(app("com.example.a", on = listOf(1))), false),            // unreported source
            reply(raw, "", listOf(app("com.example.a", on = listOf(5))), false),            // unknown source
            reply(raw, "", listOf(app("com.example.a", on = emptyList())), false),           // neither reported nor ruled
            reply(raw, "", listOf(app("com.example.a", "maybe")), false),
            reply(raw, "", emptyList(), true),                                               // empty page claiming more
            reply(raw, "", emptyList(), false).put("profile_id", "sam"),
            reply(raw, "", emptyList(), false).put("after", "com.example.z"),
            reply(raw, "", emptyList(), false).put("error", "invented"),
            reply(raw, "", listOf(app("com.example.a").put("essential", JSONArray().put(0)).put("on", JSONArray())), false),
        )
        for (o in bad) try { parse(o); fail("accepted $o") } catch (_: InstalledRefused) { fail() } catch (_: Exception) { }
        try { FamilyAppsClient.query("fw-parent", "alex", "total", now, "c".repeat(64)); fail() } catch (_: IllegalArgumentException) { }
    }
}
