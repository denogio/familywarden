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
        val grantRemaining: Map<String, Long> = emptyMap(), val grantReliable: Boolean = false,
        val budgetDebits: Map<String, Long> = emptyMap()) {
        val budgetMillis: Map<String, Long> get() = millis.toMutableMap().apply {
            budgetDebits.forEach { (target, debit) -> this[target] = Math.addExact(this[target] ?: 0, debit) }
        }
    }
    private val journal = UsageJournal(context)
    private val state = journal.load() ?: UsageCheckpoint(
        UsageCounter(System.currentTimeMillis(), screenOn(), unlocked()), zone.id,
        SystemClock.elapsedRealtime(), boot())

    init { require(state.zone == zone.id) { "usage timezone change needs explicit migration" } }

    /** [excluded]: apps that do not count towards the daily total (see [UsageCounter.excluded]). */
    fun sample(excluded: Set<String> = emptySet(), grants: GrantInput? = null): Sample {
        state.counter.excluded = state.grants.previousExclusions(excluded)
        state.counter.onInterval = { start, stop, visible, measured ->
            val day = Instant.ofEpochMilli(start).atZone(zone).toLocalDate().toString()
            state.grants.charge(start, stop, visible, budgetUsage(day, measured))
        }
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
        val reboot = boot != state.boot
        val previousVisible = state.counter.visible.keys.toSet() - (state.counter.excluded + excluded)
        if (reboot && problem == "reboot_gap" && wall < state.counter.cursor) problem = "clock_changed"
        val events = mutableListOf<UsageCounter.Event>()
        if (problem == null || problem == "reboot_gap") {
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
        if (reboot && problem == "reboot_gap") {
            problem = when {
                wall - state.counter.cursor > 86_400_000 -> "history_gap"
                events.none { it.kind == UsageCounter.Kind.STARTUP } -> "reboot_gap"
                else -> null
            }
        }
        if (problem != null) {
            state.grants.gap()
            val previousDay = Instant.ofEpochMilli(state.counter.cursor).atZone(zone).toLocalDate().toString()
            state.uncertainThroughDay = maxOf(state.uncertainThroughDay, previousDay, day)
            state.gaps = Math.addExact(state.gaps, 1)
            state.counter.gap(wall, screenOn(), unlocked())
        } else {
            // Android may synthesize shutdown from its last persistence. Never charge
            // the off interval; only startup and subsequent events reopen visibility.
            // Current screen/keyguard state is applied after replay, never retroactively.
            val continuous = if (reboot) events.dropWhile { it.kind != UsageCounter.Kind.STARTUP } else events
            if (reboot) {
                state.grants.gap() // Old signed clocks cannot authorize this boot.
                val debits = state.rebootDebits.getOrPut(day) { mutableMapOf() }
                for (target in previousVisible + UsageCounter.TOTAL)
                    debits[target] = Math.addExact(debits[target] ?: 0, REBOOT_DEBIT_MS)
                state.counter.gap(state.counter.cursor, false, false)
            }
            state.counter.consume(continuous, wall, zone)
            if (reboot) {
                // An already-awake boot may have no screen/unlock events after STARTUP.
                // Preserve replayed visibility, but resume from actual state at this boundary.
                state.counter.interactive = screenOn()
                state.counter.unlocked = unlocked()
            } else state.grants.finishInterval(elapsed - state.elapsed)
        }
        state.elapsed = elapsed
        state.boot = boot
        state.sequence = Math.addExact(state.sequence, 1)
        // Never delete old history immediately on an uncertain clock jump.
        if (problem == null && day > state.uncertainThroughDay) {
            state.counter.prune(wall, zone)
            val oldest = Instant.ofEpochMilli(wall).atZone(zone).toLocalDate().minusDays(29).toString()
            state.rebootDebits.keys.removeAll { it < oldest }
        }
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
            problem == null && day > state.uncertainThroughDay, state.gaps, extra, problem == null && grantReady,
            state.rebootDebits[day]?.toMap().orEmpty())
    }

    companion object {
        const val REBOOT_DEBIT_MS = 15_000L
    }

    private fun budgetUsage(day: String, measured: Map<String, Long>): Map<String, Long> =
        measured.toMutableMap().apply {
            state.rebootDebits[day]?.forEach { (target, debit) ->
                this[target] = Math.addExact(this[target] ?: 0, debit)
            }
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
