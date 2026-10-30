package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test

class CleanRebootTest {
    private val wall = 1_790_000_000_000L
    private fun state() = UsageCheckpoint(UsageCounter(wall), "UTC", 50_000, 4,
        shutdown = ShutdownCheckpoint(4, 50_000, wall))
    private fun startup(at: Long = wall + 10_000) = UsageCounter.Event(at, UsageCounter.Kind.STARTUP)
    private fun valid(s: UsageCheckpoint = state(), now: Long = wall + 20_000, boot: Int = 5,
        auto: Boolean = true, events: List<UsageCounter.Event> = listOf(startup())) =
        cleanReboot(s, now, boot, auto, events)

    @Test fun finalCheckpointAndStartupProveContinuity() {
        assertTrue(valid())
        assertTrue(valid(events = listOf(UsageCounter.Event(wall, UsageCounter.Kind.SHUTDOWN), startup())))
        assertTrue(valid(s = UsageCheckpoint.decode(state().encode())))
    }
    @Test fun legacyCheckpointAndCrashHaveNoEvidence() {
        assertFalse(valid(s = state().apply { shutdown = null }))
        val old = org.json.JSONObject(state().encode()).apply { remove("shutdown") }.toString()
        assertNull(UsageCheckpoint.decode(old).shutdown)
        // Android synthesizes both events even for a crash: never accept them alone.
        assertFalse(valid(s = state().apply { shutdown = null }, events = listOf(
            UsageCounter.Event(wall, UsageCounter.Kind.SHUTDOWN), startup())))
    }
    @Test fun staleOrMismatchedMarkerIsRejected() {
        assertFalse(valid(s = state().apply { boot = 3 }))
        assertFalse(valid(s = state().apply { elapsed++ }))
        assertFalse(valid(s = state().apply { counter.cursor++ }))
        assertFalse(valid(boot = 6))
        assertFalse(valid(boot = 4))
    }
    @Test fun manualClockBackwardClockAndLongHistoryAreRejected() {
        assertFalse(valid(auto = false))
        assertFalse(valid(now = wall - 1))
        assertFalse(valid(now = wall + 86_400_001))
    }
    @Test fun missingStartupReversedOrderAndEventsOutsideWindowAreRejected() {
        assertFalse(valid(events = emptyList()))
        assertFalse(valid(events = listOf(startup(), UsageCounter.Event(wall, UsageCounter.Kind.SHUTDOWN))))
        assertFalse(valid(events = listOf(startup(wall - 1))))
        assertFalse(valid(events = listOf(startup(wall + 20_000))))
        assertFalse(valid(events = listOf(startup(), startup(wall + 11_000))))
        assertFalse(valid(events = listOf(UsageCounter.Event(wall, UsageCounter.Kind.RESUME), startup())))
    }
}
