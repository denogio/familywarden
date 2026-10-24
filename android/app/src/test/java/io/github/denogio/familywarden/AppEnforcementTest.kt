package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test
import java.time.DayOfWeek
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

class AppEnforcementTest {
    private val pkg = "com.example.game"
    private val zone = ZoneId.of("Europe/Copenhagen")
    private val now = ZonedDateTime.parse("2026-10-21T22:00:00+02:00[Europe/Copenhagen]") // a Wednesday
    private fun decide(p: LocalAppPolicy, used: Long = 0, total: Long = 0, essential: Boolean = false,
        user: Boolean = true, reliable: Boolean = true, app: String = pkg, at: ZonedDateTime = now) =
        appAction(p, app, essential, user, used, total, at, reliable)
    private val bedtime = Window(setOf(DayOfWeek.WEDNESDAY), LocalTime.of(21, 0), LocalTime.of(7, 0))

    @Test fun essentialsCannotBeBlockedByAnyLocalRule() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 0), blocked = setOf(pkg), suspendNewUserApps = true,
            downtime = listOf(bedtime), dailyLimitMillis = 0)
        assertEquals(AppAction.ALLOW, decide(p, essential = true, reliable = false))
    }

    @Test fun appLimitBoundaryIsExactAndOnlyAffectsThatApp() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 10000))
        assertEquals(AppAction.ALLOW, decide(p, 9999))
        assertEquals(AppAction.APP_LIMIT, decide(p, 10000))
        assertEquals(AppAction.ALLOW, decide(p, 99999, app = "com.example.other"))
    }

    @Test fun missingUsageHoldsButKnownExhaustionAndBlocksStillApply() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 10000))
        assertEquals(AppAction.HOLD, decide(p, 9999, reliable = false))
        assertEquals(AppAction.APP_LIMIT, decide(p, 10000, reliable = false))
        assertEquals(AppAction.BLOCKED, decide(p.copy(blocked = setOf(pkg)), reliable = false))
        // Without any limit there is nothing to hold.
        assertEquals(AppAction.ALLOW, decide(LocalAppPolicy(zone), reliable = false))
    }

    @Test fun unapprovedUserAppsAreBlockedButSystemAppsAreNotBlanketSuspended() {
        val p = LocalAppPolicy(zone, suspendNewUserApps = true)
        assertEquals(AppAction.NOT_APPROVED, decide(p))
        assertEquals(AppAction.ALLOW, decide(p, user = false))
        assertEquals(AppAction.ALLOW, decide(p.copy(approved = setOf(pkg))))
    }

    @Test fun downtimePausesEveryOrdinaryAppLikeFamilyLink() {
        val p = LocalAppPolicy(zone, downtime = listOf(bedtime))
        assertEquals(AppAction.DOWNTIME, decide(p, app = "com.example.unconfigured"))
        assertEquals(AppAction.DOWNTIME, decide(p, user = false, app = "com.android.chrome"))
        // Router-timezone and start-day overnight semantics.
        assertEquals(AppAction.DOWNTIME, decide(p, at = now.withZoneSameInstant(ZoneId.of("America/New_York"))))
        assertEquals(AppAction.DOWNTIME, decide(p, at = now.plusHours(8)))
        assertEquals(AppAction.ALLOW, decide(p, at = now.plusHours(9)))
    }

    @Test fun dailyLimitPausesEveryOrdinaryAppButNotAlwaysAllowedOnes() {
        val p = LocalAppPolicy(zone, dailyLimitMillis = 60_000, alwaysAllowed = setOf("com.example.school"))
        assertEquals(AppAction.ALLOW, decide(p, total = 59_999))
        assertEquals(AppAction.DAILY_LIMIT, decide(p, total = 60_000))
        assertEquals(AppAction.DAILY_LIMIT, decide(p, total = 60_000, app = "com.example.unconfigured"))
        assertEquals(AppAction.ALLOW, decide(p, total = 60_000, app = "com.example.school"))
        assertEquals(AppAction.ALLOW, decide(p.copy(downtime = listOf(bedtime)), app = "com.example.school"))
        assertEquals(AppAction.HOLD, decide(p, total = 1, reliable = false))
    }

    @Test fun blockedWinsOverAlwaysAllowed() {
        val p = LocalAppPolicy(zone, blocked = setOf(pkg), alwaysAllowed = setOf(pkg))
        assertEquals(AppAction.BLOCKED, decide(p))
    }

    @Test fun defaultPolicyAndRemovedRulesDoNotInventBlocks() {
        assertEquals(AppAction.ALLOW, decide(LocalAppPolicy(zone), reliable = false))
    }

    @Test fun invalidLocalBudgetsAreRejected() {
        for (limit in listOf(-1L, 2 * 86_400_001L))
            assertTrue(runCatching { LocalAppPolicy(zone, mapOf(pkg to limit)) }.isFailure)
        assertTrue(runCatching { LocalAppPolicy(zone, dailyLimitMillis = -1) }.isFailure)
    }

    private val apps = AppsPolicy("0123456789abcdef", false, mapOf(pkg to 30), emptySet(), setOf(pkg), emptyList(),
        screenMinutes = mapOf(DayOfWeek.WEDNESDAY to 60, DayOfWeek.SATURDAY to 180),
        bonusDay = LocalDate.parse("2026-10-21"), bonusTotalMinutes = 15, bonusAppMinutes = mapOf(pkg to 10))

    @Test fun todaysLimitsComeFromTheWeekdayAndIncludeTodaysBonusOnly() {
        val wed = apps.toLocal(zone, LocalDate.parse("2026-10-21"))
        assertEquals(75 * 60_000L, wed.dailyLimitMillis)           // 60 + 15 bonus
        assertEquals(40 * 60_000L, wed.limitsMillis[pkg])            // 30 + 10 bonus
        val sat = apps.toLocal(zone, LocalDate.parse("2026-10-24"))
        assertEquals(180 * 60_000L, sat.dailyLimitMillis)          // bonus was for Wednesday
        assertEquals(30 * 60_000L, sat.limitsMillis[pkg])
        assertNull(apps.toLocal(zone, LocalDate.parse("2026-10-22")).dailyLimitMillis) // no limit on Thursdays
    }

    @Test fun warnOnlyInTheLastFiveMinutesAndNeverAfterTheLimit() {
        val p = LocalAppPolicy(zone, mapOf(pkg to 30 * 60_000L), dailyLimitMillis = 60 * 60_000L)
        assertEquals(emptySet<String>(), AppControl.almostUsedUp(p, mapOf(pkg to 24 * 60_000L)))
        assertEquals(setOf(pkg), AppControl.almostUsedUp(p, mapOf(pkg to 25 * 60_000L)))
        assertEquals(setOf(UsageCounter.TOTAL), AppControl.almostUsedUp(p, mapOf(UsageCounter.TOTAL to 56 * 60_000L)))
        assertEquals(emptySet<String>(), AppControl.almostUsedUp(p, mapOf(pkg to 30 * 60_000L, UsageCounter.TOTAL to 60 * 60_000L)))
    }

    @Test fun acknowledgeOnlyWhatIsFullyInForce() {
        assertTrue(AppControl.acknowledges(emptySet(), usageReliable = true))
        assertFalse(AppControl.acknowledges(setOf("com.example.game"), usageReliable = true))
        assertFalse(AppControl.acknowledges(emptySet(), usageReliable = false)) // limits only held
    }
}
