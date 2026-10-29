package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

private sealed interface HistoryView {
    data object Loading : HistoryView
    data class Loaded(val day: FamilyHistoryDay) : HistoryView
    data class Failed(val scopeChanged: Boolean) : HistoryView
}

/** One bounded read per selected day; no invented index, zero or device roster. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun FamilyHistoryContent(s: FamilyWardenState, actions: FamilyWardenActions, preferred: String?, back: () -> Unit) {
    TextButton(onClick = back) {
        Icon(Icons.Outlined.ArrowBack, null)
        Spacer(Modifier.width(8.dp))
        Text(stringResource(R.string.fw_family_screen_time))
    }
    val profile = s.familyProfile(preferred)
    if (profile == null) {
        Text(stringResource(if (s.binding == null) R.string.fw_request_waiting_approval else R.string.fw_request_no_scope))
        return
    }
    val zone = ZoneId.of(s.timeZone ?: ZoneId.systemDefault().id)
    val today = Instant.ofEpochMilli(s.requestEpoch?.times(1000) ?: s.nowMillis).atZone(zone).toLocalDate()
    val earliest = today.minusDays(29)
    var selection by rememberSaveable(profile.id, s.fingerprint, s.phoneFingerprint) { mutableStateOf(today.toString()) }
    val day = LocalDate.parse(selection).coerceIn(earliest, today)
    var choosing by remember { mutableStateOf(false) }
    var retry by remember { mutableIntStateOf(0) }
    // Removing a scope or changing household/phone identity cancels and discards
    // in-flight UI work. A slow reply can only update its old remembered slot.
    val key = listOf(profile.id, day.toString(), s.binding?.deviceId, s.fingerprint, s.phoneFingerprint,
        s.binding?.approves?.map { it.id }?.sorted().toString())
    var view by remember(key) { mutableStateOf<HistoryView>(HistoryView.Loading) }
    val loader by rememberUpdatedState(actions.loadHistory)
    LaunchedEffect(key, retry, s.lastSync) {
        view = HistoryView.Loading
        try {
            val result = withContext(Dispatchers.IO) { loader(profile.id, day) }
            require(result.profile.id == profile.id && result.day == day.toString())
            view = HistoryView.Loaded(result)
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (e: Exception) { view = HistoryView.Failed(e is HistoryRefused && e.code == "not_parent") }
    }
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = { selection = day.minusDays(1).toString() }, enabled = day > earliest) {
            Icon(Icons.Outlined.ChevronLeft, stringResource(R.string.fw_history_previous))
        }
        OutlinedButton(onClick = { choosing = true }, modifier = Modifier.weight(1f).heightIn(min = 48.dp)) {
            Icon(Icons.Outlined.CalendarMonth, stringResource(R.string.fw_history_choose_day))
            Spacer(Modifier.width(8.dp))
            Text(historyDate(day))
        }
        IconButton(onClick = { selection = day.plusDays(1).toString() }, enabled = day < today) {
            Icon(Icons.Outlined.ChevronRight, stringResource(R.string.fw_history_next))
        }
    }
    when (val current = view) {
        HistoryView.Loading -> SectionCard {
            CircularProgressIndicator(Modifier.size(24.dp))
            Text(stringResource(R.string.fw_history_loading))
        }
        is HistoryView.Failed -> SectionCard {
            Text(stringResource(if (current.scopeChanged) R.string.fw_history_scope_changed else R.string.fw_history_load_failed))
            TextButton(onClick = { retry++ }) { Text(stringResource(R.string.action_try_again)) }
            TextButton(onClick = actions.sync, enabled = !s.syncing) { Text(stringResource(R.string.action_sync)) }
        }
        is HistoryView.Loaded -> {
            val usage = current.day.usage
            SectionCard {
                Overline(stringResource(R.string.fw_history_recorded))
                if (usage == null) {
                    Text(stringResource(R.string.fw_history_no_report), style = MaterialTheme.typography.headlineSmall)
                    Text(stringResource(R.string.fw_history_no_report_body))
                } else {
                    Text(durationText(usage.totalSeconds * 1000L), style = MaterialTheme.typography.headlineMedium)
                    if (usage.totalSeconds == 0) Text(stringResource(R.string.fw_history_measured_zero))
                    if (usage.devices.any { it.incomplete }) InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_usage_incomplete))
                    val apps = usage.apps.sortedByDescending { it.seconds }
                    val busiest = apps.maxOfOrNull { it.seconds } ?: 0
                    val initials = tileInitials(apps.map { it.label.ifEmpty { it.pkg } })
                    for (app in apps) {
                        Spacer(Modifier.height(8.dp))
                        @OptIn(ExperimentalLayoutApi::class)
                        FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            val name = app.label.ifEmpty { app.pkg }
                            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(end = 8.dp)) {
                                AppTile(app.pkg, initials.getValue(name))
                                Spacer(Modifier.width(8.dp))
                                Text(name, style = MaterialTheme.typography.titleMedium)
                            }
                            Text(durationText(app.seconds * 1000L), style = MaterialTheme.typography.labelLarge)
                        }
                        TimeProgress(app.seconds.toLong(), busiest.toLong(), Modifier.fillMaxWidth())
                    }
                }
            }
            usage?.let {
                SectionCard {
                    Overline(stringResource(R.string.fw_history_sources))
                    for (source in it.devices) {
                        Text(source.name, style = MaterialTheme.typography.titleMedium)
                        Text(durationText(source.totalSeconds * 1000L))
                        Text(source.received?.let { at -> stringResource(R.string.fw_history_receipt, historyInstant(at, zone)) }
                            ?: stringResource(R.string.fw_history_receipt_unknown), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
            Text(stringResource(R.string.fw_history_read_at, historyInstant(current.day.serverTime, zone)),
                style = MaterialTheme.typography.bodySmall)
        }
    }
    if (s.syncError != null) InfoNote(Icons.Outlined.CloudOff, stringResource(R.string.fw_family_refresh_failed))
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_history_coverage))
    if (choosing) ModalBottomSheet(onDismissRequest = { choosing = false }) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(24.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.fw_history_choose_day), style = MaterialTheme.typography.titleLarge)
            repeat(30) { back ->
                val date = today.minusDays(back.toLong())
                OutlinedButton(onClick = { selection = date.toString(); choosing = false },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(historyDate(date)) }
            }
        }
    }
}

@Composable
private fun historyDate(day: LocalDate): String = day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
    .withLocale(LocalConfiguration.current.locales[0]))

@Composable
private fun historyInstant(epoch: Long, zone: ZoneId): String = Instant.ofEpochSecond(epoch).atZone(zone)
    .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
        .withLocale(LocalConfiguration.current.locales[0]))
