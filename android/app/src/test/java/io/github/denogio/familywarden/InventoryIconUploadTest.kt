package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.util.Base64

class InventoryIconUploadTest {
    @Test fun all200MaximumImagesFitIndependentBoundedMessagesWithoutLosingMetadata() {
        val icon = Base64.getEncoder().encodeToString(ByteArray(InventoryIconUpload.MAX_ICON_BYTES))
        val items = (0 until 200).map { InventoryUpload.Item("com.example.app${it.toString().padStart(3, '0')}", "App", false, false, icon) }
        val batches = InventoryIconUpload.batches("fw-alex", "alex", 253402300799L, 253402300799L, items)
        assertEquals(200, batches.size)
        val packages = batches.flatMap { raw ->
            assertTrue(raw.toByteArray().size <= InventoryUpload.MAX_BYTES)
            val o = JSONObject(raw)
            assertEquals("inventory_icons", o.getString("type"))
            assertEquals(253402300799L, o.getLong("inventory_seq"))
            val icons = o.getJSONArray("icons")
            assertTrue(icons.length() in 1..32)
            (0 until icons.length()).map { icons.getJSONObject(it).getString("package") }
        }
        assertEquals(items.map { it.pkg }, packages)
        assertEquals(200, JSONObject(InventoryUpload.report("fw-alex", "alex", 1, items, false)).getJSONArray("apps").length())
        val small = items.map { it.copy(icon = Base64.getEncoder().encodeToString(ByteArray(100))) }
        assertEquals(7, InventoryIconUpload.batches("fw-alex", "alex", 1, 1, small).size)
    }

    @Test fun deterministicPackingIncludesOnlyPresentValidSizeImagesAndFitsSignedEnvelope() {
        val items = listOf(
            InventoryUpload.Item("com.example.z", "Z", false, false, "AA=="),
            InventoryUpload.Item("com.example.a", "A", false, false, "AA=="),
            InventoryUpload.Item("invalid", "Invalid", false, false, "AA=="),
            InventoryUpload.Item("com.example.missing", "Missing", false, false),
            InventoryUpload.Item("com.example.invalid", "Invalid", false, false, "bad!"),
            InventoryUpload.Item("com.example.huge", "Huge", false, false, Base64.getEncoder().encodeToString(ByteArray(InventoryIconUpload.MAX_ICON_BYTES + 1)))
        )
        val batches = InventoryIconUpload.batches("fw-alex", "alex", 1, 1, items)
        assertEquals(batches, InventoryIconUpload.batches("fw-alex", "alex", 1, 1, items.reversed()))
        val apps = JSONObject(batches.single()).getJSONArray("icons")
        assertEquals(listOf("com.example.a", "com.example.z"), (0 until apps.length()).map { apps.getJSONObject(it).getString("package") })
        RequestUpload("inventory_icons", batches.single(), PhoneKey.generate().publicKey, "fw-alex")
    }
    private fun preferences(values: MutableMap<String, Any>): android.content.SharedPreferences {
        lateinit var editor: android.content.SharedPreferences.Editor
        editor = java.lang.reflect.Proxy.newProxyInstance(javaClass.classLoader,
            arrayOf(android.content.SharedPreferences.Editor::class.java)) { _, method, args ->
            when (method.name) {
                "commit" -> true
                "remove" -> { values.remove(args!![0]); editor }
                "putLong", "putInt", "putString" -> { values[args!![0] as String] = args[1]; editor }
                else -> error(method.name)
            }
        } as android.content.SharedPreferences.Editor
        return java.lang.reflect.Proxy.newProxyInstance(javaClass.classLoader,
            arrayOf(android.content.SharedPreferences::class.java)) { _, method, args ->
            when (method.name) {
                "edit" -> editor
                "getString", "getLong", "getInt" -> values[args!![0]] ?: args[1]
                else -> error(method.name)
            }
        } as android.content.SharedPreferences
    }

    @Test fun restartResumesAllBatchesAndStopsWithoutChurnAndNetworkErrorsNeverDowngrade() {
        val image = Base64.getEncoder().encodeToString(ByteArray(InventoryIconUpload.MAX_ICON_BYTES))
        val items = (0 until 200).map { InventoryUpload.Item("com.example.app${it.toString().padStart(3, '0')}", "App", false, false, image) }
        val values = mutableMapOf<String, Any>("inventory_seq" to 1L)
        var uploads = 0
        val seen = mutableSetOf<String>()
        fun sync(now: Long = 1L, fail: Boolean = false) {
            InventoryIconUpload.send(preferences(values), "fictional-scope", now, items, "fw-alex", "alex") { raw ->
                uploads++
                if (fail) throw IllegalStateException("unsigned old-router response or network failure")
                seen.add(JSONObject(raw).getJSONArray("icons").getJSONObject(0).getString("package"))
                UploadReply("stored", null)
            }
        }
        sync(fail = true)
        assertEquals(0, values["icons_next"])
        sync()
        assertEquals(1, uploads)
        repeat(7) { sync(1 + 6 * 3_600_000L) }
        assertEquals(200, seen.size)
        assertTrue("com.example.app199" in seen)
        val complete = uploads
        sync(2 + 6 * 3_600_000L)
        assertEquals(complete, uploads)
        sync(1 + 12 * 3_600_000L)
        assertEquals(complete + 32, uploads) // periodic repair, even with identical digest
        assertFalse(values.containsKey("legacy_scope"))
    }

}
