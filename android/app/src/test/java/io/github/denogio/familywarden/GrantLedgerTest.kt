package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

class GrantLedgerTest {
    private val at = Instant.parse("2026-10-24T12:00:00Z").toEpochMilli()
    private val scope = "router-fingerprint|fw-alex|alex"
    private val video = "com.example.video"
    private val chat = "com.example.chat"
    private val zone = ZoneId.of("UTC")
    private fun grant(target: String = video, id: String = "a".repeat(32)) =
        AppGrant(id, "fw-alex", target, 1, at / 1000, at / 1000 + 7200)
    private fun apps(grants: List<AppGrant>, limit: Int? = 1, daily: Int? = null, blocked: Boolean = false,
        downtime: Boolean = false): String = JSONObject().apply {
        put("rev", "0123456789abcdef"); put("suspend_new", false)
        put("limits", JSONArray().apply { if (limit != null) put(JSONObject().put("package", video).put("minutes", limit)) })
        put("blocked", JSONArray(if (blocked) listOf(video) else emptyList<String>()))
        put("approved", JSONArray(if (blocked) listOf(chat) else listOf(video, chat)))
        if (blocked) put("limits", JSONArray())
        put("downtime", JSONArray().apply { if (downtime) put(JSONObject().put("days", "mon tue wed thu fri sat sun").put("start", "00:00").put("stop", "00:00")) })
        put("screen", JSONObject().apply { if (daily != null) for (day in listOf("mon", "tue", "wed", "thu", "fri", "sat", "sun")) put(day, daily) })
        put("grants", JSONArray(grants.map { g -> JSONObject().put("grant_id", g.id).put("device_id", g.deviceId)
            .put("target", g.target).put("minutes", g.minutes).put("not_before", g.notBefore).put("not_after", g.notAfter) }))
    }.toString()
    private fun context(raw: String, wall: Long = at, server: Long = wall, reliable: Boolean = true) =
        GrantLedger.Context(scope, "fw-alex", raw, zone, wall, server, emptySet(), setOf(video, chat), reliable)
    private fun rejected(block: () -> Unit) {
        try { block(); fail("unsafe ledger state accepted") } catch (_: Exception) { }
    }

    @Test fun spendsOnlyThePartAfterTheOrdinaryLimit() {
        val book = GrantLedger()
        book.configure(context(apps(listOf(grant()))))
        book.charge(at, at + 90_000, setOf(video), mapOf(video to 30_000L))
        assertEquals(60_000L, book.spent(scope, grant().id))
        assertEquals(0L, book.available(scope, at + 90_000)[video])
    }

    @Test fun unusedStandingBudgetDoesNotBurnTheGrant() {
        val book = GrantLedger()
        book.configure(context(apps(listOf(grant()), limit = 10)))
        book.charge(at, at + 30_000, setOf(video), emptyMap())
        assertEquals(0L, book.spent(scope, grant().id))
        assertEquals(60_000L, book.available(scope, at + 30_000)[video])
    }

    @Test fun totalGrantCountsThePipUnionNotBothApps() {
        val g = grant("total")
        val book = GrantLedger()
        book.configure(context(apps(listOf(g), limit = null, daily = 1)))
        book.charge(at, at + 30_000, setOf(video, chat), mapOf(UsageCounter.TOTAL to 60_000L))
        assertEquals(30_000L, book.spent(scope, g.id))
    }

    @Test fun totalGrantCannotBuyMoreTimeForAnExhaustedApp() {
        val g = grant("total")
        val book = GrantLedger()
        book.configure(context(apps(listOf(g), limit = 1, daily = 1)))
        book.charge(at, at + 30_000, setOf(video), mapOf(video to 60_000L, UsageCounter.TOTAL to 60_000L))
        assertEquals(0L, book.spent(scope, g.id))
    }

    @Test fun explicitBlocksAndSafetyExclusionsNeverSpendGrants() {
        val g = grant()
        val book = GrantLedger()
        book.configure(context(apps(listOf(g), blocked = true, downtime = true)))
        book.charge(at, at + 30_000, setOf(video), emptyMap())
        assertEquals(0L, book.spent(scope, g.id))
        book.configure(context(apps(listOf(g), downtime = true), wall = at + 30_000).copy(excluded = setOf(video)))
        book.charge(at + 30_000, at + 60_000, setOf(video), emptyMap())
        assertEquals(0L, book.spent(scope, g.id))
    }

    @Test fun grantsBypassDowntimeButExpireTwoHoursAfterApproval() {
        val g = grant()
        val book = GrantLedger()
        book.configure(context(apps(listOf(g), downtime = true), wall = at + 7_190_000))
        book.charge(at + 7_190_000, at + 7_220_000, setOf(video), emptyMap())
        assertEquals(10_000L, book.spent(scope, g.id))
        assertTrue(book.available(scope, at + 7_220_000).isEmpty())
    }

    @Test fun checkpointRestartAndDuplicateDeliveryNeverRefillAGrant() {
        val g = grant()
        val raw = apps(listOf(g))
        val book = GrantLedger()
        book.configure(context(raw))
        book.charge(at, at + 30_000, setOf(video), mapOf(video to 60_000L))
        val restored = GrantLedger.decode(JSONObject(book.encode().toString()))
        restored.configure(context(raw, at + 30_000))
        assertEquals(30_000L, restored.available(scope, at + 30_000)[video])
        restored.charge(at + 30_000, at + 90_000, setOf(video), mapOf(video to 90_000L))
        assertEquals(60_000L, restored.spent(scope, g.id))
        rejected { restored.configure(context(apps(listOf(g.copy(minutes = 2))), at + 90_000)) }
    }

    @Test fun clockRollbackAndPruningDoNotReviveExpiredAuthority() {
        val g = grant()
        val book = GrantLedger()
        book.configure(context(apps(listOf(g))))
        book.configure(context(apps(emptyList()), at + 100_000_000))
        val restored = GrantLedger.decode(book.encode())
        assertFalse(restored.configure(context(apps(listOf(g)), at + 1_000)))
        assertTrue(restored.available(scope, at + 1_000).isEmpty())
    }

    @Test fun unknownGapIsNotZeroAndOnlyItsPossibleOverlapIsCharged() {
        val g = grant()
        val raw = apps(listOf(g))
        val book = GrantLedger()
        book.configure(context(raw))
        book.gap()
        assertTrue(book.available(scope, at + 10_000).isEmpty())
        val restored = GrantLedger.decode(book.encode())
        restored.configure(context(raw, at + 10_000))
        assertEquals(10_000L, restored.uncertain(scope, g.id))
        assertEquals(50_000L, restored.available(scope, at + 10_000)[video])
        restored.configure(context(raw, at + 10_000))
        assertEquals(10_000L, restored.uncertain(scope, g.id))
    }

    @Test fun eventCursorAndGrantSpendingRestartFromTheSameAtomicCheckpoint() {
        val g = grant()
        val book = GrantLedger()
        book.configure(context(apps(listOf(g))))
        val counter = UsageCounter(at, true, true, mutableMapOf(video to mutableSetOf("Video")))
        counter.days["2026-10-24"] = mutableMapOf(video to 60_000L)
        counter.onInterval = book::charge
        counter.consume(emptyList(), at + 30_000, zone)
        book.finishInterval(30_000)
        val checkpoint = UsageCheckpoint(counter, zone.id, 30_000, 1, grants = book)
        val restored = UsageCheckpoint.decode(checkpoint.encode())
        restored.counter.onInterval = restored.grants::charge
        restored.counter.consume(emptyList(), at + 60_000, zone)
        restored.grants.finishInterval(30_000)
        assertEquals(at + 60_000, restored.counter.cursor)
        assertEquals(120_000L, restored.counter.used("2026-10-24", video))
        assertEquals(60_000L, restored.grants.spent(scope, g.id))
    }

    @Test fun smallWallClockDriftDoesNotPermanentlyDisableTheMonotonicBudget() {
        val raw = apps(listOf(grant()), limit = 10)
        val book = GrantLedger()
        book.configure(context(raw))
        book.charge(at, at + 15_010, setOf(video), emptyMap())
        book.finishInterval(15_000)
        assertTrue(book.configure(context(raw, wall = at + 15_010, server = at + 15_000)))
        assertEquals(60_000L, book.available(scope, at + 15_000)[video])
        book.charge(at + 15_010, at + 30_030, setOf(video), emptyMap())
        book.finishInterval(15_000)
        assertTrue(book.configure(context(raw, wall = at + 30_030, server = at + 30_000)))
    }

    @Test fun earliestExpiryAllocationIsStableAcrossOverlappingGrants() {
        val a = grant()
        val b = grant(id = "b".repeat(32))
        val book = GrantLedger()
        book.configure(context(apps(listOf(b, a))))
        book.charge(at, at + 90_000, setOf(video), mapOf(video to 60_000L))
        assertEquals(60_000L, book.spent(scope, a.id))
        assertEquals(30_000L, book.spent(scope, b.id))
    }
}
