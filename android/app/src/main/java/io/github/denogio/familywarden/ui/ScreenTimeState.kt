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
    val grantRemaining: Map<String, Long> = emptyMap(),
    val grantReliable: Boolean = false,
    val installed: Set<String> = emptySet(),
    /** Included in [used]: today's total on the profile's other phones (router-reported). */
    val otherDevicesMillis: Long = 0,
    /** Local per-day app counters on this phone (bounded, busiest first). */
    val historyApps: Map<String, Map<String, Long>> = emptyMap(),
)

/** One app's recorded time on one day, on this phone. */
data class DayApp(val pkg: String, val label: String, val millis: Long)

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
    val grantRemaining: Map<String, Long> = emptyMap(),
    val otherDevicesMillis: Long = 0,
    val historyApps: Map<String, List<DayApp>> = emptyMap(),
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
    fun recent(day: String) = runCatching { LocalDate.parse(day) }.getOrNull()
        ?.let { it <= now.toLocalDate() && it > now.toLocalDate().minusDays(30) } == true
    return ScreenTimeState(kind, total, local.dailyLimitMillis, nextDowntimeChange(local.downtime, now),
        now.toLocalDate().plusDays(1).atStartOfDay(now.zone), rows,
        rows.filter { it.unlimited }.map { it.label }, matching?.historyTotals.orEmpty().filterKeys(::recent),
        !usageAccess, if (current?.grantReliable == true && usageAccess) current.grantRemaining else emptyMap(),
        if (reliable) current!!.otherDevicesMillis else 0,
        matching?.historyApps.orEmpty().filterKeys(::recent).mapValues { (_, apps) ->
            apps.filterKeys { it !in matching!!.essential }.map { (pkg, millis) -> DayApp(pkg, matching!!.labels[pkg] ?: pkg, millis) }
                .sortedByDescending { it.millis }
        }.filterValues { it.isNotEmpty() })
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
