package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FamilyChildSelector(state: FamilyWardenState, preferred: String?, onSelect: (String) -> Unit) {
    val selected = state.familyProfile(preferred) ?: return
    var choosing by remember { mutableStateOf(false) }
    OutlinedButton(onClick = { choosing = true }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
        Text(selected.name, modifier = Modifier.weight(1f))
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
fun FamilyTimeContent(state: FamilyWardenState, selectedChild: String? = null) {
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
    for (profile in profiles) SectionCard {
        Text(profile.name, style = MaterialTheme.typography.titleLarge)
        val usage = state.familyUsage.firstOrNull { it.profile.id == profile.id }
        if (usage == null || usage.devices.isEmpty()) {
            Text(stringResource(R.string.fw_usage_none))
        } else {
            Overline(stringResource(R.string.fw_usage_today_title))
            Text(usage.day, style = MaterialTheme.typography.bodySmall)
            val total = usage.totalSeconds / 60
            Text(pluralStringResource(R.plurals.fw_time_minutes, total, total), style = MaterialTheme.typography.headlineSmall)
            for (device in usage.devices) {
                val minutes = device.totalSeconds / 60
                Text(stringResource(R.string.fw_request_subject, device.name,
                    pluralStringResource(R.plurals.fw_time_minutes, minutes, minutes)))
            }
            for (app in usage.apps.filter { it.seconds >= 60 }) {
                val minutes = app.seconds / 60
                Text(stringResource(R.string.fw_request_subject, app.label.ifEmpty { app.pkg },
                    pluralStringResource(R.plurals.fw_time_minutes, minutes, minutes)),
                    style = MaterialTheme.typography.bodySmall)
            }
            if (usage.devices.any { it.incomplete }) Text(stringResource(R.string.fw_usage_incomplete))
        }
    }
    Text(stringResource(R.string.fw_family_reported), style = MaterialTheme.typography.bodySmall)
    state.syncError?.let { Text(stringResource(R.string.fw_family_refresh_failed)) }
}

@Composable
fun FamilyControlsContent(state: FamilyWardenState, actions: FamilyWardenActions, selectedChild: String? = null) {
    SectionCard {
        Text(stringResource(R.string.fw_family_router_controls), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_family_router_controls_body))
        state.familyProfile(selectedChild)?.let { Text(it.name) }
        if (state.binding?.approves.isNullOrEmpty()) Text(stringResource(R.string.fw_request_no_scope))
    }
    // Parent-only connection settings remain reachable without a fourth bottom tab.
    if (state.parentMode) ParentSettings(state, actions)
}
