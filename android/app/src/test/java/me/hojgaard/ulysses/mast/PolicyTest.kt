package me.hojgaard.ulysses.mast

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

class PolicyTest {
    private val zone = ZoneId.of("Europe/Copenhagen")

    // 2026-10-12 is a Monday
    private fun at(day: Int, hhmm: String) =
        ZonedDateTime.of(2026, 10, day, hhmm.substring(0, 2).toInt(), hhmm.substring(3).toInt(), 0, 0, zone)

    private fun window(days: String, start: String, stop: String) =
        Window(Policy.parseDays(days), LocalTime.parse(start), LocalTime.parse(stop))

    private fun state(
        pact: Boolean = true,
        type: String = "ongoing",
        until: Long = 0,
        releaseAt: Long = 0,
        policy: String = "scheduled",
        override: String = "none",
        enabled: Boolean = true,
        windows: List<Window> = listOf(window("mon tue wed thu fri sat sun", "06:00", "21:30")),
        captain: String = "dennis",
    ) = MastState(1, captain, pact, type, until, releaseAt, Rules(enabled, policy, override, windows))

    @Test fun notPaired() = assertEquals(Reason.NOT_PAIRED, Policy.decide(null, at(12, "23:00")).reason)

    @Test fun noCaptain() = assertEquals(Reason.NO_CAPTAIN, Policy.decide(state(captain = ""), at(12, "23:00")).reason)

    @Test fun noPactNeverBlocks() = assertFalse(Policy.decide(state(pact = false), at(12, "23:00")).block)

    @Test fun blockedOutsideWindowDuringPact() {
        val d = Policy.decide(state(), at(12, "21:30"))
        assertTrue(d.block)
        assertEquals(Reason.BLOCKED, d.reason)
    }

    @Test fun allowedInsideWindow() {
        assertFalse(Policy.decide(state(), at(12, "06:00")).block)
        assertFalse(Policy.decide(state(), at(12, "21:29")).block)
    }

    @Test fun multipleWindowsSameDayAllAllow() {
        val s = state(windows = listOf(window("mon", "07:00", "09:00"), window("mon", "15:00", "20:00")))
        assertFalse(Policy.decide(s, at(12, "08:00")).block)
        assertTrue(Policy.decide(s, at(12, "12:00")).block)
        assertFalse(Policy.decide(s, at(12, "16:00")).block)
        assertTrue(Policy.decide(s, at(13, "08:00")).block)  // tuesday: no window
    }

    @Test fun overnightWindowSpansMidnight() {
        val s = state(windows = listOf(window("sat", "22:00", "01:30")))
        assertFalse(Policy.decide(s, at(17, "23:00")).block)  // saturday night
        assertFalse(Policy.decide(s, at(18, "01:00")).block)  // sunday early
        assertTrue(Policy.decide(s, at(18, "01:30")).block)
        assertTrue(Policy.decide(s, at(17, "21:59")).block)
    }

    @Test fun startEqualsStopMeansAllDay() {
        val s = state(windows = listOf(window("mon", "00:00", "00:00")))
        assertFalse(Policy.decide(s, at(12, "03:00")).block)
        assertTrue(Policy.decide(s, at(13, "03:00")).block)
    }

    @Test fun blockPolicyAndOverride() {
        assertTrue(Policy.decide(state(policy = "block"), at(12, "12:00")).block)
        assertTrue(Policy.decide(state(override = "block"), at(12, "12:00")).block)
        assertFalse(Policy.decide(state(policy = "allow"), at(12, "23:00")).block)
        assertFalse(Policy.decide(state(enabled = false), at(12, "23:00")).block)
    }

    @Test fun untilPactEnds() {
        val now = at(12, "23:00")
        assertTrue(Policy.decide(state(type = "until", until = now.toEpochSecond() + 60), now).block)
        assertFalse(Policy.decide(state(type = "until", until = now.toEpochSecond()), now).block)
    }

    @Test fun releaseFromSailPageEndsPact() {
        val now = at(12, "23:00")
        assertTrue(Policy.decide(state(releaseAt = now.toEpochSecond() + 60), now).block)
        assertFalse(Policy.decide(state(releaseAt = now.toEpochSecond()), now).block)
    }

    @Test fun cachedPactKeepsBindingWithoutSync() {
        // No staleness: the last synced pact holds until the router says otherwise
        assertTrue(Policy.decide(state(), at(12, "23:00").plusYears(1)).block)
    }

    @Test fun nextChangeIsTheNextBoundary() {
        assertEquals(at(12, "21:30"), Policy.nextChange(state(), at(12, "20:00")))
        assertEquals(at(13, "00:00"), Policy.nextChange(state(), at(12, "22:00")))
        val now = at(12, "20:00")
        val release = now.toEpochSecond() + 600
        assertEquals(now.plusMinutes(10), Policy.nextChange(state(releaseAt = release), now))
        assertNull(Policy.nextChange(null, now))
    }

    @Test fun parseDaysIgnoresJunk() =
        assertEquals(3, Policy.parseDays(" mon  TUE xyz sun ").size)
}
