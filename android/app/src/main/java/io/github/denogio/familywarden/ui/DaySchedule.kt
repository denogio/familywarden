package io.github.denogio.familywarden.ui

import io.github.denogio.familywarden.Policy
import io.github.denogio.familywarden.Rules
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

/** Display only: half-open intervals on the selected local date's 24-hour clock. */
data class DayInterval(val startMinute: Int, val endExclusive: Int)

/** Reuse the enforcement evaluator, including weekday/overnight/override semantics. */
fun daySchedule(rules: Rules, date: LocalDate, zone: ZoneId): List<DayInterval> {
    val out = mutableListOf<DayInterval>()
    var start: Int? = null
    for (minute in 0..1440) {
        val local = if (minute < 1440) date.atTime(LocalTime.of(minute / 60, minute % 60)) else null
        val offsets = local?.let { zone.rules.getValidOffsets(it) }.orEmpty()
        // ZonedDateTime.of() silently moves a DST gap into the next hour. Do not
        // paint that nonexistent clock hour as an earlier allowed window.
        val allowed = local != null && offsets.isNotEmpty() && !Policy.captainBlocked(rules,
            ZonedDateTime.ofLocal(local, zone, offsets.first()))
        if (allowed && start == null) start = minute
        if (!allowed && start != null) {
            out += DayInterval(start, minute)
            start = null
        }
    }
    return out
}
