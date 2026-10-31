package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ChevronRight
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastThemeExt
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/** What a scheduled pause restricts. Hotspot only ever comes from an active pact. */
enum class Rests { Apps, Hotspot, AppsAndHotspot }

/** One statement "[what] rests until [until]"; [until] is null when no end is scheduled. */
data class RestLine(val what: Rests, val until: ZonedDateTime?)

/** A pause that starts after [AdultSchedule.openUntil]: "[what] rests from [from] until [until]". */
data class LaterPause(val what: Rests, val from: ZonedDateTime, val until: ZonedDateTime?)

/**
 * The adult hero's facts, from the signed schedules only (wall clock), never from measurement.
 * [openUntil] is the earliest coming pause of something open now; [then] says what pauses at
 * exactly that instant; [later] names pauses that start afterwards, so none disappears;
 * [restingNow] is what already rests.
 */
data class AdultSchedule(val openUntil: ZonedDateTime?, val then: List<RestLine>, val restingNow: List<RestLine>,
    val later: List<LaterPause> = emptyList())

/** "Apps and hotspot" is only claimed for restrictions that start (or end) at the same instant. */
fun adultSchedule(time: ScreenTimeState?, pact: PactStatus?, zone: ZoneId): AdultSchedule {
    class Item(val what: Rests, val resting: Boolean, val start: ZonedDateTime?, val end: ZonedDateTime?)
    fun at(millis: Long?) = millis?.let { Instant.ofEpochMilli(it).atZone(zone) }
    val items = buildList {
        if (time != null) {
            val resting = time.kind == TodayKind.Downtime
            add(Item(Rests.Apps, resting, time.nextDowntimeChange.takeIf { !resting },
                if (resting) time.nextDowntimeChange else time.followingDowntimeChange))
        }
        if (pact is PactStatus.Bound) {
            // While allowed, the next change is always a lock: pact end is never reported as one.
            add(Item(Rests.Hotspot, pact.blocked, at(pact.nextChangeMillis).takeIf { !pact.blocked },
                if (pact.blocked) at(pact.nextChangeMillis) else at(pact.followingChangeMillis)))
        }
    }
    fun lines(group: List<Item>) = group.groupBy { it.end?.toInstant() }.map { (_, same) ->
        RestLine(if (same.size > 1) Rests.AppsAndHotspot else same.single().what, same.first().end)
    }
    val upcoming = items.filter { !it.resting && it.start != null }
    val first = upcoming.minOfOrNull { it.start!!.toInstant() }
    return AdultSchedule(first?.atZone(zone), lines(upcoming.filter { it.start!!.toInstant() == first }),
        lines(items.filter { it.resting }),
        upcoming.filter { it.start!!.toInstant() != first }.map { LaterPause(it.what, it.start!!, it.end) })
}

/** Adult (captain) overview: hero "Open until", the compact pact card, then "Running low". */
@Composable
internal fun AdultOverview(state: FamilyWardenState, actions: FamilyWardenActions, onAsk: () -> Unit, onOpenPact: (() -> Unit)?) {
    val time = state.screenTime
    val bound = state.pact as? PactStatus.Bound
    if (time != null) OverviewNotices(state, actions, time)
    when {
        time != null && (time.kind == TodayKind.TimesUp || time.kind == TodayKind.NearLimit || time.kind == TodayKind.Unknown) ->
            TodayHero(state, time)
        time != null || bound != null -> AdultHero(state, time, bound)
        else -> DnsHero(state)
    }
    if (bound != null) CompactPactCard(state, bound, onOpenPact)
    if (time != null) {
        AskAction(state, time, onAsk)
        TodayDetails(state, actions, time)
        RunningLow(time)
    } else {
        SyncProblemNote(state)
        AppLimitsProblems(state, actions)
    }
    if ((time != null || bound != null) && state.headline !in calmDns) DnsHero(state)
}

/** DNS states that need no card on the overview; the DNS page has the details. */
private val calmDns = setOf(DnsHeadline.AppliedAndTested, DnsHeadline.AppliedUntested, DnsHeadline.Testing, DnsHeadline.NotConfigured)

@Composable
private fun AdultHero(state: FamilyWardenState, time: ScreenTimeState?, bound: PactStatus.Bound?) {
    val zone = time?.resetsAt?.zone ?: routerZone(state)
    val now = Instant.ofEpochMilli(state.nowMillis).atZone(zone)
    val schedule = adultSchedule(time, bound, zone)
    val appsOpen = time != null && time.kind != TodayKind.Downtime
    val night = MastThemeExt.colors
    NightCard {
        val label: String
        val title: String
        val body = mutableListOf<String>()
        // Resting apps (or a hotspot-only pact that rests) answer first; "open until" never hides a rest.
        val resting = schedule.restingNow.isNotEmpty() &&
            ((time != null && time.kind == TodayKind.Downtime) || (time == null && bound?.blocked == true))
        when {
            resting -> {
                label = stringResource(R.string.fw_adult_resting)
                title = restText(schedule.restingNow.first(), now, then = false)
                schedule.restingNow.drop(1).forEach { body += restText(it, now, then = false) }
            }
            schedule.openUntil != null -> {
                label = stringResource(R.string.fw_adult_open_until, whenText(schedule.openUntil, now))
                title = durationText(java.time.Duration.between(now, schedule.openUntil).toMillis().coerceAtLeast(0))
                schedule.then.forEach { body += restText(it, now, then = true) }
                schedule.later.forEach { body += laterText(it, now) }
                schedule.restingNow.forEach { body += restText(it, now, then = false) }
            }
            else -> {
                label = stringResource(R.string.fw_adult_open)
                title = stringResource(R.string.fw_adult_no_pause)
                schedule.restingNow.forEach { body += restText(it, now, then = false) }
            }
        }
        time?.remainingMillis?.takeIf { appsOpen }?.let { body += stringResource(R.string.fw_adult_left_today, durationText(it, roundUp = true)) }
        Text(label, style = MaterialTheme.typography.labelMedium.copy(letterSpacing = 1.2.sp), color = night.nightAccent)
        Spacer(Modifier.height(8.dp))
        HeroTitle(title, night.onNight)
        body.forEach { HeroBody(it, night.onNightMuted) }
    }
}

@Composable
private fun restText(line: RestLine, now: ZonedDateTime, then: Boolean): String {
    val until = line.until ?: return stringResource(when (line.what) {
        Rests.Apps -> if (then) R.string.fw_adult_then_apps_no_end else R.string.fw_adult_rest_apps_no_end
        Rests.Hotspot -> if (then) R.string.fw_adult_then_hotspot_no_end else R.string.fw_adult_rest_hotspot_no_end
        Rests.AppsAndHotspot -> if (then) R.string.fw_adult_then_both_no_end else R.string.fw_adult_rest_both_no_end
    })
    return stringResource(when (line.what) {
        Rests.Apps -> if (then) R.string.fw_adult_then_apps else R.string.fw_adult_rest_apps
        Rests.Hotspot -> if (then) R.string.fw_adult_then_hotspot else R.string.fw_adult_rest_hotspot
        Rests.AppsAndHotspot -> if (then) R.string.fw_adult_then_both else R.string.fw_adult_rest_both
    }, whenText(until, now))
}

@Composable
private fun laterText(pause: LaterPause, now: ZonedDateTime): String {
    val from = whenText(pause.from, now)
    val until = pause.until ?: return stringResource(if (pause.what == Rests.Hotspot)
        R.string.fw_adult_later_hotspot_no_end else R.string.fw_adult_later_apps_no_end, from)
    return stringResource(if (pause.what == Rests.Hotspot) R.string.fw_adult_later_hotspot else R.string.fw_adult_later_apps,
        from, whenText(until, now))
}

/** Compact pact card: the pact, then the hotspot state right now; opens the Ulysses page. */
@Composable
private fun CompactPactCard(state: FamilyWardenState, status: PactStatus.Bound, onOpen: (() -> Unit)?) {
    val night = MastThemeExt.colors
    val zone = state.screenTime?.resetsAt?.zone ?: routerZone(state)
    val now = Instant.ofEpochMilli(state.nowMillis).atZone(zone)
    val title = status.pact.untilMillis?.let { stringResource(R.string.fw_pact_card_until, whenText(Instant.ofEpochMilli(it).atZone(zone), now)) }
        ?: stringResource(R.string.fw_pact_card_ongoing)
    // Not one merged button: each line stays its own TalkBack/UiAutomator text.
    Surface(modifier = Modifier.fillMaxWidth(), shape = MaterialTheme.shapes.large, color = night.nightTop) {
        Row(Modifier.padding(start = 18.dp, end = 4.dp, top = 12.dp, bottom = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(MastIcons.Mast, null, tint = night.nightAccent, modifier = Modifier.size(24.dp))
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.titleMedium, color = night.onNight)
                Text(stringResource(if (status.blocked) R.string.status_locked_title else R.string.status_allowed_title),
                    style = MaterialTheme.typography.bodySmall, color = night.onNightMuted)
                if (status.pact.releaseAtMillis != null) Text(stringResource(R.string.pact_release_requested),
                    style = MaterialTheme.typography.bodySmall, color = night.onNightMuted)
            }
            if (onOpen != null) IconButton(onClick = onOpen) {
                Icon(Icons.Outlined.ChevronRight, stringResource(R.string.fw_open_ulysses), tint = night.onNight)
            }
        }
    }
}

/** App rules without a screen-time view: only problems need attention. */
@Composable
private fun AppLimitsProblems(s: FamilyWardenState, actions: FamilyWardenActions) {
    if (!s.appsActive) return
    when (s.appsStatus) {
        "usage_access_missing" -> {
            InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_usage_missing))
            TextButton(onClick = actions.openUsageAccess, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_apps_open_usage))
            }
        }
        "suspend_failed" -> InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_suspend_failed))
        "ok" -> Unit
        else -> InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_time_unavailable))
    }
}

internal fun routerZone(state: FamilyWardenState): ZoneId =
    state.timeZone?.let { runCatching { ZoneId.of(it) }.getOrNull() } ?: ZoneId.systemDefault()
