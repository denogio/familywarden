package io.github.denogio.familywarden.ui

import io.github.denogio.familywarden.*
import java.time.LocalDate
import java.time.ZonedDateTime

/** Immutable service observation, never a second usage collector or authorization source. */
data class AppUsageReport(
    val revision: String,
    val zone: String,
    val day: LocalDate,
    val elapsedMillis: Long,
    val reliable: Boolean,
    val used: Map<String, Long>,
    val labels: Map<String, String> = emptyMap(),
    val historyTotals: Map<String, Long> = emptyMap(),
    val essential: Set<String> = emptySet(),
)

enum class TodayKind { Allowed, NearLimit, TimesUp, Downtime, Unknown }

data class ScreenTimeState(
    val kind: TodayKind,
    val usedMillis: Long?,
    val limitMillis: Long?,
    val nextDowntimeChange: ZonedDateTime?,
    val resetsAt: ZonedDateTime,
    val apps: List<AppTimeRow>,
    val alwaysAllowed: List<String>,
    val historyTotals: Map<String, Long>,
    val usageAccessMissing: Boolean = false,
) {
    val remainingMillis: Long? get() = usedMillis?.let { used -> limitMillis?.let { (it - used).coerceAtLeast(0) } }
}

data class AppTimeRow(val pkg: String, val label: String, val usedMillis: Long?,
    val limitMillis: Long?, val blocked: Boolean, val unlimited: Boolean)

/** Display the signed rules and fresh, reliable observations; never extrapolate a countdown. */
fun screenTimeState(apps: AppsPolicy, report: AppUsageReport?, now: ZonedDateTime,
    elapsedMillis: Long, usageAccess: Boolean): ScreenTimeState {
    val local = apps.toLocal(now.zone, now.toLocalDate())
    val matching = report?.takeIf { it.revision == apps.rev && it.zone == now.zone.id && it.day == now.toLocalDate() }
    // This is display freshness, NOT expiry of the trusted offline policy.
    val current = matching?.takeIf { elapsedMillis - it.elapsedMillis in 0..2 * AppControl.SCREEN_ON_INTERVAL_MS }
    val reliable = current?.reliable == true && usageAccess
    val total = if (reliable) current!!.used[UsageCounter.TOTAL] ?: 0 else null
    val downtime = local.downtime.any { Policy.inWindow(it, now) }
    val kind = when {
        downtime -> TodayKind.Downtime
        local.dailyLimitMillis != null && total != null && total >= local.dailyLimitMillis -> TodayKind.TimesUp
        !reliable -> TodayKind.Unknown
        local.dailyLimitMillis != null && local.dailyLimitMillis - total!! <= AppControl.WARN_BEFORE_MS -> TodayKind.NearLimit
        else -> TodayKind.Allowed
    }
    val rows = (local.limitsMillis.keys + local.blocked + local.alwaysAllowed)
        .filter { it !in matching?.essential.orEmpty() }.map { pkg ->
        AppTimeRow(pkg, matching?.labels?.get(pkg) ?: pkg,
            if (reliable) current!!.used[pkg] ?: 0 else null,
            local.limitsMillis[pkg], pkg in local.blocked, pkg in local.alwaysAllowed && pkg !in local.blocked)
    }.sortedWith(compareBy({ it.label.lowercase() }, { it.pkg }))
    return ScreenTimeState(kind, total, local.dailyLimitMillis, nextDowntimeChange(local.downtime, now),
        now.toLocalDate().plusDays(1).atStartOfDay(now.zone), rows,
        rows.filter { it.unlimited }.map { it.label }, matching?.historyTotals.orEmpty().filterKeys {
            val date = LocalDate.parse(it)
            date <= now.toLocalDate() && date > now.toLocalDate().minusDays(30)
        }, !usageAccess)
}

/** Skip overlapping window boundaries that do not actually change downtime. */
fun nextDowntimeChange(windows: List<Window>, now: ZonedDateTime): ZonedDateTime? {
    val blocked = windows.any { Policy.inWindow(it, now) }
    return (0L..8).flatMap { offset ->
        val date = now.toLocalDate().plusDays(offset)
        listOf(date.atStartOfDay(now.zone)) + windows.flatMap { w ->
            listOf(date.atTime(w.start).atZone(now.zone), date.atTime(w.stop).atZone(now.zone))
        }
    }.filter { it.isAfter(now) && windows.any { w -> Policy.inWindow(w, it) } != blocked }.minOrNull()
}
