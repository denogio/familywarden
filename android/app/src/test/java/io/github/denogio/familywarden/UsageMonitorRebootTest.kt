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
            mutableMapOf("com.example.game" to mutableSetOf("Game"))), zone.id, 50_000, 1)
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
        assertTrue(UsageJournal(context).load()!!.rebootDebits.isEmpty())
    }
    @Test fun rebootResumesWithoutChargingOffPeriod() {
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
        var malformed = false

        @Implementation
        override fun queryEvents(beginTime: Long, endTime: Long): UsageEvents? {
            if (malformed) {
                addEvent(ShadowUsageStatsManager.EventBuilder.buildEvent()
                    .setEventType(UsageEvents.Event.DEVICE_STARTUP).setTimeStamp(endTime + 1).build())
                return super.queryEvents(beginTime, endTime + 2)
            }
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

    @Test fun missingStartupFailsClosed() { gap() }

    @Test fun crashWithSyntheticEventsResumesAndDebitsExactlyOnce() {
        event(UsageEvents.Event.DEVICE_SHUTDOWN, wall - 20_000)
        event(UsageEvents.Event.DEVICE_STARTUP)
        event(UsageEvents.Event.SCREEN_INTERACTIVE, wall - 9_000)
        event(UsageEvents.Event.KEYGUARD_HIDDEN, wall - 9_000)
        event(UsageEvents.Event.ACTIVITY_RESUMED, wall - 8_000)
        val first = sample()
        assertEquals("ok", first.status)
        assertEquals(8_000L, first.millis["com.example.game"])
        assertEquals(23_000L, first.budgetMillis["com.example.game"])
        assertEquals(23_000L, first.budgetMillis[UsageCounter.TOTAL])
        val monitor = UsageMonitor(context, zone)
        assertEquals(first.budgetMillis, monitor.sample().budgetMillis)
        assertEquals(first.budgetMillis, UsageMonitor(context, zone).sample().budgetMillis)
        assertEquals(8_000L, monitor.historyTotals()[first.day])
        assertEquals(8_000L, monitor.historyApps()[first.day]!!["com.example.game"])
        val saved = UsageJournal(context).load()!!
        val upload = org.json.JSONObject(UsageUpload.report("fw-alex", "alex", first.day,
            saved.sequence, saved.counter.days[first.day].orEmpty(), emptyMap(), emptyMap(), false))
        assertEquals(8L, upload.getLong("total"))
        assertEquals(8L, upload.getJSONObject("apps").getLong("com.example.game"))
    }

    @Test fun splitScreenDebitsAppsButTotalOnlyOnceAndExcludesAllowedApps() {
        state.counter.visible["com.example.other"] = mutableSetOf("A", "B")
        state.counter.visible["com.example.essential"] = mutableSetOf("A")
        state.counter.visible["com.example.allowed"] = mutableSetOf("A")
        event(UsageEvents.Event.DEVICE_STARTUP)
        UsageJournal(context).save(state)
        val sample = UsageMonitor(context, zone).sample(setOf("com.example.essential", "com.example.allowed"))
        assertEquals(mapOf("total" to 15_000L, "com.example.game" to 15_000L,
            "com.example.other" to 15_000L), sample.budgetMillis)
        assertTrue(sample.millis.isEmpty())
    }

    @Test fun midnightUsesRouterLocalCurrentDayAndPreservesHistory() {
        val localZone = ZoneId.of("Pacific/Auckland") // Next router-local day, unlike UTC.
        state = state.copy(zone = localZone.id)
        state.counter.cursor = wall - 3_600_001
        state.counter.days["2026-10-24"] = mutableMapOf("total" to 42_000L)
        event(UsageEvents.Event.DEVICE_STARTUP)
        UsageJournal(context).save(state)
        val sample = UsageMonitor(context, localZone).sample()
        assertEquals("2026-10-25", sample.day)
        assertEquals(15_000L, sample.budgetMillis["total"])
        val saved = UsageJournal(context).load()!!
        assertEquals(setOf(sample.day), saved.rebootDebits.keys)
        assertEquals(42_000L, saved.counter.days["2026-10-24"]!!["total"])
    }

    @Test fun autoTimeSettingIsNotRequiredForPersistedHistory() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        Settings.Global.putInt(context.contentResolver, Settings.Global.AUTO_TIME, 0)
        assertEquals("ok", sample().status)
    }

    @Test fun longHistoryFailsClosed() {
        state.counter.cursor = wall - 86_400_001
        event(UsageEvents.Event.DEVICE_STARTUP)
        gap("history_gap")
    }

    @Test fun backwardWallClockFailsClosed() {
        state.counter.cursor = wall + 1
        gap("clock_changed")
    }

    @Test fun malformedEventsFailClosed() {
        state.counter.visible.clear()
        queryShadow().malformed = true
        gap("event_order_invalid")
    }

    @Test fun sameBootElapsedRegressionFailsClosedWithoutDebit() {
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 1)
        state.elapsed = SystemClock.elapsedRealtime() + 50_000
        event(UsageEvents.Event.DEVICE_STARTUP)
        gap()
    }

    @Test fun sameBootWallClockChangeFailsClosedWithoutDebit() {
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 1)
        state.elapsed = SystemClock.elapsedRealtime()
        gap("clock_changed")
    }

    @Test fun anotherObservedBootDebitsOnceMore() {
        event(UsageEvents.Event.DEVICE_STARTUP)
        sample()
        SystemClock.setCurrentTimeMillis(wall + 20_000)
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 3)
        event(UsageEvents.Event.DEVICE_STARTUP, wall + 10_000)
        val next = UsageMonitor(context, zone).sample()
        assertEquals(30_000L, next.budgetMillis[UsageCounter.TOTAL])
        // No visible app at the second checkpoint: only total receives another debit.
        assertEquals(15_000L, next.budgetMillis["com.example.game"])
        assertEquals(next.budgetMillis, UsageMonitor(context, zone).sample().budgetMillis)
    }

    @Test fun recoveryDoesNotClearUnrelatedUncertaintyAndPrunesOldDebitDays() {
        state.uncertainThroughDay = "2026-10-24"
        state.rebootDebits["2026-09-01"] = mutableMapOf("total" to 15_000L)
        event(UsageEvents.Event.DEVICE_STARTUP)
        val first = sample()
        assertEquals("usage_gap", first.status)
        assertFalse(first.reliable)
        assertEquals(0L, first.gaps)
        assertEquals("2026-10-24", UsageJournal(context).load()!!.uncertainThroughDay)
        // Move to tomorrow with matching elapsed time; normal pruning resumes.
        org.robolectric.shadows.ShadowSystemClock.advanceBy(java.time.Duration.ofDays(1))
        val monitor = UsageMonitor(context, zone)
        assertTrue(monitor.sample().reliable)
        assertFalse(UsageJournal(context).load()!!.rebootDebits.containsKey("2026-09-01"))
    }

    @Test fun malformedDebitCheckpointCannotRefillBudget() {
        for (bad in listOf<Any>("invalid", org.json.JSONObject().put("2026-10-24",
            org.json.JSONObject().put("total", -1)))) {
            try {
                UsageCheckpoint.decode(org.json.JSONObject(state.encode()).put("reboot_debits", bad).toString())
                fail("malformed debit accepted")
            } catch (_: IllegalArgumentException) { }
              catch (_: org.json.JSONException) { }
        }
    }

    @Test fun legacyShutdownObjectIsIgnoredAndMissingDebitDecodes() {
        val legacy = org.json.JSONObject(state.encode()).apply {
            remove("reboot_debits")
            put("shutdown", org.json.JSONObject().put("obsolete", true))
        }
        val decoded = UsageCheckpoint.decode(legacy.toString())
        assertTrue(decoded.rebootDebits.isEmpty())
        assertEquals(state.counter.visible, decoded.counter.visible)
        assertFalse(org.json.JSONObject(decoded.encode()).has("shutdown"))
    }
}
