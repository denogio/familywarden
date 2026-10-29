package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class AppRuleClientTest {
    private val router = PhoneKey.generate()
    private val phone = PhoneKey.generate()
    private val now = 1800000000L
    private fun operation(draft: RuleDraft = RuleDraft("limit", "0")): RuleOperation {
        val raw = AppRuleClient.query("fw-parent", "alex", "com.example.video", "0123456789abcdef", draft, now, "a".repeat(64))
        return RuleOperation(raw, phone.sign("familywarden-rule-v1\n".toByteArray() + raw.toByteArray()), router.publicKey, phone.publicKey)
    }
    private fun reply(op: RuleOperation, status: Boolean, state: String? = null, error: String? = null): ByteArray {
        val o = JSONObject().put("v", 1).put("type", if (status) "rule_status_reply" else "rule_reply")
            .put("device", "fw-parent").put("message_sha256", RequestProtocol.digest(op.raw.toByteArray())).put("server_time", now)
        if (error != null) o.put("error", error)
        else o.put("state", state).put("profile_id", "alex").put("package", "com.example.video").put("apps_rev", "fedcba9876543210")
        val bytes = o.toString().toByteArray()
        return (router.sign(bytes) + "\n").toByteArray() + bytes
    }
    @Test fun exactSignedOperationSurvivesRestartIndependentlyOfDraft() {
        val op = operation()
        val restored = RuleOperation.decode(op.encode())
        assertEquals(op, restored)
        assertArrayEquals(op.body(), restored.body())
        assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), op.signature,
            "familywarden-rule-v1\n".toByteArray() + restored.raw.toByteArray()))
        assertEquals(0, JSONObject(restored.raw).getInt("minutes"))
        assertEquals("limit", JSONObject(restored.raw).getString("mode"))
        assertEquals(now + 600, JSONObject(restored.raw).getLong("expires"))
        assertTrue(restored.unresolved)
        assertEquals(RuleDraft("blocked").quota(), 0)
        for (n in listOf("-1", "1441", "", "1.5")) try { operation(RuleDraft("limit", n)); fail(n) } catch (_: IllegalArgumentException) { }
    }
    @Test fun statusNeverInventsAReceiptOrErasesAnUncertainCommitOnRevocation() {
        val op = operation()
        for (code in listOf("not_parent", "configuration_required", "unavailable", "outcome_unknown")) {
            assertEquals("unknown", AppRuleClient.parse(reply(op, true, error = code), op, true, now).state)
        }
        assertEquals("refused", AppRuleClient.parse(reply(op, false, error = "configuration_required"), op, false, now).state)
        assertTrue(AppRuleClient.parse(reply(op, false, error = "outcome_unknown"), op, false, now).unresolved)
        assertEquals("saved", AppRuleClient.parse(reply(op, false, "saved"), op, false, now).state)
        assertEquals("matched", AppRuleClient.parse(reply(op, true, "matched"), op, true, now).state)
        assertTrue(AppRuleClient.parse(reply(op, true, "retryable"), op, true, now).unresolved)
        assertFalse(AppRuleClient.parse(reply(op, true, "conflict"), op, true, now).unresolved)
        try { AppRuleClient.parse(reply(op, true, "saved"), op, true, now); fail("status is not a receipt") } catch (_: IllegalArgumentException) { }
    }
    @Test fun responseMustBeBoundToOriginalBytesRouterAndEndpoint() {
        val op = operation()
        val changed = operation(RuleDraft("blocked"))
        for (bad in listOf(changed, op.copy(router = phone.publicKey))) {
            try { AppRuleClient.parse(reply(op, true, "matched"), bad, true, now); fail() } catch (_: Exception) { }
        }
        try { AppRuleClient.parse(reply(op, false, "saved"), op, true, now); fail() } catch (_: Exception) { }
    }
    @Test fun retryAlwaysChecksStatusAndNeverCreatesOrWidensAnOperation() {
        val op = RuleOperation.decode(operation().encode())
        for (state in listOf("unknown", "matched", "conflict", "expired", "retryable")) {
            val calls = mutableListOf<Boolean>()
            val result = AppRuleClient.reconcile(op, true) { sent, status ->
                assertEquals(op, sent)
                assertArrayEquals(op.body(), sent.body())
                calls += status
                op.copy(state = if (status) state else "saved")
            }
            assertEquals(if (state == "retryable") listOf(true, false) else listOf(true), calls)
            assertEquals(if (state == "retryable") "saved" else state, result.state)
        }
    }

    @Test fun preflightFailuresDoNotInventARetainedOperation() {
        val previous = operation().copy(state = "saved")
        assertTrue(ruleSaveNotSent(null, null))
        assertTrue(ruleSaveNotSent(previous, previous))
        assertTrue(ruleSaveNotSent(previous, null))
        assertFalse(ruleSaveNotSent(previous, operation(RuleDraft("blocked"))))
        for (code in listOf("replay", "operation_capacity")) {
            val op = operation()
            assertEquals("refused", AppRuleClient.parse(reply(op, false, error = code), op, false, now).state)
            assertEquals("unknown", AppRuleClient.parse(reply(op, true, error = code), op, true, now).state)
        }
    }

}
