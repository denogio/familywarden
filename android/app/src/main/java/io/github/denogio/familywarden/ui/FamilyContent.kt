package io.github.denogio.familywarden.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*

/** Signed summaries only: no inferred online, applied or enforcement state. */
@Composable
fun FamilyTimeContent(state: FamilyWardenState) {
    if (state.binding == null) {
        HeroCard {
            HeroTitle(stringResource(R.string.fw_request_waiting_approval))
            HeroBody(stringResource(R.string.fw_request_waiting_body))
        }
        return
    }
    val profiles = state.binding?.approves.orEmpty()
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
fun FamilyControlsContent(state: FamilyWardenState, actions: FamilyWardenActions) {
    SectionCard {
        Text(stringResource(R.string.fw_family_router_controls), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_family_router_controls_body))
        for (profile in state.binding?.approves.orEmpty()) Text(profile.name)
        if (state.binding?.approves.isNullOrEmpty()) Text(stringResource(R.string.fw_request_no_scope))
    }
    // Parent-only connection settings remain reachable without a fourth bottom tab.
    if (state.parentMode) ParentSettings(state, actions)
}
