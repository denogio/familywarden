package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test
import java.time.ZoneId
import java.time.ZonedDateTime

class GrantEnforcementTest {
    private val pkg = "com.example.video"
    private val now = ZonedDateTime.parse("2026-10-24T12:00:00Z")
    private val zone = ZoneId.of("UTC")
    private fun action(p: LocalAppPolicy, used: Long = 60_000, total: Long = 60_000,
        reliable: Boolean = true, essential: Boolean = false) = appAction(p, pkg, essential, true, used, total, now, reliable)

    @Test fun appGrantBypassesBothExhaustedBudgetsButNotExplicitBlocks() {
        val p = LocalAppPolicy(zone, limitsMillis = mapOf(pkg to 60_000), dailyLimitMillis = 60_000,
            grantRemaining = mapOf(pkg to 30_000), grantReliable = true)
        assertEquals(AppAction.GRANT, action(p))
        assertEquals(AppAction.BLOCKED, action(p.copy(blocked = setOf(pkg))))
        assertEquals(AppAction.NOT_APPROVED, action(p.copy(suspendNewUserApps = true)))
        assertEquals(AppAction.ALLOW, action(p, essential = true))
    }

    @Test fun totalGrantNeverBypassesAnAppLimit() {
        val p = LocalAppPolicy(zone, limitsMillis = mapOf(pkg to 60_000), dailyLimitMillis = 60_000,
            grantRemaining = mapOf("total" to 30_000), grantReliable = true)
        assertEquals(AppAction.DAILY_LIMIT, action(p))
        assertEquals(AppAction.GRANT, action(p, used = 30_000))
        assertEquals(AppAction.HOLD, action(p, used = 0, total = 0, reliable = false))
    }

    @Test fun unknownBaselineMayUseAnExplicitAppGrantButNeverUnknownGrantAccounting() {
        val p = LocalAppPolicy(zone, limitsMillis = mapOf(pkg to 60_000),
            grantRemaining = mapOf(pkg to 30_000), grantReliable = true)
        assertEquals(AppAction.GRANT, action(p, used = 0, total = 0, reliable = false))
        assertEquals(AppAction.HOLD, action(p.copy(grantReliable = false), used = 0, total = 0, reliable = false))
        assertEquals(AppAction.APP_LIMIT, action(p.copy(grantRemaining = emptyMap())))
    }

    @Test fun networkDelayCannotExtendAWindowOrRewindTheExistingClock() {
        val first = PolicyClock.receipt(1_800_000_000, 0, 5_000, 7, "a".repeat(64))
        assertEquals(1_800_000_005_000, first.now(5_000, 7))
        val faster = PolicyClock.receipt(1_800_000_001, 6_000, 6_100, 7, "b".repeat(64), first)
        assertEquals(1_800_000_006_100, faster.now(6_100, 7))
        val rebooted = PolicyClock.receipt(1_800_000_010, 500, 600, 8, "c".repeat(64), first)
        assertEquals(1_800_000_010_100, rebooted.now(600, 8))
    }

    @Test fun policyClockCannotSurviveRebootByReinterpretingElapsedTime() {
        val clock = PolicyClock(1_800_000_000_000, 50_000, 7, "a".repeat(64))
        assertEquals(1_800_000_060_000, clock.now(110_000, 7))
        assertNull(clock.now(110_000, 8))
        assertNull(clock.now(10_000, 7))
    }
}
