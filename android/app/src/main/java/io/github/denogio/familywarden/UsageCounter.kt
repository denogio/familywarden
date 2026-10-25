package io.github.denogio.familywarden

import java.time.Instant
import java.time.ZoneId

/** Plain, checkpointable event reducer. Milliseconds, union of visible activity classes per package.
 * Public UsageEvents has no instance ID: simultaneous instances of the same class cannot
 * be distinguished and may be undercounted. No hidden-API/reflection workaround.
 * PAUSED is intentionally not STOPPED: PiP/split-screen can remain visible while paused.
 * This event model is a candidate until physical-device PiP tests pass.
 */
class UsageCounter(
    var cursor: Long,
    var interactive: Boolean = false,
    var unlocked: Boolean = false,
    val visible: MutableMap<String, MutableSet<String>> = mutableMapOf(),
    val days: MutableMap<String, MutableMap<String, Long>> = mutableMapOf(),
) {
    /**
     * Apps that never count towards the daily total (essential and always-allowed). Set by
     * the caller before consuming; not persisted. Changing it affects only time from then on.
     */
    var excluded: Set<String> = emptySet()

    /** Same-writer accounting hook; invoked before adding each daily foreground slice. */
    var onInterval: ((Long, Long, Set<String>, Map<String, Long>) -> Unit)? = null

    enum class Kind { RESUME, STOP, SCREEN_ON, SCREEN_OFF, UNLOCK, LOCK, STARTUP, SHUTDOWN }
    data class Event(val at: Long, val kind: Kind, val pkg: String = "", val activity: String = "")

    /** Query contract: [cursor, until), in event order. All events at an instant are consumed
     * together; the next query starts at until, never at the last event timestamp.
     */
    fun consume(events: List<Event>, until: Long, zone: ZoneId) {
        require(until >= cursor)
        require(events.zipWithNext().all { (a, b) -> a.at <= b.at })
        require(events.all { it.at >= cursor && it.at < until })
        for (e in events) {
            charge(e.at, zone)
            when (e.kind) {
                Kind.RESUME -> if (e.pkg.isNotEmpty()) visible.getOrPut(e.pkg) { mutableSetOf() }.add(e.activity)
                Kind.STOP -> visible[e.pkg]?.let { it.remove(e.activity); if (it.isEmpty()) visible.remove(e.pkg) }
                Kind.SCREEN_ON -> interactive = true
                Kind.SCREEN_OFF -> interactive = false
                Kind.UNLOCK -> unlocked = true
                Kind.LOCK -> unlocked = false
                Kind.STARTUP, Kind.SHUTDOWN -> { visible.clear(); interactive = false; unlocked = false }
            }
        }
        charge(until, zone)
    }

    /** Missing events, reboot/clock uncertainty or revoked access is a gap, not zero use.
     * Caller retains totals and records the gap; unknown intervals are never invented.
     */
    fun gap(until: Long, screenOn: Boolean, keyguardUnlocked: Boolean) {
        cursor = until
        visible.clear()
        interactive = screenOn
        unlocked = keyguardUnlocked
    }

    fun used(day: String, pkg: String) = days[day]?.get(pkg) ?: 0L

    companion object {
        /** Key of the daily total in [days]; not a package name (those contain a dot). */
        const val TOTAL = "total"
    }

    fun prune(now: Long, zone: ZoneId) {
        val oldest = Instant.ofEpochMilli(now).atZone(zone).toLocalDate().minusDays(29).toString()
        days.keys.removeAll { it < oldest }
    }

    private fun charge(until: Long, zone: ZoneId) {
        if (interactive && unlocked && visible.isNotEmpty()) {
            var start = cursor
            while (start < until) {
                val local = Instant.ofEpochMilli(start).atZone(zone)
                val stop = minOf(until, local.toLocalDate().plusDays(1).atStartOfDay(zone).toInstant().toEpochMilli())
                val totals = days.getOrPut(local.toLocalDate().toString()) { mutableMapOf() }
                onInterval?.invoke(start, stop, visible.keys.toSet(), totals.toMap())
                for (pkg in visible.keys) totals[pkg] = Math.addExact(totals[pkg] ?: 0, stop - start)
                // Daily total (D9): the union, so PiP over another app counts once.
                if (visible.keys.any { it !in excluded }) totals[TOTAL] = Math.addExact(totals[TOTAL] ?: 0, stop - start)
                start = stop
            }
        }
        cursor = until
    }
}
