package io.github.denogio.familywarden.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastPalette
import io.github.denogio.familywarden.ui.theme.MastThemeExt
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun ScreenTimeContent(state: FamilyWardenState, actions: FamilyWardenActions, onAsk: () -> Unit = {}) {
    val time = state.screenTime ?: return
    var forget by rememberSaveable { mutableStateOf(false) }
    if (state.uploadPending) SectionCard {
        Text(stringResource(R.string.fw_request_retry_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_request_retry_body))
        Button(onClick = actions.retryUpload, enabled = !state.syncing, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.fw_request_retry))
        }
        TextButton(onClick = { forget = true }, enabled = !state.syncing) { Text(stringResource(R.string.fw_request_forget)) }
    }
    if (forget) AlertDialog(onDismissRequest = { forget = false },
        title = { Text(stringResource(R.string.fw_request_forget)) }, text = { Text(stringResource(R.string.fw_request_forget_body)) },
        confirmButton = { TextButton(onClick = { forget = false; actions.forgetUpload() }) { Text(stringResource(R.string.fw_request_forget)) } },
        dismissButton = { TextButton(onClick = { forget = false }) { Text(stringResource(R.string.action_cancel)) } })
    state.uploadReply?.takeIf { it.error != null }?.let { InfoNote(Icons.Outlined.WarningAmber, stringResource(R.string.fw_request_action_rejected)) }
    ChildRequestCard(state, time.resetsAt.zone)
    if (state.waitingApps.isNotEmpty()) SectionCard {
        Overline(stringResource(R.string.fw_app_waiting_title))
        state.waitingApps.forEach { Text(it, style = MaterialTheme.typography.titleMedium) }
        Text(stringResource(R.string.fw_app_waiting_body), style = MaterialTheme.typography.bodySmall)
    }
    val night = MastThemeExt.colors
    val now = java.time.Instant.ofEpochMilli(state.nowMillis).atZone(time.resetsAt.zone)
    val extra = time.grantRemaining.filter { (target, amount) -> amount > 0 &&
        (target == "total" || (target in state.requestTargets.map { it.pkg })) }
    val calm = time.kind == TodayKind.TimesUp || time.kind == TodayKind.Downtime
    val phone = stringResource(R.string.fw_time_phone_tile)
    val stillWorks = listOf(PHONE_TILE to phone) + time.apps.filter { it.unlimited }.map { it.pkg to it.label }
    val stillWorksDescriptions = mapOf(PHONE_TILE to stringResource(R.string.fw_time_calls_clock))
    NightCard {
        HeroIcon(if (time.kind == TodayKind.Downtime) Icons.Outlined.Bedtime else Icons.Outlined.HourglassEmpty)
        val label = when (time.kind) {
            TodayKind.Downtime -> R.string.fw_time_downtime
            TodayKind.TimesUp -> R.string.fw_time_up
            TodayKind.NearLimit -> R.string.fw_time_almost
            else -> if (time.limitMillis != null) R.string.fw_time_left else R.string.fw_time_recorded
        }
        Text(stringResource(if (extra.isNotEmpty()) R.string.fw_request_approved else label),
            style = MaterialTheme.typography.labelMedium.copy(letterSpacing = 1.2.sp), color = night.nightAccent)
        Spacer(Modifier.height(8.dp))
        HeroTitle(if (extra.isNotEmpty()) stringResource(R.string.fw_request_extra_title) else when (time.kind) {
            TodayKind.Downtime -> time.nextDowntimeChange?.let { stringResource(R.string.fw_time_downtime_until, whenText(it, now)) }
                ?: stringResource(R.string.fw_time_downtime_title)
            TodayKind.TimesUp -> stringResource(R.string.fw_time_up_title)
            else -> (time.remainingMillis ?: time.usedMillis)?.let { durationText(it, time.limitMillis != null) } ?: "—"
        }, night.onNight)
        val used = time.usedMillis
        val limit = time.limitMillis
        if (extra.isEmpty() && !calm && used != null && limit != null) {
            Spacer(Modifier.height(12.dp))
            TimeProgress(used, limit, Modifier.fillMaxWidth(), night.nightAccent, night.nightDivider)
        }
        val next = time.nextDowntimeChange?.takeIf { !calm }?.let { stringResource(R.string.fw_time_downtime_starts, whenText(it, now)) }
        when {
            extra.isNotEmpty() -> HeroBody(stringResource(R.string.fw_request_extra_body), night.onNightMuted)
            time.kind == TodayKind.TimesUp -> HeroBody(stringResource(R.string.fw_time_reset, whenText(time.resetsAt, now)), night.onNightMuted)
            time.kind == TodayKind.Downtime -> HeroBody(stringResource(R.string.fw_time_still_work), night.onNightMuted)
            used == null -> HeroBody(listOfNotNull(stringResource(R.string.fw_time_not_counted), next).joinToString(" · "), night.onNightMuted)
            limit != null -> HeroBody(listOfNotNull(stringResource(R.string.fw_time_used_of, durationText(used), durationText(limit)), next)
                .joinToString(" · "), night.onNightMuted)
            else -> HeroBody(listOfNotNull(stringResource(R.string.fw_time_no_daily_limit), next).joinToString(" · "), night.onNightMuted)
        }
        if (time.kind == TodayKind.NearLimit && extra.isEmpty()) HeroBody(stringResource(R.string.fw_time_save_work), night.onNightMuted)
        if (calm && extra.isEmpty()) {
            // Calm full status, not a lock screen: what still works stays in view.
            Spacer(Modifier.height(16.dp))
            val initials = io.github.denogio.familywarden.ui.components.tileInitials(stillWorks.map { it.second })
            Column(Modifier.fillMaxWidth().background(night.onNight.copy(alpha = 0.06f), MaterialTheme.shapes.large)
                .padding(horizontal = 14.dp, vertical = 6.dp)) {
                stillWorks.forEachIndexed { index, (key, name) ->
                    if (index > 0) HorizontalDivider(color = night.nightDivider)
                    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp).semantics(mergeDescendants = true) {
                        contentDescription = stillWorksDescriptions[key] ?: name
                    }, verticalAlignment = Alignment.CenterVertically) {
                        AppTile(key, initials.getValue(name), 32.dp)
                        Spacer(Modifier.width(12.dp))
                        Text(stillWorksDescriptions[key] ?: name, style = MaterialTheme.typography.bodyMedium, color = night.onNight)
                    }
                }
            }
        }
    }
    Text(if (time.otherDevicesMillis >= 60_000) stringResource(R.string.fw_time_other_devices, minutes(time.otherDevicesMillis))
        else stringResource(R.string.fw_time_this_phone), style = MaterialTheme.typography.bodySmall)
    // The request card already shows the remaining extra time of its own grant.
    val otherExtra = extra.filterKeys { target -> state.childRequest?.let { it.state == "approved" && it.target == target } != true }
    if (otherExtra.isNotEmpty()) SectionCard {
        otherExtra.forEach { (target, amount) ->
            Text(if (target == "total") stringResource(R.string.fw_request_screen)
                else state.requestTargets.firstOrNull { it.pkg == target }?.label ?: target,
                style = MaterialTheme.typography.titleMedium)
            val min = ((amount + 59_999) / 60_000).toInt()
            Text(pluralStringResource(R.plurals.fw_request_remaining, min, min))
        }
    }
    if (!state.bound) {
        // An approved grant also works in downtime, so asking stays available there.
        val open = state.askOpen()
        val canAsk = !open && !state.syncing && !state.uploadPending
        val content: @Composable RowScope.() -> Unit = {
            Icon(Icons.Outlined.Add, null, Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.fw_request_ask))
        }
        if (time.kind == TodayKind.TimesUp) {
            Button(onClick = onAsk, enabled = canAsk, modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp),
                colors = ButtonDefaults.buttonColors(containerColor = MastPalette.GoldBright, contentColor = MastPalette.GoldInk),
                content = content)
            Text(stringResource(R.string.fw_ask_who), style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.fillMaxWidth(), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
        } else FilledTonalButton(onClick = onAsk, enabled = canAsk, modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp), content = content)
        if (open) Text(stringResource(R.string.fw_req_one_open), style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.fillMaxWidth(), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
    }
    if (state.syncError != null) InfoNote(Icons.Outlined.Info, stringResource(when (state.syncError) {
        "router unreachable" -> R.string.fw_time_offline
        "unsupported capabilities" -> R.string.fw_sync_upgrade_required
        else -> R.string.fw_sync_failed
    }))
    if (time.usedMillis == null) {
        if (time.usageAccessMissing) {
            Banner(Icons.Outlined.WarningAmber, stringResource(R.string.fw_time_uncounted_title),
                stringResource(R.string.fw_time_uncounted_body), warning = true)
            TextButton(onClick = actions.openUsageAccess, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text(stringResource(R.string.fw_apps_open_usage))
            }
        } else InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_time_unavailable))
    }
    if (state.appsStatus == "suspend_failed") InfoNote(Icons.Outlined.WarningAmber,
        stringResource(R.string.fw_apps_suspend_failed))
    if (state.binding?.captain == true && state.showUlysses) StatusHero(state.pact ?: PactStatus.FreeSailing, state.nowMillis)
    val limited = time.apps.filter { !it.unlimited }
    if (limited.isNotEmpty() && !calm) SectionCard {
        Overline(stringResource(R.string.fw_time_apps))
        val initials = io.github.denogio.familywarden.ui.components.tileInitials(limited.map { it.label })
        limited.forEachIndexed { index, app ->
            if (index > 0) Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth().semantics(mergeDescendants = true) { }, verticalAlignment = Alignment.Top) {
                AppTile(app.pkg, initials.getValue(app.label))
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                        Text(app.label, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(end = 8.dp))
                        Text(when {
                            app.blocked -> stringResource(R.string.fw_time_not_allowed)
                            app.usedMillis == null -> stringResource(R.string.fw_time_not_counted_short)
                            app.limitMillis != null -> stringResource(R.string.fw_time_row_of, durationText(app.usedMillis), durationText(app.limitMillis))
                            else -> stringResource(R.string.fw_time_no_app_limit)
                        }, style = MaterialTheme.typography.labelLarge)
                    }
                    if (!app.blocked && app.limitMillis != null)
                        TimeProgress(app.usedMillis ?: 0, app.limitMillis, Modifier.fillMaxWidth(),
                            if (app.usedMillis == null) MaterialTheme.colorScheme.surfaceContainerHigh else MaterialTheme.colorScheme.secondary)
                }
            }
        }
    }
    if (!calm) SectionCard {
        Overline(stringResource(R.string.fw_time_always))
        AppTileGrid(stillWorks, stillWorksDescriptions)
    }
}

private const val PHONE_TILE = "android.phone-and-clock"

/** Small gold-tinted badge at the top of the night card. */
@Composable
private fun HeroIcon(icon: androidx.compose.ui.graphics.vector.ImageVector) {
    val night = MastThemeExt.colors
    Box(Modifier.size(32.dp).background(night.nightAccent.copy(alpha = 0.18f), CircleShape).clearAndSetSemantics { },
        contentAlignment = Alignment.Center) {
        Icon(icon, null, tint = night.nightAccent, modifier = Modifier.size(18.dp))
    }
    Spacer(Modifier.height(12.dp))
}

/** A time today reads as "20:00"; other days add the short weekday ("Wed 07:00"). */
@Composable
private fun whenText(at: ZonedDateTime, now: ZonedDateTime): String {
    val locale = LocalConfiguration.current.locales[0]
    val time = at.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT).withLocale(locale))
    return if (at.toLocalDate() == now.toLocalDate()) time
    else at.format(DateTimeFormatter.ofPattern("EEE", locale)) + " " + time
}

/** Local recorded totals per day and app on this phone: not router upload, not browsing history. */
@Composable
internal fun ScreenTimeHistory(time: ScreenTimeState?) {
    val days = (time?.historyTotals.orEmpty().keys + time?.historyApps.orEmpty().keys).sortedDescending()
    if (time == null || days.isEmpty()) {
        Text(stringResource(R.string.fw_history_empty))
        InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_history_local))
        return
    }
    var selected by rememberSaveable { mutableStateOf(days.first()) }
    val day = selected.takeIf { it in days } ?: days.first()
    val locale = LocalConfiguration.current.locales[0]
    val today = time.resetsAt.toLocalDate().minusDays(1)
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        days.forEach { key ->
            val date = java.time.LocalDate.parse(key)
            FilterChip(selected = key == day, onClick = { selected = key }, modifier = Modifier.heightIn(min = 48.dp), label = {
                Text(when {
                    date == today -> stringResource(R.string.fw_today)
                    date > today.minusDays(7) -> date.format(DateTimeFormatter.ofPattern("EEE", locale))
                    else -> date.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale))
                })
            })
        }
    }
    SectionCard {
        val total = time.historyTotals[day]
        Overline(java.time.LocalDate.parse(day).format(DateTimeFormatter.ofPattern("EEEE", locale)) +
            (total?.let { " · " + durationText(it) } ?: ""))
        val apps = time.historyApps[day].orEmpty()
        if (apps.isEmpty()) Text(stringResource(R.string.fw_history_no_apps), style = MaterialTheme.typography.bodySmall)
        else {
            // Bars are relative to the busiest app that day, never to a limit.
            val busiest = apps.maxOf { it.millis }
            val initials = io.github.denogio.familywarden.ui.components.tileInitials(apps.map { it.label })
            apps.forEachIndexed { index, app ->
                if (index > 0) Spacer(Modifier.height(12.dp))
                Row(Modifier.fillMaxWidth().semantics(mergeDescendants = true) { }, verticalAlignment = Alignment.Top) {
                    AppTile(app.pkg, initials.getValue(app.label))
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        @OptIn(ExperimentalLayoutApi::class)
                        FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(app.label, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(end = 8.dp))
                            Text(durationText(app.millis), style = MaterialTheme.typography.labelLarge)
                        }
                        TimeProgress(app.millis, busiest, Modifier.fillMaxWidth())
                    }
                }
            }
        }
    }
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_history_local))
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
    // A plain rounded bar, as on the boards: no stop dot or gap.
    LinearProgressIndicator(progress = { if (limit == 0L) 1f else (used.toFloat() / limit).coerceIn(0f, 1f) },
        modifier = modifier.height(6.dp).clearAndSetSemantics { }, color = color, trackColor = track,
        gapSize = 0.dp, drawStopIndicator = {})
}
