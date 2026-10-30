package io.github.denogio.familywarden.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

private const val TOTAL = "total"
private val PRESETS = listOf(15, 30, 60, 120, 240)

/** Whether the child may start a request now; the router rechecks everything. */
internal fun FamilyWardenState.askOpen(): Boolean =
    childRequest?.let { it.state == "pending" && (requestEpoch == null || requestEpoch < it.expires) } == true

/** "YOUR REQUEST" on Today: status words only; decisions are never made here. */
@Composable
internal fun ChildRequestCard(state: FamilyWardenState, zone: ZoneId) {
    val r = state.childRequest ?: return
    val closed = state.requestEpoch?.let { now -> now >= (r.notAfter ?: r.expires) } == true || r.state == "expired"
    val label = if (r.target == TOTAL) stringResource(R.string.fw_ask_all_apps_lower)
        else state.requestTargets.firstOrNull { it.pkg == r.target }?.label ?: r.target
    val colors = MaterialTheme.colorScheme
    val (chip, chipColor) = when {
        r.state == "approved" && !closed -> R.string.fw_req_chip_approved to colors.tertiaryContainer
        r.state == "denied" -> R.string.fw_req_chip_denied to colors.surfaceContainerHigh
        closed -> R.string.fw_req_chip_closed to Color.Transparent
        else -> R.string.fw_req_chip_waiting to colors.secondaryContainer
    }
    SectionCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.weight(1f)) { Overline(stringResource(R.string.fw_your_request)) }
            Surface(shape = CircleShape, color = chipColor,
                border = if (chipColor == Color.Transparent) BorderStroke(1.dp, colors.outline) else null) {
                Text(stringResource(chip), style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp))
            }
        }
        val amount = r.granted ?: r.minutes
        Text(stringResource(R.string.fw_req_title, durationText(amount * 60_000L), label),
            style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(top = 4.dp))
        val body = when {
            r.state == "approved" && !closed -> stringResource(R.string.fw_req_approved_body,
                durationText(r.minutes * 60_000L), durationText(amount * 60_000L), epochTime(r.notAfter ?: r.expires, zone))
            r.state == "denied" -> stringResource(R.string.fw_req_denied_body)
            closed -> stringResource(R.string.fw_req_closed_body)
            else -> stringResource(R.string.fw_req_pending_body, epochTime(r.expires, zone))
        }
        Text(body, style = MaterialTheme.typography.bodyMedium, color = colors.onSurfaceVariant,
            modifier = Modifier.padding(top = 6.dp))
        if (r.state == "approved" && !closed) {
            val remaining = state.screenTime?.grantRemaining?.get(r.target)
            Text(if (remaining != null) ((remaining + 59_999) / 60_000).toInt().let {
                pluralStringResource(R.plurals.fw_request_remaining, it, it)
            } else stringResource(if (state.appsStatus == "grant_unavailable")
                R.string.fw_request_time_unavailable else R.string.fw_request_apply_pending),
                style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 6.dp))
        }
    }
}

/** Ask for more time: what for, then how much (Family Link's two steps). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun AskFlow(state: FamilyWardenState, actions: FamilyWardenActions, onClose: () -> Unit) {
    var target by rememberSaveable { mutableStateOf<String?>(null) }
    var choice by rememberSaveable { mutableStateOf<String?>(null) }
    var preset by rememberSaveable { mutableStateOf<Int?>(30) }
    var other by rememberSaveable { mutableStateOf("") }
    var note by rememberSaveable { mutableStateOf("") }
    val time = state.screenTime
    val ready = !state.syncing && !state.uploadPending && !state.bound && !state.askOpen()
    val options = (if (state.canRequestTotal) listOf(TOTAL) else emptyList()) + state.requestTargets.map { it.pkg }
    fun label(key: String) = if (key == TOTAL) null else state.requestTargets.firstOrNull { it.pkg == key }?.label ?: key
    val allApps = stringResource(R.string.fw_ask_all_apps)
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { if (target != null) target = null else onClose() }) {
                Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.action_back))
            }
            Text(if (target == null) stringResource(R.string.fw_request_ask)
                else stringResource(R.string.fw_ask_more_for, label(target!!) ?: stringResource(R.string.fw_ask_all_apps_lower)),
                style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
        }
        val chosen = target
        if (chosen == null) {
            Text(stringResource(R.string.fw_ask_what))
            SectionCard {
                val initials = tileInitials(options.map { label(it) ?: allApps })
                Column(Modifier.selectableGroup()) {
                    options.forEachIndexed { index, key ->
                        if (index > 0) HorizontalDivider()
                        val name = label(key) ?: allApps
                        val left = time?.let { t ->
                            if (key == TOTAL) t.remainingMillis
                            else t.apps.firstOrNull { it.pkg == key }?.remainingMillis
                        }
                        Row(Modifier.fillMaxWidth().heightIn(min = 56.dp)
                            .selectable(choice == key, role = Role.RadioButton) { choice = key }
                            .padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                            RadioButton(selected = choice == key, onClick = null)
                            Spacer(Modifier.width(12.dp))
                            AppTile(key, if (key == TOTAL) stringResource(R.string.fw_ask_all_short) else initials.getValue(name))
                            Spacer(Modifier.width(12.dp))
                            Column {
                                Text(name, style = MaterialTheme.typography.titleMedium)
                                if (left != null) Text(stringResource(R.string.fw_ask_left_today, durationText(left)),
                                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                }
            }
            Button(onClick = { target = choice }, enabled = choice != null && ready,
                modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp)) { Text(stringResource(R.string.fw_ask_next)) }
        } else {
            Text(stringResource(R.string.fw_ask_how_much))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                PRESETS.forEach { minutes ->
                    FilterChip(selected = preset == minutes, onClick = { preset = minutes },
                        label = { Text(durationText(minutes * 60_000L)) }, modifier = Modifier.heightIn(min = 48.dp))
                }
                FilterChip(selected = preset == null, onClick = { preset = null },
                    label = { Text(stringResource(R.string.fw_ask_custom)) }, modifier = Modifier.heightIn(min = 48.dp))
            }
            val custom = other.toIntOrNull()?.takeIf { it in 1..240 }
            if (preset == null) OutlinedTextField(other, onValueChange = { other = it.filter { c -> c in '0'..'9' }.take(3) },
                label = { Text(stringResource(R.string.fw_request_minutes)) }, singleLine = true, isError = custom == null,
                supportingText = { Text(stringResource(R.string.fw_request_range, 240)) },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
            else Text(stringResource(R.string.fw_ask_up_to), style = MaterialTheme.typography.bodySmall)
            OutlinedTextField(note, onValueChange = { if (it.codePointCount(0, it.length) <= 140) note = it },
                label = { Text(stringResource(R.string.fw_ask_note)) }, modifier = Modifier.fillMaxWidth())
            InfoNote(Icons.Outlined.Schedule, stringResource(R.string.fw_ask_closes))
            val minutes = preset ?: custom
            Button(onClick = { minutes?.let { actions.askTime(chosen, it, note); onClose() } },
                enabled = minutes != null && ready, modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp)) {
                Icon(Icons.AutoMirrored.Outlined.Send, null, Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.fw_request_send))
            }
        }
    }
}

/** "38 min", "1 h 22 min", "2 h": whole minutes, never a second-by-second countdown. */
@Composable
internal fun durationText(millis: Long, roundUp: Boolean = false): String {
    val total = ((millis + if (roundUp) 59_999 else 0) / 60_000).toInt()
    val hours = total / 60
    val rest = total % 60
    return when {
        hours == 0 -> pluralStringResource(R.plurals.fw_time_minutes, rest, rest)
        rest == 0 -> stringResource(R.string.fw_time_hours, hours)
        else -> stringResource(R.string.fw_time_hours_minutes, hours, rest)
    }
}

@Composable
private fun epochTime(epochSeconds: Long, zone: ZoneId): String = Instant.ofEpochSecond(epochSeconds).atZone(zone)
    .format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT).withLocale(LocalConfiguration.current.locales[0]))
