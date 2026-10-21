package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test
import java.time.DayOfWeek
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

class AppEnforcementTest {
    private val pkg = "com.example.game"
    private val zone = ZoneId.of("Europe/Copenhagen")
    private val now = ZonedDateTime.parse("2026-10-21T22:00:00+02:00[Europe/Copenhagen]")
    private fun decide(p: LocalAppPolicy, used: Long = 0, essential: Boolean = false,
        user: Boolean = true, reliable: Boolean = true) = appAction(p, pkg, essential, user, used, now, reliable)

    @Test fun essentialsCannotBeBlockedByAnyLocalRule() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 0), blocked = setOf(pkg), suspendNewUserApps = true)
        assertEquals(AppAction.ALLOW, decide(p, essential = true, reliable = false))
    }

    @Test fun budgetBoundaryIsExactAndOnlyAffectsConfiguredPackages() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 10000))
        assertEquals(AppAction.ALLOW, decide(p, 9999))
        assertEquals(AppAction.BLOCK, decide(p, 10000))
        assertEquals(AppAction.ALLOW, appAction(p, "com.example.other", false, true, 99999, now, true))
    }

    @Test fun missingUsageHoldsPriorDecisionButKnownExhaustionStillBlocks() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 10000))
        assertEquals(AppAction.HOLD, decide(p, 9999, reliable = false))
        assertEquals(AppAction.BLOCK, decide(p, 10000, reliable = false))
        assertEquals(AppAction.BLOCK, decide(p.copy(blocked = setOf(pkg)), reliable = false))
    }

    @Test fun unapprovedUserAppsAreBlockedButSystemAppsAreNotBlanketSuspended() {
        val p = LocalAppPolicy(zone, suspendNewUserApps = true)
        assertEquals(AppAction.BLOCK, decide(p))
        assertEquals(AppAction.ALLOW, decide(p, user = false))
        assertEquals(AppAction.ALLOW, decide(p.copy(approved = setOf(pkg))))
    }

    @Test fun downtimeUsesRouterTimezoneAndStartDayOvernightSemantics() {
        val w = Window(setOf(DayOfWeek.WEDNESDAY), LocalTime.of(21, 0), LocalTime.of(7, 0))
        val p = LocalAppPolicy(zone, approved = setOf(pkg), downtime = listOf(w))
        assertEquals(AppAction.BLOCK, decide(p))
        assertEquals(AppAction.BLOCK, appAction(p, pkg, false, true, 0, now.withZoneSameInstant(ZoneId.of("America/New_York")), true))
        assertEquals(AppAction.BLOCK, appAction(p, pkg, false, true, 0, now.plusHours(8), true))
        assertEquals(AppAction.ALLOW, appAction(p, pkg, false, true, 0, now.plusHours(9), true))
    }

    @Test fun defaultPolicyAndRemovedRulesDoNotInventBlocks() {
        assertEquals(AppAction.ALLOW, decide(LocalAppPolicy(zone), reliable = false))
    }

    @Test fun invalidLocalBudgetsAreRejected() {
        for (limit in listOf(-1L, 86400001L))
            assertTrue(runCatching { LocalAppPolicy(zone, mapOf(pkg to limit)) }.isFailure)
    }
}
