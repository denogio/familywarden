package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastThemeExt
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

@Composable
internal fun ScreenTimeContent(state: FamilyWardenState, actions: FamilyWardenActions) {
    val time = state.screenTime ?: return
    val night = MastThemeExt.colors
    NightCard {
        val title = when (time.kind) {
            TodayKind.Downtime -> R.string.fw_time_downtime
            TodayKind.TimesUp -> R.string.fw_time_up
            TodayKind.NearLimit -> R.string.fw_time_almost
            else -> if (time.limitMillis != null) R.string.fw_time_left else R.string.fw_time_recorded
        }
        Text(stringResource(title), style = MaterialTheme.typography.labelLarge, color = night.nightAccent)
        Spacer(Modifier.height(12.dp))
        HeroTitle(when (time.kind) {
            TodayKind.Downtime -> stringResource(R.string.fw_time_downtime_title)
            TodayKind.TimesUp -> stringResource(R.string.fw_time_up_title)
            else -> (time.remainingMillis ?: time.usedMillis)?.let { minutes(it, time.limitMillis != null) } ?: "—"
        }, night.onNight)
        val used = time.usedMillis
        val limit = time.limitMillis
        if (time.kind == TodayKind.TimesUp || time.kind == TodayKind.Downtime) {
            HeroBody(stringResource(R.string.fw_time_still_work), night.onNightMuted)
        } else if (used == null) {
            HeroBody(stringResource(R.string.fw_time_not_counted), night.onNightMuted)
        } else if (limit != null) {
            Spacer(Modifier.height(12.dp))
            TimeProgress(used, limit, Modifier.fillMaxWidth(), night.nightAccent, night.nightDivider)
            HeroBody(stringResource(R.string.fw_time_used_of, minutes(used), minutes(limit)), night.onNightMuted)
        } else HeroBody(stringResource(R.string.fw_time_no_daily_limit), night.onNightMuted)
        if (time.kind == TodayKind.NearLimit) HeroBody(stringResource(R.string.fw_time_save_work), night.onNightMuted)
        time.nextDowntimeChange?.let { change ->
            HeroBody(stringResource(if (time.kind == TodayKind.Downtime) R.string.fw_time_downtime_ends
                else R.string.fw_time_downtime_starts, localTime(change)), night.onNightMuted)
        }
        if (time.kind == TodayKind.TimesUp) HeroBody(stringResource(R.string.fw_time_reset,
            localTime(time.resetsAt)), night.onNightMuted)
    }
    Text(stringResource(R.string.fw_time_this_phone), style = MaterialTheme.typography.bodySmall)
    if (state.syncError != null) InfoNote(Icons.Outlined.Info, stringResource(when (state.syncError) {
        "router unreachable" -> R.string.fw_time_offline
        "unsupported capabilities" -> R.string.fw_sync_upgrade_required
        else -> R.string.fw_sync_failed
    }))
    if (time.usedMillis == null) {
        InfoNote(Icons.Outlined.Info, stringResource(if (time.usageAccessMissing)
            R.string.fw_apps_usage_missing else R.string.fw_time_unavailable))
        if (time.usageAccessMissing) TextButton(onClick = actions.openUsageAccess,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
            Text(stringResource(R.string.fw_apps_open_usage))
        }
    }
    if (state.appsStatus == "suspend_failed") InfoNote(Icons.Outlined.WarningAmber,
        stringResource(R.string.fw_apps_suspend_failed))
    if (state.binding?.captain == true && state.showUlysses) StatusHero(state.pact ?: PactStatus.FreeSailing, state.nowMillis)
    val limited = time.apps.filter { !it.unlimited }
    if (limited.isNotEmpty() && time.kind !in setOf(TodayKind.TimesUp, TodayKind.Downtime)) SectionCard {
        Overline(stringResource(R.string.fw_time_apps))
        limited.forEachIndexed { index, app ->
            if (index > 0) HorizontalDivider(Modifier.padding(vertical = 12.dp))
            Column(Modifier.fillMaxWidth().semantics(mergeDescendants = true) { }, verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(app.label, style = MaterialTheme.typography.titleMedium)
                Text(when {
                    app.blocked -> stringResource(R.string.fw_time_not_allowed)
                    app.usedMillis == null -> stringResource(R.string.fw_time_not_counted)
                    app.limitMillis != null -> stringResource(R.string.fw_time_used_of, minutes(app.usedMillis), minutes(app.limitMillis))
                    else -> stringResource(R.string.fw_time_no_app_limit)
                }, style = MaterialTheme.typography.bodyMedium)
                if (!app.blocked && app.usedMillis != null && app.limitMillis != null)
                    TimeProgress(app.usedMillis, app.limitMillis, Modifier.fillMaxWidth())
            }
        }
    }
    SectionCard {
        Overline(stringResource(R.string.fw_time_always))
        Text(stringResource(R.string.fw_time_calls_clock))
        time.alwaysAllowed.forEach { Text(it, modifier = Modifier.padding(top = 8.dp)) }
        Text(stringResource(R.string.fw_time_always_body), style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.padding(top = 8.dp))
    }
}

/** Local recorded totals, not router upload, browsing history, or a claim of complete coverage. */
@Composable
internal fun ScreenTimeHistory(time: ScreenTimeState?) {
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_history_local))
    val days = time?.historyTotals.orEmpty().toSortedMap(reverseOrder())
    if (days.isEmpty()) Text(stringResource(R.string.fw_history_empty))
    else days.forEach { (day, millis) -> SectionCard {
        Text(java.time.LocalDate.parse(day).format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
            .withLocale(LocalConfiguration.current.locales[0])), style = MaterialTheme.typography.titleMedium)
        Text(minutes(millis), style = MaterialTheme.typography.headlineSmall)
    } }
}

@Composable
private fun minutes(millis: Long, roundUp: Boolean = false): String {
    val value = ((millis + if (roundUp) 59_999 else 0) / 60_000).toInt()
    return pluralStringResource(R.plurals.fw_time_minutes, value, value)
}

@Composable
private fun localTime(at: ZonedDateTime): String = at.format(DateTimeFormatter
    .ofLocalizedDateTime(FormatStyle.SHORT).withLocale(LocalConfiguration.current.locales[0]))

@Composable
private fun TimeProgress(used: Long, limit: Long, modifier: Modifier = Modifier,
    color: androidx.compose.ui.graphics.Color = MaterialTheme.colorScheme.secondary,
    track: androidx.compose.ui.graphics.Color = MaterialTheme.colorScheme.surfaceContainerHigh) {
    LinearProgressIndicator(progress = { if (limit == 0L) 1f else (used.toFloat() / limit).coerceIn(0f, 1f) },
        modifier = modifier.clearAndSetSemantics { }, color = color, trackColor = track)
}
