package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastThemeExt
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.time.format.TextStyle

/** Verified voyage facts and current enforcement are deliberately separate. */
@Composable
internal fun VoyageContent(s: FamilyWardenState, actions: FamilyWardenActions) {
    val bound = s.pact as? PactStatus.Bound
    val night = MastThemeExt.colors
    if (s.syncError != null) InfoNote(if (s.syncError == "router unreachable") Icons.Outlined.CloudOff else Icons.Outlined.WarningAmber,
        stringResource(if (s.syncError == "router unreachable") R.string.fw_voyage_offline else R.string.fw_sync_failed))
    if (s.binding == null || s.pact == null) {
        SectionCard {
            Text(stringResource(R.string.fw_voyage_unknown_title), style = MaterialTheme.typography.titleLarge)
            Text(stringResource(R.string.fw_voyage_unknown_body))
            TextButton(onClick = actions.sync, enabled = !s.syncing) { Text(stringResource(R.string.action_sync)) }
        }
        return
    }
    if (bound == null) HeroCard {
        HeroBadge(MastIcons.Mast, MaterialTheme.colorScheme.secondaryContainer, MaterialTheme.colorScheme.secondary)
        Overline(stringResource(R.string.fw_voyage_not_bound))
        HeroTitle(stringResource(if (s.pact == PactStatus.NoCaptain) R.string.status_no_captain_title else R.string.fw_voyage_calm))
        HeroBody(stringResource(if (s.pact == PactStatus.NoCaptain) R.string.fw_voyage_not_captain else R.string.fw_voyage_router_only))
    } else NightCard {
        HeroBadge(MastIcons.Mast, night.nightAccent.copy(alpha = .18f), night.nightAccent)
        Overline(stringResource(if (bound.pact.releaseAtMillis != null) R.string.status_sailing_home_overline else R.string.fw_voyage_bound), night.nightAccent)
        HeroTitle(bound.pact.untilMillis?.let { stringResource(R.string.pact_until, voyageTime(s, it)) }
            ?: stringResource(R.string.pact_ongoing), night.onNight)
        bound.pact.releaseAtMillis?.let { at ->
            HeroBody(stringResource(R.string.fw_voyage_release_expected, voyageTime(s, at)), night.onNight)
            HeroBody(stringResource(R.string.fw_voyage_holds), night.onNightMuted)
        }
        s.voyageDetails?.waitingSeconds?.let { seconds ->
            HeroBody(stringResource(R.string.fw_voyage_wait, formatDuration(seconds * 1000)), night.onNightMuted)
        }
        if (s.lastSync > 0) HeroBody(stringResource(R.string.fw_voyage_verified, voyageTime(s, s.lastSync * 1000)), night.onNightMuted)
    }
    SectionCard {
        Overline(stringResource(R.string.fw_voyage_now))
        Text(stringResource(if (bound?.blocked == true) R.string.status_locked_title else R.string.status_allowed_title),
            style = MaterialTheme.typography.titleMedium)
        bound?.nextChangeMillis?.let { Text(stringResource(R.string.fw_voyage_next, voyageTime(s, it)), style = MaterialTheme.typography.bodySmall) }
        s.screenTime?.let { time ->
            Text(stringResource(when (time.kind) {
                TodayKind.Downtime -> R.string.fw_voyage_apps_downtime
                TodayKind.TimesUp -> R.string.fw_voyage_apps_exhausted
                TodayKind.Unknown -> R.string.fw_time_unavailable
                else -> R.string.fw_voyage_apps_within
            }))
            time.remainingMillis?.let { Text(stringResource(R.string.fw_voyage_remaining, durationText(it))) }
        }
        if (s.appsStatus == "suspend_failed") Text(stringResource(R.string.fw_apps_suspend_failed))
        if (s.screenTime?.usageAccessMissing == true) TextButton(onClick = actions.openUsageAccess) {
            Text(stringResource(R.string.fw_apps_open_usage))
        }
        Text(stringResource(R.string.fw_voyage_now_note), style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    if (bound != null) {
        SectionCard {
            Overline(stringResource(R.string.fw_voyage_course))
            Text(stringResource(R.string.fw_voyage_frozen_note))
            if (s.screenTime != null) Text(stringResource(R.string.fw_voyage_delivered), style = MaterialTheme.typography.bodySmall)
            s.screenTime?.apps?.forEach { app ->
                Text(app.label + " · " + when {
                    app.blocked -> stringResource(R.string.fw_time_not_allowed)
                    app.unlimited -> stringResource(R.string.fw_time_always)
                    app.limitMillis != null -> durationText(app.limitMillis)
                    else -> stringResource(R.string.fw_time_no_app_limit)
                }, style = MaterialTheme.typography.bodyMedium)
            }
            s.rules?.let { rules ->
                Text(stringResource(R.string.fw_schedule_today), style = MaterialTheme.typography.titleMedium)
                val zone = ZoneId.of(s.timeZone ?: ZoneId.systemDefault().id)
                val day = Instant.ofEpochMilli(s.nowMillis).atZone(zone).toLocalDate()
                val intervals = daySchedule(rules, day, zone)
                Text(if (intervals.isEmpty()) stringResource(R.string.fw_schedule_none) else intervals.joinToString(" · ") {
                    fun clock(n: Int) = "%02d:%02d".format(n / 60, n % 60)
                    clock(it.startMinute) + "–" + clock(it.endExclusive)
                })
            }
            s.desiredDns?.let { Text(stringResource(R.string.fw_dns) + " · " + it.hostname) }
            Text(stringResource(R.string.pact_locked_note), style = MaterialTheme.typography.bodySmall)
        }
        s.voyageDetails?.amendment?.let { window ->
            SectionCard {
                Overline(stringResource(R.string.fw_voyage_port))
                val locale = LocalConfiguration.current.locales[0]
                val end = if (window.stop == java.time.LocalTime.MIDNIGHT) "24:00" else window.stop.toString()
                Text(window.days.single().getDisplayName(TextStyle.FULL, locale) + " · " + window.start + "–" + end)
                Text(stringResource(R.string.fw_voyage_port_note), style = MaterialTheme.typography.bodySmall)
            }
        }
        if (s.voyageDetails?.waitingSeconds == null || (bound.pact.untilMillis == null && s.voyageDetails.amendment == null))
            InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_voyage_metadata_unknown))
    }
    Button(onClick = actions.openRouter, enabled = s.host != null, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
        Icon(Icons.Outlined.OpenInBrowser, null)
        Spacer(Modifier.width(8.dp))
        Text(stringResource(R.string.fw_voyage_open_router))
    }
    Text(stringResource(R.string.fw_voyage_router_only), style = MaterialTheme.typography.bodySmall)
    if (bound != null) {
        OutlinedButton(onClick = actions.openSail, enabled = s.host != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(R.string.fw_open_sail)) }
        InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_sail_note))
    }
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_timezone, s.timeZone ?: ZoneId.systemDefault().id))
}

@Composable
private fun voyageTime(s: FamilyWardenState, millis: Long): String = Instant.ofEpochMilli(millis)
    .atZone(ZoneId.of(s.timeZone ?: ZoneId.systemDefault().id))
    .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
        .withLocale(LocalConfiguration.current.locales[0]))
