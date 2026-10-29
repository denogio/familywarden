package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.foundation.clickable
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.ChevronRight
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import java.time.Instant
import java.time.ZoneId

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FamilyChildSelector(state: FamilyWardenState, preferred: String?, onSelect: (String) -> Unit) {
    val selected = state.familyProfile(preferred) ?: return
    var choosing by remember { mutableStateOf(false) }
    OutlinedButton(onClick = { choosing = true }, modifier = Modifier.widthIn(max = 240.dp).heightIn(min = 48.dp)) {
        AppTile(selected.id, selected.name.take(1).uppercase(), 28.dp, CircleShape)
        Spacer(Modifier.width(8.dp))
        Text(selected.name, modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp))
        Icon(Icons.Outlined.ExpandMore, stringResource(R.string.fw_family_choose_child))
    }
    if (choosing) ModalBottomSheet(onDismissRequest = { choosing = false }) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(24.dp).selectableGroup()) {
            Text(stringResource(R.string.fw_family_choose_child), style = MaterialTheme.typography.titleLarge)
            for (profile in state.binding?.approves.orEmpty()) {
                Row(Modifier.fillMaxWidth().heightIn(min = 48.dp)
                    .selectable(selected = profile.id == selected.id, role = Role.RadioButton,
                        onClick = { onSelect(profile.id); choosing = false })
                    .padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    RadioButton(selected = profile.id == selected.id, onClick = null)
                    Text(profile.name, modifier = Modifier.padding(start = 12.dp).weight(1f))
                }
            }
        }
    }
}

/** Signed summaries only: no inferred online, applied or enforcement state. */
@Composable
fun FamilyTimeContent(state: FamilyWardenState, selectedChild: String? = null, onHistory: () -> Unit = {}) {
    if (state.binding == null) {
        HeroCard {
            HeroTitle(stringResource(R.string.fw_request_waiting_approval))
            HeroBody(stringResource(R.string.fw_request_waiting_body))
        }
        return
    }
    val profiles = listOfNotNull(state.familyProfile(selectedChild))
    if (profiles.isEmpty()) SectionCard {
        Text(stringResource(R.string.fw_request_no_scope))
        Text(stringResource(R.string.fw_request_no_scope_body))
    }
    for (profile in profiles) {
        val usage = state.familyUsage.firstOrNull { it.profile.id == profile.id }
        val today = Instant.ofEpochMilli(state.requestEpoch?.times(1000) ?: state.nowMillis)
            .atZone(ZoneId.of(state.timeZone ?: ZoneId.systemDefault().id)).toLocalDate().toString()
        SectionCard {
            Overline(stringResource(if (usage == null || usage.day == today) R.string.fw_usage_today_title else R.string.fw_history_recorded))
            if (usage == null) Text(stringResource(R.string.fw_usage_unavailable))
            else if (usage.devices.isEmpty()) {
                Text(stringResource(if (usage.day == today) R.string.fw_usage_none else R.string.fw_history_no_report_body))
            } else {
                Text(durationText(usage.totalSeconds * 1000L), style = MaterialTheme.typography.headlineMedium)
                Text(stringResource(R.string.fw_usage_reported_day, usage.day),
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                val apps = usage.apps.filter { it.seconds >= 60 }.sortedByDescending { it.seconds }
                val busiest = apps.maxOfOrNull { it.seconds } ?: 0
                for (app in apps) {
                    Spacer(Modifier.height(8.dp))
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        @OptIn(ExperimentalLayoutApi::class)
                        FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(app.label.ifEmpty { app.pkg }, modifier = Modifier.padding(end = 8.dp),
                                style = MaterialTheme.typography.titleMedium)
                            Text(durationText(app.seconds * 1000L), style = MaterialTheme.typography.labelLarge)
                        }
                        TimeProgress(app.seconds.toLong(), busiest.toLong(), Modifier.fillMaxWidth())
                    }
                }
                if (usage.devices.any { it.incomplete }) Text(stringResource(R.string.fw_usage_incomplete),
                    style = MaterialTheme.typography.bodySmall)
            }
        }
        for (device in usage?.devices.orEmpty()) SectionCard {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.PhoneAndroid, null, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(device.name, style = MaterialTheme.typography.titleMedium)
                    Text(durationText(device.totalSeconds * 1000L), style = MaterialTheme.typography.bodyMedium)
                    Text(device.received?.let { stringResource(R.string.fw_history_receipt, formatWhen(it * 1000, state.nowMillis)) }
                        ?: stringResource(R.string.fw_history_receipt_unknown), style = MaterialTheme.typography.bodySmall)
                    if (device.incomplete) Text(stringResource(R.string.fw_usage_device_incomplete),
                        style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
    if (profiles.isNotEmpty()) TextButton(onClick = onHistory, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
        Text(stringResource(R.string.fw_history))
    }
    Text(stringResource(R.string.fw_family_reported), style = MaterialTheme.typography.bodySmall)
    state.syncError?.let { Text(stringResource(R.string.fw_family_refresh_failed)) }
}

@Composable
fun FamilyControlsContent(state: FamilyWardenState, actions: FamilyWardenActions, selectedChild: String? = null,
    onApps: () -> Unit = {}) {
    state.familyProfile(selectedChild)?.let { child ->
        SectionCard {
            Overline(stringResource(R.string.fw_family_rules))
            Row(Modifier.fillMaxWidth().heightIn(min = 56.dp).clickable(role = Role.Button, onClick = onApps),
                verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Icon(Icons.Outlined.Apps, null)
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.fw_apps_entry), style = MaterialTheme.typography.titleMedium)
                    Text(stringResource(R.string.fw_apps_entry_body, child.name), style = MaterialTheme.typography.bodySmall)
                }
                Icon(Icons.Outlined.ChevronRight, null)
            }
        }
    }
    SectionCard {
        Text(stringResource(R.string.fw_family_router_controls), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_family_router_controls_body))
        state.familyProfile(selectedChild)?.let { Text(it.name) }
        if (state.binding?.approves.isNullOrEmpty()) Text(stringResource(R.string.fw_request_no_scope))
    }
    Button(onClick = actions.openRouter, enabled = state.host != null,
        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(R.string.fw_voyage_open_router)) }
    Text(stringResource(R.string.fw_capabilities_router_login), style = MaterialTheme.typography.bodySmall)
    if (!state.parentMode) CapabilityEntry(state, actions)
    // Parent-only connection settings remain reachable without a fourth bottom tab.
    if (state.parentMode) ParentSettings(state, actions)
}
