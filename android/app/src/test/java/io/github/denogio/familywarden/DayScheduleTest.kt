package io.github.denogio.familywarden

import io.github.denogio.familywarden.ui.DayInterval
import io.github.denogio.familywarden.ui.daySchedule
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

class DayScheduleTest {
    private val monday = LocalDate.parse("2026-10-19")
    private val zone = ZoneId.of("Europe/Copenhagen")
    private fun rules(vararg windows: Window) = Rules(true, "scheduled", "none", windows.toList())
    private fun window(days: String, start: String, stop: String) = Window(Policy.parseDays(days), LocalTime.parse(start), LocalTime.parse(stop))

    @Test fun normalWindowHasExclusiveEndAndCorrectWeekday() {
        val r = rules(window("mon", "07:00", "20:00"))
        assertEquals(listOf(DayInterval(420, 1200)), daySchedule(r, monday, zone))
        assertEquals(emptyList<DayInterval>(), daySchedule(r, monday.plusDays(1), zone))
    }

    @Test fun overnightSpillsIntoNextDayIncludingSundayToMonday() {
        val r = rules(window("sun", "22:00", "01:30"))
        assertEquals(listOf(DayInterval(0, 90)), daySchedule(r, monday, zone))
        assertEquals(listOf(DayInterval(1320, 1440)), daySchedule(r, monday.minusDays(1), zone))
    }

    @Test fun mergesOverlappingAndTouchingWindows() {
        val r = rules(window("mon", "07:00", "12:00"), window("mon", "11:00", "15:00"), window("mon", "15:00", "20:00"))
        assertEquals(listOf(DayInterval(420, 1200)), daySchedule(r, monday, zone))
    }

    @Test fun equalStartAndStopMeansWholeSelectedDay() {
        val r = rules(window("mon", "07:00", "07:00"))
        assertEquals(listOf(DayInterval(0, 1440)), daySchedule(r, monday, zone))
        assertEquals(emptyList<DayInterval>(), daySchedule(r, monday.plusDays(1), zone))
    }

    @Test fun springDstGapDoesNotShiftAWindowIntoNonexistentClockHours() {
        val date = LocalDate.parse("2026-03-29")
        val r = rules(window("sun", "03:00", "04:00"))
        assertEquals(listOf(DayInterval(180, 240)), daySchedule(r, date, zone))
        assertEquals(emptyList<DayInterval>(), daySchedule(rules(window("sun", "02:00", "03:00")), date, zone))
    }

    @Test fun autumnRepeatedHourIsShownAsLocalClockCoverageNotElapsedDuration() {
        val date = LocalDate.parse("2026-10-25")
        val r = rules(window("sun", "02:00", "03:00"))
        assertEquals(listOf(DayInterval(120, 180)), daySchedule(r, date, zone))
    }

    @Test fun overridesAndDisabledRulesUseExistingPolicySemantics() {
        val r = rules()
        assertEquals(emptyList<DayInterval>(), daySchedule(r, monday, zone))
        assertEquals(listOf(DayInterval(0, 1440)), daySchedule(r.copy(override = "allow"), monday, zone))
        assertEquals(emptyList<DayInterval>(), daySchedule(r.copy(policy = "allow", override = "block"), monday, zone))
        assertEquals(listOf(DayInterval(0, 1440)), daySchedule(r.copy(enabled = false, override = "block"), monday, zone))
    }
}
