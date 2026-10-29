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
}
