package io.github.denogio.familywarden.ui

import org.junit.Assert.*
import org.junit.Test
import java.time.ZoneId
import java.time.ZonedDateTime

class AdultScheduleTest {
    private val zone = ZoneId.of("Europe/Copenhagen")
    private val now = ZonedDateTime.parse("2026-10-23T16:32:00+02:00[Europe/Copenhagen]")
    private val pause = now.withHour(21).withMinute(30)
    private val morning = now.plusDays(1).withHour(6).withMinute(30)

    private fun time(kind: TodayKind = TodayKind.Allowed, next: ZonedDateTime? = pause, following: ZonedDateTime? = morning) =
        ScreenTimeState(kind, 0, null, next, now.plusDays(1).toLocalDate().atStartOfDay(zone), emptyList(), emptyList(),
            emptyMap(), followingDowntimeChange = following)
    private fun pact(blocked: Boolean, next: ZonedDateTime?, following: ZonedDateTime?) = PactStatus.Bound(blocked,
        next?.toInstant()?.toEpochMilli(), Pact(null, null, null), following?.toInstant()?.toEpochMilli())

    @Test fun appsAndHotspotAreCombinedOnlyAtTheSameInstant() {
        val s = adultSchedule(time(), pact(false, pause, morning), zone)
        assertEquals(pause, s.openUntil)
        assertEquals(listOf(RestLine(Rests.AppsAndHotspot, morning)), s.then)
        assertTrue(s.restingNow.isEmpty())
        assertTrue(s.later.isEmpty())
    }

    @Test fun differentInstantsNameOnlyTheEarlierPause() {
        val s = adultSchedule(time(), pact(false, pause.plusMinutes(30), morning), zone)
        assertEquals(pause, s.openUntil)
        assertEquals(listOf(RestLine(Rests.Apps, morning)), s.then)
        val hotspotFirst = adultSchedule(time(), pact(false, pause.minusHours(1), morning), zone)
        assertEquals(pause.minusHours(1), hotspotFirst.openUntil)
        assertEquals(listOf(RestLine(Rests.Hotspot, morning)), hotspotFirst.then)
        // The apps' own later pause never disappears behind the earlier hotspot lock.
        assertEquals(listOf(LaterPause(Rests.Apps, pause, morning)), hotspotFirst.later)
        assertEquals(listOf(LaterPause(Rests.Hotspot, pause.plusMinutes(30), morning)), s.later)
    }

    @Test fun sameStartWithDifferentEndsStaysSeparate() {
        val s = adultSchedule(time(), pact(false, pause, morning.plusHours(1)), zone)
        assertEquals(listOf(RestLine(Rests.Apps, morning), RestLine(Rests.Hotspot, morning.plusHours(1))), s.then)
    }

    @Test fun noPactNeverClaimsHotspot() {
        for (status in listOf(null, PactStatus.FreeSailing, PactStatus.NoCaptain)) {
            val s = adultSchedule(time(), status, zone)
            assertEquals(listOf(RestLine(Rests.Apps, morning)), s.then)
        }
        val hotspotOnly = adultSchedule(null, pact(false, pause, morning), zone)
        assertEquals(listOf(RestLine(Rests.Hotspot, morning)), hotspotOnly.then)
    }

    @Test fun restingNowIsGroupedByEnd() {
        val both = adultSchedule(time(TodayKind.Downtime, morning, pause.plusDays(1)), pact(true, morning, null), zone)
        assertNull(both.openUntil)
        assertEquals(listOf(RestLine(Rests.AppsAndHotspot, morning)), both.restingNow)
        // A locked hotspot while apps are open: the apps' pause still answers "open until".
        val hotspot = adultSchedule(time(), pact(true, morning.plusHours(2), null), zone)
        assertEquals(pause, hotspot.openUntil)
        assertEquals(listOf(RestLine(Rests.Apps, morning)), hotspot.then)
        assertEquals(listOf(RestLine(Rests.Hotspot, morning.plusHours(2))), hotspot.restingNow)
    }

    @Test fun nothingScheduledHasNoCountdown() {
        val s = adultSchedule(time(next = null, following = null), pact(false, null, null), zone)
        assertNull(s.openUntil)
        assertTrue(s.then.isEmpty())
        assertTrue(s.restingNow.isEmpty())
    }
}
