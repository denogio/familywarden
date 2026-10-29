package io.github.denogio.familywarden

import android.content.ContextWrapper
import android.content.SharedPreferences
import java.lang.reflect.Proxy
import org.junit.Assert.*
import org.junit.Test

class StoreRuleOperationTest {
    /** Exercise the real Store.unpair path without an emulator or new test runtime. */
    private fun store(): Store {
        val values = mutableMapOf<String, Any?>()
        lateinit var editor: SharedPreferences.Editor
        editor = Proxy.newProxyInstance(SharedPreferences.Editor::class.java.classLoader,
            arrayOf(SharedPreferences.Editor::class.java)) { _, method, args ->
            when (method.name) {
                "remove" -> { values.remove(args!![0]); editor }
                "putString" -> { values[args!![0] as String] = args[1]; editor }
                "commit" -> true
                "apply" -> null
                else -> error("unexpected editor call: ${method.name}")
            }
        } as SharedPreferences.Editor
        val prefs = Proxy.newProxyInstance(SharedPreferences::class.java.classLoader,
            arrayOf(SharedPreferences::class.java)) { _, method, args ->
            when (method.name) {
                "edit" -> editor
                "getString" -> values[args!![0]] ?: args[1]
                else -> error("unexpected preferences call: ${method.name}")
            }
        } as SharedPreferences
        return Store(object : ContextWrapper(null) {
            override fun getSharedPreferences(name: String, mode: Int): SharedPreferences = prefs
        })
    }

    @Test fun explicitUnpairClearsOldHouseholdOperationButOrdinaryReadsRetainIt() {
        for (forgetKey in listOf(false, true)) {
            val store = store()
            store.host = "router.example.invalid"
            store.publicKey = PhoneKey.generate().publicKey
            val oldKey = store.phoneKey.publicKey
            val raw = AppRuleClient.query(store.deviceId, "alex", "com.example.video", "0123456789abcdef",
                RuleDraft("blocked"), 1800000000L, "a".repeat(64))
            val op = RuleOperation(raw, store.phoneKey.sign("familywarden-rule-v1\n".toByteArray() + raw.toByteArray()),
                requireNotNull(store.publicKey), oldKey)
            store.ruleOperation = op
            assertEquals(op, store.ruleOperation)
            assertTrue(requireNotNull(store.ruleOperation).unresolved)
            store.unpair(forgetKey)
            assertNull(store.ruleOperation)
            assertNull(store.publicKey)
            if (forgetKey) assertNotEquals(oldKey, store.phoneKey.publicKey)
            else assertEquals(oldKey, store.phoneKey.publicKey)
            // A different household cannot inherit the old unresolved Save blocker.
            store.publicKey = PhoneKey.generate().publicKey
            assertNull(store.ruleOperation)
        }
    }
}
