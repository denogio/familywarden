package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class InventoryUploadTest {
    private fun item(pkg: String, label: String = "App") = InventoryUpload.Item(pkg, label, false, false)

    @Test fun reportMatchesTheRouterContractSortedAndComplete() {
        val o = JSONObject(InventoryUpload.report("fw-alex", "alex", 3, listOf(item("com.example.b", " B\u0000 "),
            InventoryUpload.Item("com.android.dialer", "Phone", true, true), item("not a package"), item("com.example.b"))))
        // Exactly the fields the Go strict decoder knows.
        assertEquals(setOf("type", "v", "device", "profile", "seq", "installed", "apps"), o.keys().asSequence().toSet())
        val apps = o.getJSONArray("apps")
        assertEquals(2, o.getInt("installed"))
        assertEquals(listOf("com.android.dialer", "com.example.b"), (0 until apps.length()).map { apps.getJSONObject(it).getString("package") })
        assertEquals(setOf("package", "label", "system", "essential"), apps.getJSONObject(0).keys().asSequence().toSet())
        assertTrue(apps.getJSONObject(0).getBoolean("essential"))
        assertEquals("B", apps.getJSONObject(1).getString("label"))
    }

    @Test fun oversizedSnapshotIsHonestlyPartialAndFitsTheTransport() {
        val many = (0 until 700).map { item("com.example.app$it" + "x".repeat(200), "😀".repeat(80)) }
        val raw = InventoryUpload.report("fw-alex", "alex", 1, many)
        val o = JSONObject(raw)
        assertEquals(700, o.getInt("installed"))
        assertTrue(o.getJSONArray("apps").length() in 1 until 700)
        assertTrue(raw.toByteArray().size <= InventoryUpload.MAX_BYTES)
        // Accepted by the signed-upload envelope, unlike an ordinary 16 KiB action.
        RequestUpload("inventory", raw, PhoneKey.generate().publicKey, "fw-alex")
        val count = JSONObject(InventoryUpload.report("fw-alex", "alex", 1, (0 until 600).map { item("com.e.a$it", "") }))
        assertEquals(InventoryUpload.MAX_APPS, count.getJSONArray("apps").length())
        assertEquals(600, count.getInt("installed"))
    }

    @Test fun iconsUseOnlySpareBytesAndPreferUserApps() {
        val icon = java.util.Base64.getEncoder().encodeToString(ByteArray(2048))
        val items = (0 until 20).map { InventoryUpload.Item("com.example.app${it.toString().padStart(2, '0')}", "App", it < 10, false, icon) }
        val with = JSONObject(InventoryUpload.report("fw-alex", "alex", 1, items))
        val without = JSONObject(InventoryUpload.report("fw-alex", "alex", 1, items, false))
        val apps = with.getJSONArray("apps")
        assertEquals(without.getJSONArray("apps").length(), apps.length())
        assertTrue(with.toString().toByteArray().size <= InventoryUpload.MAX_BYTES)
        assertEquals(8, (0 until apps.length()).count { apps.getJSONObject(it).has("icon") })
        assertFalse(apps.getJSONObject(0).has("icon")) // system apps lose to user apps
        assertTrue(apps.getJSONObject(10).has("icon"))
        for (i in 0 until apps.length()) {
            apps.getJSONObject(i).remove("icon")
            assertEquals(without.getJSONArray("apps").getJSONObject(i).toString(), apps.getJSONObject(i).toString())
        }
        val oversized = listOf(InventoryUpload.Item("com.example.game", "Game", false, false,
            java.util.Base64.getEncoder().encodeToString(ByteArray(2049))))
        assertFalse(JSONObject(InventoryUpload.report("fw-alex", "alex", 1, oversized)).getJSONArray("apps").getJSONObject(0).has("icon"))
    }

    @Test fun onlyVerifiedMalformedRetriesWithoutIconsWithFreshSequence() {
        val icon = java.util.Base64.getEncoder().encodeToString(ByteArray(64))
        val items = listOf(InventoryUpload.Item("com.example.game", "Game", false, false, icon))
        var seq = 0L
        var legacy = false
        val uploads = mutableListOf<JSONObject>()
        val reply = InventoryUpload.deliver("fw-alex", "alex", items, true, { ++seq }, { raw ->
            uploads.add(JSONObject(raw))
            if (uploads.size == 1) UploadReply(null, "malformed") else UploadReply("stored", null)
        }, { legacy = true })
        assertEquals("stored", reply.state)
        assertTrue(legacy)
        assertEquals(listOf(1L, 2L), uploads.map { it.getLong("seq") })
        assertTrue(uploads[0].getJSONArray("apps").getJSONObject(0).has("icon"))
        assertFalse(uploads[1].getJSONArray("apps").getJSONObject(0).has("icon"))
        for (error in listOf("unavailable", "wrong_profile")) {
            legacy = false
            InventoryUpload.deliver("fw-alex", "alex", items, true, { ++seq }, { UploadReply(null, error) }, { legacy = true })
            assertFalse(legacy)
        }
        for (failure in listOf(java.io.IOException("network"), IllegalArgumentException("signature"))) {
            legacy = false
            try {
                InventoryUpload.deliver("fw-alex", "alex", items, true, { ++seq }, { throw failure }, { legacy = true })
                fail("failure swallowed")
            } catch (caught: Exception) { assertSame(failure, caught) }
            assertFalse(legacy)
        }
        val until = 100L + InventoryUpload.LEGACY_MS
        assertTrue(InventoryUpload.legacyActive("router", "router", until, 100))
        assertFalse(InventoryUpload.legacyActive("router", "other", until, 100))
        assertFalse(InventoryUpload.legacyActive("router", "router", until, until))
        // The mode is part of digest input: unchanged metadata cannot suppress the TTL probe.
        assertNotEquals(InventoryUpload.report("fw-alex", "alex", 1, items, false),
            InventoryUpload.report("fw-alex", "alex", 1, items, true))
    }
}
