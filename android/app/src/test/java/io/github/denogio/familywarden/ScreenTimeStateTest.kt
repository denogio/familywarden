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

    @Test fun historyIsBoundedLocalAndNeverCarriedAcrossPolicyOrZoneChanges() {
        val history = mapOf(now.toLocalDate().toString() to 123L,
            now.toLocalDate().minusDays(29).toString() to 456L,
            now.toLocalDate().minusDays(30).toString() to 789L,
            now.toLocalDate().plusDays(1).toString() to 100L)
        assertEquals(2, map(r = report.copy(historyTotals = history)).historyTotals.size)
        assertTrue(map(r = report.copy(historyTotals = history, revision = "old")).historyTotals.isEmpty())
        assertTrue(map(r = report.copy(historyTotals = history, zone = "UTC")).historyTotals.isEmpty())
    }
}
