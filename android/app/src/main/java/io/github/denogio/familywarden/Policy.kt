package io.github.denogio.familywarden

import java.time.DayOfWeek
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * What the router told us, as cached on the phone. Mirrors the payload of
 * familywarden's cgi-bin/mast (see mast_payload in the rpcd plugin).
 */
data class MastState(
    val serverTime: Long,
    val captain: String,
    val pactActive: Boolean,
    val pactType: String,      // "until" | "ongoing" | ""
    val pactUntil: Long,       // epoch, 0 for ongoing
    val releaseAt: Long,       // epoch the sail page releases the pact, 0 = not requested
    val rules: Rules,
    val binding: DeviceBinding? = null,
    val dns: DnsPolicy? = null,
    val requestNonce: String? = null,
    val wireVersion: Int = 1,
    val timeZone: String? = null,
    /** v2: the device id the router answered for. */
    val device: String? = null,
    /** v2: not approved (yet): no binding, no policy. */
    val pending: Boolean = false,
    /** v3: app policy of this phone's profile; null = no app control. */
    val apps: AppsPolicy? = null,
    /** v3: parent phones whose keys may later sign grants for this phone's profile. */
    val parents: List<ParentKey> = emptyList(),
    val request: ChildRequest? = null,
    val requests: List<ParentRequest> = emptyList(),
)

data class Rules(
    val enabled: Boolean,
    val policy: String,        // "allow" | "block" | "scheduled"
    val override: String,      // "none" | "block" | "allow"
    val windows: List<Window>,
)

/** An allow window like the router's schedules: days "mon tue", "HH:MM" bounds. */
data class Window(val days: Set<DayOfWeek>, val start: LocalTime, val stop: LocalTime)

/** Why hotspot is (not) blocked right now — shown in the app. */
enum class Reason { NOT_PAIRED, NO_CAPTAIN, NO_PACT, CAPTAIN_ALLOWED, BLOCKED }

data class Decision(val block: Boolean, val reason: Reason)

/**
 * Pure decision logic, no Android: block tethering while the captain is bound
 * by a pact AND blocked by their rules. Same semantics as familywarden's
 * forward chain (allow windows, overnight windows split at midnight).
 *
 * Works from the last synced state until the next successful sync. A pact only
 * ends when the router says so (or an "until" pact runs out), so staying off
 * the home Wi-Fi keeps the phone bound rather than freeing it.
 */
object Policy {
    fun decide(state: MastState?, now: ZonedDateTime): Decision {
        if (state == null) return Decision(false, Reason.NOT_PAIRED)
        if (state.captain.isEmpty()) return Decision(false, Reason.NO_CAPTAIN)
        if (!pactActive(state, now.toEpochSecond())) return Decision(false, Reason.NO_PACT)
        if (!captainBlocked(state.rules, now)) return Decision(false, Reason.CAPTAIN_ALLOWED)
        return Decision(true, Reason.BLOCKED)
    }

    fun pactActive(state: MastState, epoch: Long): Boolean {
        if (!state.pactActive) return false
        if (state.releaseAt > 0 && epoch >= state.releaseAt) return false
        return state.pactType == "ongoing" || state.pactUntil > epoch
    }

    fun captainBlocked(rules: Rules, now: ZonedDateTime): Boolean {
        if (!rules.enabled) return false
        if (rules.override == "block") return true
        if (rules.override == "allow") return false
        return when (rules.policy) {
            "block" -> true
            "scheduled" -> rules.windows.none { inWindow(it, now) }
            else -> false
        }
    }

    fun inWindow(w: Window, now: ZonedDateTime): Boolean {
        val day = now.dayOfWeek
        val t = now.toLocalTime()
        return when {
            w.start < w.stop -> day in w.days && t >= w.start && t < w.stop
            w.start > w.stop -> // overnight: start..24:00 on the day, 00:00..stop the next day
                (day in w.days && t >= w.start) || (day.minus(1) in w.days && t < w.stop)
            else -> day in w.days  // start == stop: the whole day
        }
    }

    /**
     * The next moment the decision may change, so an exact alarm can flip the
     * restriction on time (window bounds, pact end, release).
     */
    fun nextChange(state: MastState?, now: ZonedDateTime): ZonedDateTime? {
        return changeCandidates(state, now).filter { it.isAfter(now) }.minOrNull()
    }

    /** UI countdown: skip evaluation boundaries that do not actually flip hotspot. */
    fun nextBlockChange(state: MastState?, now: ZonedDateTime): ZonedDateTime? {
        val blocked = decide(state, now).block
        return changeCandidates(state, now)
            .filter { it.isAfter(now) && decide(state, it).block != blocked }
            .minOrNull()
    }

    private fun changeCandidates(state: MastState?, now: ZonedDateTime): List<ZonedDateTime> {
        if (state == null) return emptyList()
        val zone = now.zone
        val candidates = mutableListOf<ZonedDateTime>()
        val today = now.toLocalDate()
        for (offset in 0L..7L) {
            val date = today.plusDays(offset)
            for (w in state.rules.windows) {
                candidates += at(date, w.start, zone)
                candidates += at(date, w.stop, zone)
            }
            candidates += at(date, LocalTime.MIDNIGHT, zone)
        }
        listOf(state.pactUntil, state.releaseAt)
            .filter { it > 0 }
            .forEach { candidates += Instant.ofEpochSecond(it).atZone(zone) }
        return candidates
    }

    private fun at(date: LocalDate, time: LocalTime, zone: ZoneId) = ZonedDateTime.of(date, time, zone)

    private val dayNames = mapOf(
        "mon" to DayOfWeek.MONDAY, "tue" to DayOfWeek.TUESDAY, "wed" to DayOfWeek.WEDNESDAY,
        "thu" to DayOfWeek.THURSDAY, "fri" to DayOfWeek.FRIDAY, "sat" to DayOfWeek.SATURDAY,
        "sun" to DayOfWeek.SUNDAY,
    )

    fun parseDays(days: String): Set<DayOfWeek> =
        days.trim().split(Regex("\\s+")).mapNotNull { dayNames[it.lowercase()] }.toSet()
}
