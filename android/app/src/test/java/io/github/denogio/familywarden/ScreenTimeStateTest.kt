package io.github.denogio.familywarden

import io.github.denogio.familywarden.ui.*
import org.junit.Assert.*
import org.junit.Test
import java.time.*

class ScreenTimeStateTest {
    private val now = ZonedDateTime.parse("2026-10-23T19:00:00+02:00[Europe/Copenhagen]")
    private val pkg = "com.example.video"
    private val policy = AppsPolicy("0123456789abcdef", false, mapOf(pkg to 30), emptySet(), setOf(pkg),
        emptyList(), mapOf(DayOfWeek.FRIDAY to 120))
    private val report = AppUsageReport(policy.rev, now.zone.id, now.toLocalDate(), 1000, true,
        mapOf(UsageCounter.TOTAL to 82 * 60_000L, pkg to 12 * 60_000L), mapOf(pkg to "Video"))
    private fun map(p: AppsPolicy = policy, r: AppUsageReport? = report, at: ZonedDateTime = now,
        elapsed: Long = 1000, access: Boolean = true) = screenTimeState(p, r, at, elapsed, access)

    @Test fun rebootDebitReducesRemainingButNeverMeasuredLabels() {
        val unlimited = "com.example.allowed"
        val p = policy.copy(alwaysAllowed = setOf(unlimited))
        val measured = mapOf(UsageCounter.TOTAL to 60_000L, pkg to 60_000L, unlimited to 60_000L)
        val s = map(p = p, r = report.copy(used = measured,
            budgetDebits = mapOf(UsageCounter.TOTAL to 15_000L, pkg to 15_000L, unlimited to 15_000L)))
        assertEquals(60_000L, s.usedMillis)
        assertEquals(120 * 60_000L - 75_000L, s.remainingMillis)
        val limited = s.apps.single { it.pkg == pkg }
        assertEquals(60_000L, limited.usedMillis)
        assertEquals(30 * 60_000L - 75_000L, limited.remainingMillis)
        val allowed = s.apps.single { it.pkg == unlimited }
        assertEquals(60_000L, allowed.usedMillis)
        assertNull(allowed.remainingMillis)
        assertEquals(TodayKind.TimesUp, map(r = report.copy(used = mapOf("total" to 120 * 60_000L - 15_000L),
            budgetDebits = mapOf("total" to 15_000L))).kind)
    }

    @Test fun allowedShowsTheUnionTotalNotTheSumOfAppRows() {
        val s = map()
        assertEquals(TodayKind.Allowed, s.kind)
        assertEquals(38 * 60_000L, s.remainingMillis)
        assertEquals("Video", s.apps.single().label)
        assertEquals(12 * 60_000L, s.apps.single().usedMillis)
        assertEquals(now.toLocalDate().plusDays(1).atStartOfDay(now.zone), s.resetsAt)
    }

    @Test fun missingUnreliableOrStaleObservationsNeverInventZeroOrRemainingTime() {
        listOf(null, report.copy(reliable = false), report.copy(revision = "other"),
            report.copy(zone = "UTC"), report.copy(day = now.toLocalDate().minusDays(1)),
            report.copy(elapsedMillis = -30_000), report.copy(elapsedMillis = 2000)).forEach { r ->
            val s = map(r = r)
            assertEquals(TodayKind.Unknown, s.kind)
            assertNull(s.usedMillis)
            assertNull(s.remainingMillis)
            assertNull(s.apps.single().usedMillis)
        }
    }

    @Test fun revokedUsageAccessInvalidatesEvenAFreshReliableReport() {
        assertEquals(TodayKind.Unknown, map(access = false).kind)
        assertNull(map(access = false).usedMillis)
    }

    @Test fun warningAndExhaustionUseTodayIncludingItsBonus() {
        val near = report.copy(used = mapOf(UsageCounter.TOTAL to 115 * 60_000L))
        assertEquals(TodayKind.NearLimit, map(r = near).kind)
        val exhausted = near.copy(used = mapOf(UsageCounter.TOTAL to 120 * 60_000L))
        assertEquals(TodayKind.TimesUp, map(r = exhausted).kind)
        assertEquals(0L, map(r = exhausted).remainingMillis)
        val bonus = policy.copy(bonusDay = now.toLocalDate(), bonusTotalMinutes = 30)
        assertEquals(30 * 60_000L, map(p = bonus, r = exhausted).remainingMillis)
        assertEquals(120 * 60_000L, map(p = bonus.copy(bonusDay = now.toLocalDate().minusDays(1))).limitMillis)
    }

    @Test fun downtimeIsKnownWithoutUsageAndSkipsOverlappingBoundaries() {
        val windows = listOf(Window(setOf(DayOfWeek.FRIDAY), LocalTime.of(18, 0), LocalTime.of(22, 0)),
            Window(setOf(DayOfWeek.FRIDAY), LocalTime.of(21, 0), LocalTime.of(23, 0)))
        val s = map(p = policy.copy(downtime = windows), r = null)
        assertEquals(TodayKind.Downtime, s.kind)
        assertEquals(now.withHour(23), s.nextDowntimeChange)
        assertNull(s.usedMillis)
    }

    @Test fun overnightDowntimeAndMidnightRespectRouterZone() {
        val w = Window(setOf(DayOfWeek.SATURDAY), LocalTime.of(22, 0), LocalTime.of(7, 0))
        val beforeDst = ZonedDateTime.parse("2026-10-25T01:00:00+02:00[Europe/Copenhagen]")
        assertEquals(ZonedDateTime.parse("2026-10-25T07:00:00+01:00[Europe/Copenhagen]"),
            nextDowntimeChange(listOf(w), beforeDst))
        val fullDay = Window(setOf(DayOfWeek.FRIDAY), LocalTime.NOON, LocalTime.NOON)
        assertEquals(now.toLocalDate().plusDays(1).atStartOfDay(now.zone), nextDowntimeChange(listOf(fullDay), now))
    }

    @Test fun noDailyLimitIsNotAnUnlimitedAppAndConflictingBlockWins() {
        val s = map(p = policy.copy(screenMinutes = emptyMap(), alwaysAllowed = setOf(pkg)))
        assertNull(s.limitMillis)
        assertNull(s.remainingMillis)
        assertEquals(listOf("Video"), s.alwaysAllowed)
        val conflict = map(p = policy.copy(alwaysAllowed = setOf(pkg), blocked = setOf(pkg)))
        assertTrue(conflict.alwaysAllowed.isEmpty())
        assertTrue(conflict.apps.single().blocked)
        assertFalse(conflict.apps.single().unlimited)
        assertTrue(map(r = report.copy(essential = setOf(pkg))).apps.isEmpty())
    }

    @Test fun appHistoryUsesRecordedDaysAndLabelsWithoutInventingMissingTotals() {
        val day = now.toLocalDate().toString()
        val history = mapOf(day to mapOf(pkg to 12L, "com.example.game" to 45L, "com.example.phone" to 100L),
            now.toLocalDate().minusDays(30).toString() to mapOf(pkg to 1L),
            now.toLocalDate().plusDays(1).toString() to mapOf(pkg to 1L),
            "bad-date" to mapOf(pkg to 1L))
        val observation = report.copy(historyApps = history, essential = setOf("com.example.phone"))
        val s = map(r = observation, elapsed = 100_000)
        assertEquals(listOf(day), s.historyApps.keys.toList())
        assertEquals(listOf(DayApp("com.example.game", "com.example.game", 45), DayApp(pkg, "Video", 12)), s.historyApps[day])
        assertTrue(s.historyTotals.isEmpty()) // Per-app sums are not a union total.
        assertNull(s.usedMillis) // Historical records survive stale live observations.
        assertTrue(map(r = observation.copy(revision = "old")).historyApps.isEmpty())
        assertTrue(map(r = observation.copy(zone = "UTC")).historyApps.isEmpty())
    }

    @Test fun historyIsBoundedLocalAndNeverCarriedAcrossPolicyOrZoneChanges() {
        val history = mapOf(now.toLocalDate().toString() to 123L,
            now.toLocalDate().minusDays(29).toString() to 456L,
            now.toLocalDate().minusDays(30).toString() to 789L,
            now.toLocalDate().plusDays(1).toString() to 100L)
        assertEquals(2, map(r = report.copy(historyTotals = history)).historyTotals.size)
        assertTrue(map(r = report.copy(historyTotals = history, revision = "old")).historyTotals.isEmpty())
        assertTrue(map(r = report.copy(historyTotals = history, zone = "UTC")).historyTotals.isEmpty())
    }

    @Test fun runningLowIsOpenMeasuredLimitedAppsWithinTheExistingWarning() {
        fun row(remaining: Long, blocked: Boolean = false, unlimited: Boolean = false, used: Long? = 30 * 60_000L - remaining) =
            AppTimeRow(pkg, "Video", used, 30 * 60_000L, blocked, unlimited)
        assertEquals(5 * 60_000L, AppControl.WARN_BEFORE_MS)
        assertTrue(row(5 * 60_000L).runningLow)
        assertFalse(row(5 * 60_000L + 1_000L).runningLow)
        assertFalse(row(0).runningLow) // time is up: paused, not "running low"
        assertTrue(row(1).runningLow)
        assertFalse(row(60_000L, blocked = true).runningLow)
        assertFalse(row(60_000L, used = null).runningLow) // never guessed without measurement
        assertFalse(AppTimeRow(pkg, "Video", 29 * 60_000L, null, false, true).runningLow)
        // The RB1 reboot debit counts towards the budget, not towards measured use.
        val debited = AppTimeRow(pkg, "Video", 25 * 60_000L - 15_000L, 30 * 60_000L, false, false, 15_000L)
        assertEquals(5 * 60_000L, debited.remainingMillis)
        assertTrue(debited.runningLow)
    }

    @Test fun followingDowntimeChangeIsTheEndOfTheNextPause() {
        val p = policy.copy(downtime = listOf(Window(DayOfWeek.entries.toSet(), LocalTime.of(21, 30), LocalTime.of(6, 30))))
        val open = map(p = p)
        assertEquals(now.withHour(21).withMinute(30), open.nextDowntimeChange)
        assertEquals(now.plusDays(1).withHour(6).withMinute(30), open.followingDowntimeChange)
        val resting = map(p = p, at = now.withHour(22), r = report)
        assertEquals(TodayKind.Downtime, resting.kind)
        assertEquals(now.plusDays(1).withHour(6).withMinute(30), resting.nextDowntimeChange)
        assertEquals(now.plusDays(1).withHour(21).withMinute(30), resting.followingDowntimeChange)
        assertNull(map().followingDowntimeChange)
    }
}
