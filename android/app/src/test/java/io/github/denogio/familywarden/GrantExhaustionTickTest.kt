package io.github.denogio.familywarden

import android.app.AppOpsManager
import android.app.KeyguardManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageInfo
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.os.Process
import android.os.SystemClock
import android.os.UserManager
import android.provider.Settings
import org.json.JSONArray
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RuntimeEnvironment
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.LooperMode
import org.robolectric.shadows.ShadowSystemClock
import org.robolectric.shadows.ShadowUsageStatsManager
import java.time.Duration
import java.time.Instant
import java.time.ZoneId

@RunWith(RobolectricTestRunner::class)
// Instrument app calls to System.currentTimeMillis so UsageMonitor shares Robolectric's clock.
@Config(sdk = [35], instrumentedPackages = ["io.github.denogio.familywarden"])
@LooperMode(LooperMode.Mode.PAUSED)
class GrantExhaustionTickTest {
    private val video = "com.example.video"
    private val grantId = "a".repeat(32)
    private val zone = ZoneId.of("UTC")
    private val router = PhoneKey.generate() // Ephemeral fixture key; never a deployment key.
    private lateinit var context: Context
    private lateinit var store: Store
    private lateinit var service: AppControlService
    private val handlers = mutableListOf<Handler>()
    private val defaultClock = AppControl.nowProvider

    @Before fun setUp() {
        context = RuntimeEnvironment.getApplication()
        SystemClock.setCurrentTimeMillis(Instant.parse("2026-10-24T12:00:00Z").toEpochMilli())
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 1)
        shadowOf(context.getSystemService(PowerManager::class.java)).setIsInteractive(true)
        shadowOf(context.getSystemService(KeyguardManager::class.java)).setKeyguardLocked(false)
        shadowOf(context.getSystemService(UserManager::class.java)).setUserUnlocked(true)
        shadowOf(context.getSystemService(AppOpsManager::class.java)).setMode(
            AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName, AppOpsManager.MODE_ALLOWED)
        shadowOf(Mast.dpm(context)).setDeviceOwner(Mast.admin(context))
        shadowOf(context.packageManager).installPackage(PackageInfo().apply {
            packageName = video
            applicationInfo = ApplicationInfo().apply { packageName = video; flags = 0 }
        })
        context.getSharedPreferences("familywarden", Context.MODE_PRIVATE).edit().putString("device_id", "fw-alex").commit()
        store = Store(context).apply { host = "router.test"; publicKey = router.publicKey }
        AppControl.stopped = false
        AppControl.publish(null)
        AppControl.nowProvider = { Instant.ofEpochMilli(SystemClock.uptimeMillis()).atZone(it) }
        service = newProcessService()
    }

    @After fun tearDown() {
        handlers.forEach { it.removeCallbacksAndMessages(null) }
        AppControl.nowProvider = defaultClock
        AppControl.stopped = false
        AppControl.publish(null)
    }

    /** Attach a fresh service without starting its worker: only explicit ticks run.
     * A restart drops every service/monitor field, retaining only Android prefs and files.
     */
    private fun newProcessService(): AppControlService = Robolectric.buildService(AppControlService::class.java).get().also {
        val handler = Handler(Looper.getMainLooper())
        handlers.add(handler)
        AppControlService::class.java.getDeclaredField("handler").apply { isAccessible = true }.set(it, handler)
    }

    private fun reply(grant: Boolean, blocked: Boolean = false, nonce: String = "c".repeat(64),
        id: String = grantId, grantStart: Long? = null): Sync.Result.Ok {
        val fixture = javaClass.classLoader!!.getResourceAsStream("v3-payload.json")!!.bufferedReader().use { it.readText() }
        val now = SystemClock.uptimeMillis() / 1000
        val raw = JSONObject(fixture).apply {
            put("device", "fw-alex"); put("nonce", nonce); put("server_time", now); put("timezone", zone.id)
            getJSONObject("binding").put("device_id", "fw-alex").put("profile_id", "alex")
            put("requires", JSONArray(listOf("apps-v1", "screen-time-v1", "grants-v1")))
            put("apps", JSONObject().apply {
                put("rev", "0123456789abcdef"); put("suspend_new", false)
                put("limits", JSONArray().apply { if (!blocked) put(JSONObject().put("package", video).put("minutes", 1)) })
                put("approved", JSONArray(if (blocked) emptyList<String>() else listOf(video)))
                put("blocked", JSONArray(if (blocked) listOf(video) else emptyList<String>()))
                put("downtime", JSONArray()); put("always_allowed", JSONArray())
                put("grants", JSONArray().apply { if (grant) put(JSONObject().put("grant_id", id)
                    .put("device_id", "fw-alex").put("target", video).put("minutes", 1)
                    .put("not_before", grantStart ?: now).put("not_after", (grantStart ?: now) + 7200)) })
            })
        }.toString()
        val bytes = raw.toByteArray(Charsets.UTF_8)
        assertTrue(Usign.verify(Usign.parsePublicKey(router.publicKey), router.sign(bytes), bytes))
        return Sync.Result.Ok(raw, Payload.parse(raw), SystemClock.elapsedRealtime())
    }

    private fun event(type: Int) {
        shadowOf(context.getSystemService(UsageStatsManager::class.java)).addEvent(
            ShadowUsageStatsManager.EventBuilder.buildEvent().setEventType(type)
                .setPackage(video).setClass("VideoActivity").setTimeStamp(SystemClock.uptimeMillis()).build())
    }

    private fun advance(millis: Long) { ShadowSystemClock.advanceBy(Duration.ofMillis(millis)) }
    private fun tick() {
        service.tick()
        assertNotNull("tick must publish a sample: ${org.robolectric.shadows.ShadowLog.getLogsForTag("UlyssesMast")}", AppControl.usageReport)
        assertTrue("reliable sample required: ${store.appsStatus}; ${checkpoint().encode()}", AppControl.usageReport!!.reliable)
    }
    // Robolectric DPM does not propagate suspension into PackageManager; query the same DPM API as reconcile.
    private fun suspended() = Mast.dpm(context).isPackageSuspended(Mast.admin(context), video)
    private fun checkpoint() = requireNotNull(UsageJournal(context).load())
    private fun spent(): Long {
        val state = requireNotNull(store.effectiveState)
        val binding = requireNotNull(state.binding)
        val scope = "${Usign.parsePublicKey(router.publicKey).fingerprint}|${store.phoneKey.fingerprint}|${binding.deviceId}|${binding.profileId}"
        return checkpoint().grants.spent(scope, grantId)
    }

    private fun exhaustStandingLimit() {
        store.accept(reply(grant = false))
        tick()
        event(UsageEvents.Event.SCREEN_INTERACTIVE)
        event(UsageEvents.Event.KEYGUARD_HIDDEN)
        event(UsageEvents.Event.ACTIVITY_RESUMED)
        advance(59_999)
        tick()
        assertEquals(59_999L, AppControl.usageReport!!.used[video])
        assertFalse("standing minute still has one millisecond", suspended())
        advance(1)
        tick()
        assertEquals(60_000L, AppControl.usageReport!!.used[video])
        assertTrue("standing minute exhausted without a grant", suspended())
    }

    @Test fun approvedMinuteExhaustsExactlyWithoutRefillAndSurvivesProcessDeath() {
        exhaustStandingLimit()
        val approved = reply(grant = true, nonce = "d".repeat(64))
        store.accept(approved)
        tick()
        assertFalse(suspended())
        advance(30_000)
        tick()
        assertEquals(30_000L, spent())
        assertEquals(30_000L, AppControl.usageReport!!.grantRemaining[video])
        assertFalse(suspended())

        val persisted = checkpoint().encode()
        handlers.forEach { it.removeCallbacksAndMessages(null) }
        AppControl.publish(null)
        service = newProcessService() // kill -9: no stop/lift or graceful flush.
        store = Store(context)
        assertEquals(persisted, checkpoint().encode())
        tick()
        assertEquals(30_000L, spent())
        assertFalse(suspended())
        advance(29_999)
        tick()
        assertEquals(59_999L, spent())
        assertFalse("one approved millisecond is still available", suspended())
        advance(1)
        tick()
        assertEquals(60_000L, spent())
        assertEquals(0L, AppControl.usageReport!!.grantRemaining[video])
        assertTrue("approved minute exhausted: pause again", suspended())

        val exhausted = checkpoint().grants.encode().getJSONArray("records").toString()
        advance(15_000)
        store.accept(approved) // Exact same nonce and signed bytes, not a new grant.
        tick()
        assertEquals(exhausted, checkpoint().grants.encode().getJSONArray("records").toString())
        assertEquals(60_000L, spent())
        assertTrue(suspended())
        handlers.forEach { it.removeCallbacksAndMessages(null) }
        service = newProcessService()
        tick()
        assertEquals(60_000L, spent())
        assertTrue(suspended())
    }

    @Test fun cleanShutdownKeepsCountingAndAllowsAckAfterSignedSync() {
        Settings.Global.putInt(context.contentResolver, Settings.Global.AUTO_TIME, 1)
        exhaustStandingLimit()
        store.accept(reply(grant = true, nonce = "d".repeat(64)))
        tick()
        advance(10_000)
        // Exercise the same runtime receiver callback as the protected shutdown broadcast.
        val receiver = AppControlService::class.java.getDeclaredField("receiver").apply { isAccessible = true }
            .get(service) as android.content.BroadcastReceiver
        receiver.onReceive(context, android.content.Intent(android.content.Intent.ACTION_SHUTDOWN))
        val final = checkpoint()
        assertEquals(final.counter.cursor, final.shutdown!!.wall)
        assertEquals(10_000L, spent())
        advance(30_000)
        service.tick() // No queued/screen tick may invalidate the final checkpoint.
        assertEquals(final.encode(), checkpoint().encode())
        event(UsageEvents.Event.DEVICE_STARTUP)
        Settings.Global.putInt(context.contentResolver, Settings.Global.BOOT_COUNT, 2)
        advance(1)
        service = newProcessService()
        store = Store(context)
        store.appsApplied = ""
        tick()
        assertEquals("grant_unavailable", store.appsStatus)
        assertFalse(AppControl.usageReport!!.grantReliable)
        assertTrue("cached grants cannot unpause an unanchored boot", suspended())
        assertEquals(70_000L, AppControl.usageReport!!.used[video])
        assertEquals(10_000L, spent()) // Off interval was not foreground grant spending.
        assertNull(checkpoint().shutdown)
        advance(1)
        tick() // Another pre-sync sample must still fail closed for grants.
        assertFalse(AppControl.usageReport!!.grantReliable)
        advance(1)
        store.accept(reply(grant = true, nonce = "e".repeat(64),
            grantStart = checkpoint().grants.encode().getJSONArray("records").getJSONObject(0).getLong("from")))
        tick()
        assertEquals("ok", store.appsStatus)
        assertEquals(store.effectiveState!!.apps!!.rev, store.appsApplied)
        assertTrue(AppControl.usageReport!!.grantReliable)
        // The unanchored interval may conservatively consume grants, but never daily use.
        assertEquals(70_000L, AppControl.usageReport!!.used[video])
        assertTrue("signed sync must not refill the spent grant", spent() >= 10_000L)
        assertTrue(AppControl.usageReport!!.grantRemaining.getValue(video) <= 50_000L)
        event(UsageEvents.Event.SCREEN_INTERACTIVE)
        event(UsageEvents.Event.KEYGUARD_HIDDEN)
        event(UsageEvents.Event.ACTIVITY_RESUMED)
        advance(1000)
        tick()
        assertEquals(71_000L, AppControl.usageReport!!.used[video])
    }

    @Test fun blockedAppNeverUnpausesEvenWithAGrant() {
        store.accept(reply(grant = true, blocked = true))
        tick()
        assertTrue(suspended())
        event(UsageEvents.Event.ACTIVITY_RESUMED)
        advance(30_000)
        tick()
        assertTrue(suspended())
        assertEquals(0L, spent())
    }

    @Test fun midReadPolicyReplacementDoesNotEnforceAMixedSnapshot() {
        exhaustStandingLimit()
        store.accept(reply(grant = true, nonce = "d".repeat(64)))
        tick()
        advance(30_000)
        tick()
        assertFalse(suspended())
        val before = checkpoint().encode()
        val report = AppControl.usageReport
        val enforcement = context.getSharedPreferences("app-enforcement", Context.MODE_PRIVATE).all.toMap()
        val replacement = reply(grant = true, nonce = "e".repeat(64), id = "b".repeat(32))
        var replaced = false
        val clock = AppControl.nowProvider
        AppControl.nowProvider = { z ->
            // First clock read is after effectiveState, before payload/clock/grant reads.
            if (!replaced) { replaced = true; store.accept(replacement) }
            clock(z)
        }
        service.tick()
        AppControl.nowProvider = clock
        assertTrue(replaced)
        assertEquals(replacement.payload, store.payload)
        assertFalse("mixed snapshot must not push Alex out of the approved app", suspended())
        assertEquals(enforcement, context.getSharedPreferences("app-enforcement", Context.MODE_PRIVATE).all)
        assertEquals("retry must not sample or checkpoint mixed input", before, checkpoint().encode())
        assertSame(report, AppControl.usageReport)
        tick()
        assertEquals("e".repeat(64), store.effectiveState!!.requestNonce)
        assertTrue(AppControl.usageReport!!.grantReliable)
        assertEquals(60_000L, AppControl.usageReport!!.grantRemaining[video])
        assertFalse(suspended())
        assertEquals(checkpoint().sequence, UsageCheckpoint.decode(before).sequence + 1)
    }
}
