package io.github.denogio.familywarden

import android.app.AppOpsManager
import android.app.KeyguardManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.os.PowerManager
import android.os.Process
import android.os.SystemClock
import android.os.UserManager
import android.provider.Settings
import java.time.Instant
import java.time.ZoneId
import kotlin.math.abs

/** Check continuity before trusting wall-clock usage event timestamps. */
fun usageDiscontinuity(wall: Long, elapsed: Long, boot: Int,
    previousWall: Long, previousElapsed: Long, previousBoot: Int): String? = when {
    boot != previousBoot || elapsed < previousElapsed -> "reboot_gap"
    abs((wall - previousWall) - (elapsed - previousElapsed)) > 2_000 -> "clock_changed"
    wall - previousWall > 86_400_000 -> "history_gap"
    else -> null
}

/** Inputs come only from the verified cache; not from UI commands or upload replies. */
data class GrantInput(val scope: String, val device: String, val appsJson: String,
    val clock: PolicyClock?, val userApps: Set<String>,
    val sharedDay: String = "", val shared: Map<String, Long> = emptyMap())

/** Local, durable collector. No networking, grants, shared-budget allocation or v3 activation.
 * One service owns this instance on one worker thread. Persist before enforcing decisions.
 */
class UsageMonitor(private val context: Context, private val zone: ZoneId) {
    data class Sample(val day: String, val millis: Map<String, Long>, val sequence: Long,
        val status: String, val reliable: Boolean, val gaps: Long,
        val grantRemaining: Map<String, Long> = emptyMap(), val grantReliable: Boolean = false)
    private val journal = UsageJournal(context)
    private val state = journal.load() ?: UsageCheckpoint(
        UsageCounter(System.currentTimeMillis(), screenOn(), unlocked()), zone.id,
        SystemClock.elapsedRealtime(), boot())

    init { require(state.zone == zone.id) { "usage timezone change needs explicit migration" } }

    /** [excluded]: apps that do not count towards the daily total (see [UsageCounter.excluded]). */
    fun sample(excluded: Set<String> = emptySet(), grants: GrantInput? = null): Sample {
        state.counter.excluded = state.grants.previousExclusions(excluded)
        state.counter.onInterval = state.grants::charge
        val wall = System.currentTimeMillis()
        val elapsed = SystemClock.elapsedRealtime()
        val boot = boot()
        val day = Instant.ofEpochMilli(wall).atZone(zone).toLocalDate().toString()
        val ops = context.getSystemService(AppOpsManager::class.java)
        var problem = when {
            ops.unsafeCheckOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName) != AppOpsManager.MODE_ALLOWED -> "usage_access_missing"
            !context.getSystemService(UserManager::class.java).isUserUnlocked -> "user_locked"
            else -> usageDiscontinuity(wall, elapsed, boot, state.counter.cursor, state.elapsed, state.boot)
        }
        val events = mutableListOf<UsageCounter.Event>()
        if (problem == null) {
            val raw = context.getSystemService(UsageStatsManager::class.java).queryEvents(state.counter.cursor, wall)
            if (raw == null) problem = "usage_unavailable"
            else {
                val e = UsageEvents.Event()
                var count = 0
                while (raw.getNextEvent(e)) {
                    if (++count > 50_000) { problem = "event_overflow"; break }
                    val kind = when (e.eventType) {
                        UsageEvents.Event.ACTIVITY_RESUMED -> UsageCounter.Kind.RESUME
                        UsageEvents.Event.ACTIVITY_STOPPED -> UsageCounter.Kind.STOP
                        UsageEvents.Event.SCREEN_INTERACTIVE -> UsageCounter.Kind.SCREEN_ON
                        UsageEvents.Event.SCREEN_NON_INTERACTIVE -> UsageCounter.Kind.SCREEN_OFF
                        UsageEvents.Event.KEYGUARD_HIDDEN -> UsageCounter.Kind.UNLOCK
                        UsageEvents.Event.KEYGUARD_SHOWN -> UsageCounter.Kind.LOCK
                        UsageEvents.Event.DEVICE_STARTUP -> UsageCounter.Kind.STARTUP
                        UsageEvents.Event.DEVICE_SHUTDOWN -> UsageCounter.Kind.SHUTDOWN
                        else -> null
                    }
                    if (kind != null) events.add(UsageCounter.Event(e.timeStamp, kind, e.packageName.orEmpty(), e.className.orEmpty()))
                }
                if (events.zipWithNext().any { (a, b) -> a.at > b.at } ||
                    events.any { it.at < state.counter.cursor || it.at >= wall }) problem = "event_order_invalid"
            }
        }
        if (problem != null) {
            state.grants.gap()
            val previousDay = Instant.ofEpochMilli(state.counter.cursor).atZone(zone).toLocalDate().toString()
            state.uncertainThroughDay = maxOf(state.uncertainThroughDay, previousDay, day)
            state.gaps = Math.addExact(state.gaps, 1)
            state.counter.gap(wall, screenOn(), unlocked())
        } else {
            state.counter.consume(events, wall, zone)
            state.grants.finishInterval(elapsed - state.elapsed)
        }
        state.elapsed = elapsed
        state.boot = boot
        state.sequence = Math.addExact(state.sequence, 1)
        // Never delete old history immediately on an uncertain clock jump.
        if (problem == null && day > state.uncertainThroughDay) state.counter.prune(wall, zone)
        val server = grants?.clock?.now(elapsed, boot)
        val grantReady = if (grants != null && server != null) state.grants.configure(GrantLedger.Context(
            grants.scope, grants.device, grants.appsJson, zone, wall, server, excluded, grants.userApps,
            problem == null && day > state.uncertainThroughDay, state.uncertainThroughDay,
            grants.sharedDay, grants.shared))
            else { state.grants.gap(); false }
        val extra = if (problem == null && grantReady && grants != null)
            state.grants.available(grants.scope, server) else emptyMap()
        journal.save(state)
        return Sample(day, state.counter.days[day]?.toMap() ?: emptyMap(), state.sequence,
            problem ?: if (day <= state.uncertainThroughDay) "usage_gap" else "ok",
            problem == null && day > state.uncertainThroughDay, state.gaps, extra, problem == null && grantReady)
    }

    /** Recorded local totals only; gaps are not reconstructed or presented as complete history. */
    fun historyTotals(): Map<String, Long> = state.counter.days.mapNotNull { (day, apps) ->
        apps[UsageCounter.TOTAL]?.let { day to it }
    }.toMap()

    /** Per-day app counters for display, busiest first and bounded; never the total key. */
    fun historyApps(perDay: Int = 12): Map<String, Map<String, Long>> = state.counter.days.mapValues { (_, apps) ->
        apps.filterKeys { it != UsageCounter.TOTAL }.filterValues { it > 0 }.entries
            .sortedByDescending { it.value }.take(perDay).associate { it.key to it.value }
    }.filterValues { it.isNotEmpty() }

    private fun screenOn() = context.getSystemService(PowerManager::class.java).isInteractive
    private fun unlocked() = !context.getSystemService(KeyguardManager::class.java).isKeyguardLocked
    private fun boot() = Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)
}
