package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

class UsageCounterTest {
    private val zone = ZoneId.of("Europe/Copenhagen")
    private val base = Instant.parse("2026-10-21T10:00:00Z").toEpochMilli()
    private val pkg = "com.example.game"
    private val day = "2026-10-21"
    private fun event(offset: Long, kind: UsageCounter.Kind, instance: Int = 1) =
        UsageCounter.Event(base + offset, kind, pkg, "Activity$instance")
    private fun fresh() = UsageCounter(base, interactive = true, unlocked = true)

    @Test fun screenOffCutsBeforeActivityStops() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.SCREEN_OFF),
            event(1700, UsageCounter.Kind.STOP)), base + 5000, zone)
        assertEquals(1000, c.used(day, pkg))
    }

    @Test fun multipleClassesAreAUnionNotDoubleCounted() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.RESUME, 2),
            event(2000, UsageCounter.Kind.STOP), event(4000, UsageCounter.Kind.STOP, 2)), base + 6000, zone)
        assertEquals(4000, c.used(day, pkg))
    }

    @Test fun duplicateClassResumeDoesNotLeakAnActivity() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.RESUME),
            event(2000, UsageCounter.Kind.STOP)), base + 3000, zone)
        assertEquals(2000, c.used(day, pkg))
        assertTrue(c.visible.isEmpty())
    }

    @Test fun keyguardTimeIsExcluded() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.LOCK),
            event(4000, UsageCounter.Kind.UNLOCK), event(5000, UsageCounter.Kind.STOP)), base + 6000, zone)
        assertEquals(2000, c.used(day, pkg))
    }

    @Test fun shutdownAndStartupCloseOldVisibility() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.SHUTDOWN),
            event(2000, UsageCounter.Kind.STARTUP), event(3000, UsageCounter.Kind.SCREEN_ON),
            event(4000, UsageCounter.Kind.UNLOCK)), base + 6000, zone)
        assertEquals(1000, c.used(day, pkg))
        assertTrue(c.visible.isEmpty())
    }

    @Test fun pollingAndCheckpointReplayAreEquivalentToOneBatch() {
        val events = listOf(event(0, UsageCounter.Kind.RESUME), event(1000, UsageCounter.Kind.SCREEN_OFF),
            event(2000, UsageCounter.Kind.SCREEN_ON), event(3000, UsageCounter.Kind.STOP))
        val one = fresh().apply { consume(events, base + 4000, zone) }
        var split = fresh().apply { consume(events.filter { it.at < base + 1000 }, base + 1000, zone) }
        split = UsageCheckpoint.decode(UsageCheckpoint(split, zone.id, 1000, 1, 5).encode()).counter
        split.consume(events.filter { it.at >= base + 1000 }, base + 4000, zone)
        assertEquals(one.days, split.days)
        assertEquals(one.cursor, split.cursor)
        assertEquals(2000, split.used(day, pkg))
    }

    @Test fun routerMidnightSplitsElapsedTime() {
        val midnight = ZonedDateTime.parse("2026-10-22T00:00:00+02:00[Europe/Copenhagen]").toInstant().toEpochMilli()
        val c = UsageCounter(midnight - 1000, true, true)
        c.consume(listOf(UsageCounter.Event(midnight - 1000, UsageCounter.Kind.RESUME, pkg, "Activity")), midnight + 2000, zone)
        assertEquals(1000, c.used("2026-10-21", pkg))
        assertEquals(2000, c.used("2026-10-22", pkg))
    }

    @Test fun daylightSavingDaysUseRealElapsedTime() {
        for ((date, hours) in listOf("2026-03-29" to 23L, "2026-10-25" to 25L)) {
            val start = java.time.LocalDate.parse(date).atStartOfDay(zone)
            val from = start.toInstant().toEpochMilli()
            val until = start.plusDays(1).toInstant().toEpochMilli()
            val c = UsageCounter(from, true, true)
            c.consume(listOf(UsageCounter.Event(from, UsageCounter.Kind.RESUME, pkg, "Activity")), until, zone)
            assertEquals(hours * 3600000, c.used(date, pkg))
        }
    }

    @Test fun gapRetainsTotalsButClearsVisibility() {
        val c = fresh()
        c.consume(listOf(event(0, UsageCounter.Kind.RESUME)), base + 1000, zone)
        c.gap(base + 10000, true, true)
        c.consume(emptyList(), base + 11000, zone)
        assertEquals(1000, c.used(day, pkg))
    }

    @Test fun retentionKeepsThirtyCalendarDays() {
        val c = fresh()
        c.days["2026-09-21"] = mutableMapOf(pkg to 1)
        c.days["2026-09-22"] = mutableMapOf(pkg to 2)
        c.prune(base, zone)
        assertFalse(c.days.containsKey("2026-09-21"))
        assertTrue(c.days.containsKey("2026-09-22"))
    }

    @Test fun checkpointRejectsCorruptionRatherThanResettingBudget() {
        val c = fresh().apply { days[day] = mutableMapOf(pkg to 10) }
        val raw = UsageCheckpoint(c, zone.id, 1000, 1, 7, 2, day).encode()
        val restored = UsageCheckpoint.decode(raw)
        assertEquals(7, restored.sequence)
        assertEquals(2, restored.gaps)
        assertEquals(day, restored.uncertainThroughDay)
        for (bad in listOf(raw.replace("\"schema\":1", "\"schema\":2"), raw.replace(":10}", ":-1}"), "{broken")) {
            assertTrue(runCatching { UsageCheckpoint.decode(bad) }.isFailure)
        }
    }

    @Test fun boundaryEventsCannotBeProcessedTwice() {
        val c = fresh()
        assertTrue(runCatching { c.consume(listOf(event(1000, UsageCounter.Kind.RESUME)), base + 1000, zone) }.isFailure)
        c.consume(emptyList(), base + 1000, zone)
        assertTrue(runCatching { c.consume(listOf(event(0, UsageCounter.Kind.RESUME)), base + 2000, zone) }.isFailure)
    }

    @Test fun clockAndBootChangesAreExplicitGaps() {
        assertNull(usageDiscontinuity(2000, 2000, 1, 1000, 1000, 1))
        assertEquals("clock_changed", usageDiscontinuity(6000, 2000, 1, 1000, 1000, 1))
        assertEquals("clock_changed", usageDiscontinuity(0, 5000, 1, 1000, 1000, 1))
        assertEquals("reboot_gap", usageDiscontinuity(2000, 10, 2, 1000, 1000, 1))
        assertEquals("history_gap", usageDiscontinuity(90000000, 90000000, 1, 1000, 1000, 1))
    }

    @Test fun dailyTotalIsTheUnionOfOrdinaryAppsAndSkipsExcludedOnes() {
        val c = fresh().apply { excluded = setOf("com.android.dialer") }
        c.consume(listOf(
            event(0, UsageCounter.Kind.RESUME),                                       // game visible
            UsageCounter.Event(base + 1000, UsageCounter.Kind.RESUME, "com.example.video", "Pip"), // PiP on top
            event(3000, UsageCounter.Kind.STOP),
            UsageCounter.Event(base + 4000, UsageCounter.Kind.STOP, "com.example.video", "Pip"),
            UsageCounter.Event(base + 5000, UsageCounter.Kind.RESUME, "com.android.dialer", "Call"),
        ), base + 8000, zone)
        assertEquals(4000, c.used(day, UsageCounter.TOTAL))    // 0..4 s once, not 3 + 3 s
        assertEquals(3000, c.used(day, pkg))
        assertEquals(3000, c.used(day, "com.android.dialer"))  // per-app time is still known
    }
}
