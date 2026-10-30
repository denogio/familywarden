package io.github.denogio.familywarden

import android.app.AppOpsManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.os.Process
import android.os.SystemClock
import android.os.UserManager
import android.provider.Settings
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RuntimeEnvironment
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.Implementation
import org.robolectric.annotation.Implements
import org.robolectric.shadow.api.Shadow
import org.robolectric.shadows.ShadowSystemClock
import java.time.Duration
import org.robolectric.shadows.ShadowUsageStatsManager
import java.time.Instant
import java.time.ZoneId

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], instrumentedPackages = ["io.github.denogio.familywarden"],
    shadows = [UsageMonitorRebootTest.QueryUsageStatsManager::class])
class UsageMonitorRebootTest {
    private lateinit var context: Context
    private val zone = ZoneId.of("UTC")
    private val wall = Instant.parse("2026-10-24T12:00:00Z").toEpochMilli()
    private lateinit var state: UsageCheckpoint

    @Before fun setup() {
        context = RuntimeEnvironment.getApplication()
        SystemClock.setCurrentTimeMillis(wall)
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 2)
        Settings.Global.putInt(context.contentResolver, Settings.Global.AUTO_TIME, 1)
        shadowOf(context.getSystemService(UserManager::class.java)).setUserUnlocked(true)
        access(AppOpsManager.MODE_ALLOWED)
        state = UsageCheckpoint(UsageCounter(wall - 20_000, true, true,
            mutableMapOf("com.example.game" to mutableSetOf("Game"))), zone.id, 50_000, 1,
            shutdown = ShutdownCheckpoint(1, 50_000, wall - 20_000))
    }
    private fun access(mode: Int) = shadowOf(context.getSystemService(AppOpsManager::class.java)).setMode(
        AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName, mode)
    private fun event(type: Int, at: Long = wall - 10_000) {
        shadowOf(context.getSystemService(UsageStatsManager::class.java)).addEvent(
            ShadowUsageStatsManager.EventBuilder.buildEvent().setEventType(type).setTimeStamp(at)
                .setPackage("com.example.game").setClass("Game").build())
    }
    private fun sample(): UsageMonitor.Sample {
        UsageJournal(context).save(state)
        return UsageMonitor(context, zone).sample()
    }
    private fun gap(status: String = "reboot_gap") {
        val sample = sample()
        assertEquals(status, sample.status)
        assertFalse(sample.reliable)
        assertEquals(1L, sample.gaps)
        assertNull(UsageJournal(context).load()!!.shutdown)
    }
    @Test fun cleanRebootDoesNotChargeOldVisibilityAcrossOffPeriod() {
        event(UsageEvents.Event.DEVICE_SHUTDOWN, wall - 20_000)
        event(UsageEvents.Event.DEVICE_STARTUP)
        val sample = sample()
        assertEquals("ok", sample.status)
        assertTrue(sample.reliable)
        assertTrue(sample.millis.isEmpty())
        assertEquals(0L, sample.gaps)
    }
    @Test fun missingUsageAccessFailsClosedEvenWithEvidence() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        access(AppOpsManager.MODE_IGNORED)
        gap("usage_access_missing")
    }
    @Test fun lockedUserFailsClosedEvenWithEvidence() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        shadowOf(context.getSystemService(UserManager::class.java)).setUserUnlocked(false)
        gap("user_locked")
    }
    @Implements(UsageStatsManager::class)
    class QueryUsageStatsManager : ShadowUsageStatsManager() {
        var unavailable = false
        var queryDelayMillis = 0L

        @Implementation
        override fun queryEvents(beginTime: Long, endTime: Long): UsageEvents? {
            ShadowSystemClock.advanceBy(Duration.ofMillis(queryDelayMillis))
            return if (unavailable) null else super.queryEvents(beginTime, endTime)
        }
    }

    private fun queryShadow(): QueryUsageStatsManager =
        Shadow.extract(context.getSystemService(UsageStatsManager::class.java))

    @Test fun unavailableHistoryFailsClosedEvenWithEvidence() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        queryShadow().unavailable = true
        gap("usage_unavailable")
    }

    @Test fun shutdownSamplePastDeadlineCannotWriteMarker() {
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 1)
        state.elapsed = SystemClock.elapsedRealtime() - 20_000
        event(UsageEvents.Event.SCREEN_INTERACTIVE)
        UsageJournal(context).save(state)
        queryShadow().queryDelayMillis = 5_001
        val sample = UsageMonitor(context, zone).sample(shutdown = true)
        assertEquals("shutdown_timeout", sample.status)
        assertFalse(sample.reliable)
        assertEquals(1L, sample.gaps)
        assertNull(UsageJournal(context).load()!!.shutdown)
    }

    @Test fun missingStartupFailsClosed() { gap() }
    @Test fun crashWithSyntheticEventsFailsClosed() {
        state.shutdown = null
        event(UsageEvents.Event.DEVICE_SHUTDOWN, wall - 20_000)
        event(UsageEvents.Event.DEVICE_STARTUP)
        gap()
    }
    @Test fun manualTimeFailsClosed() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        Settings.Global.putInt(context.contentResolver, Settings.Global.AUTO_TIME, 0)
        gap()
    }
    @Test fun longHistoryFailsClosed() {
        state.counter.cursor = wall - 86_400_001
        state.shutdown = ShutdownCheckpoint(1, state.elapsed, state.counter.cursor)
        event(UsageEvents.Event.DEVICE_STARTUP)
        gap()
    }
    @Test fun sameBootElapsedRegressionIsNotCleanReboot() {
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 1)
        state.elapsed = SystemClock.elapsedRealtime() + 50_000
        state.shutdown = ShutdownCheckpoint(1, state.elapsed, state.counter.cursor)
        event(UsageEvents.Event.DEVICE_STARTUP)
        gap()
    }
}
