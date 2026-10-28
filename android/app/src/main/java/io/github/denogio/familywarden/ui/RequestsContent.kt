package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*

private data class DecisionChoice(val request: ParentRequest, val approve: Boolean)

@Composable
fun RequestsContent(state: FamilyWardenState, actions: FamilyWardenActions, family: Boolean = state.canManageFamily) {
    var decision by remember { mutableStateOf<DecisionChoice?>(null) }
    var forget by rememberSaveable { mutableStateOf(false) }
    val ready = !state.syncing && !state.uploadPending
    val requests = state.familyRequests
    val newApps = state.familyNewApps
    if (state.binding == null) {
        HeroCard {
            HeroTitle(stringResource(R.string.fw_request_waiting_approval))
            HeroBody(stringResource(R.string.fw_request_waiting_body))
        }
        state.phoneFingerprint?.let { fingerprint -> SectionCard { Text(fingerprint, style = MaterialTheme.typography.bodySmall) } }
        return
    }
    if (state.uploadPending) SectionCard {
        Text(stringResource(R.string.fw_request_retry_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_request_retry_body))
        Button(onClick = actions.retryUpload, enabled = !state.syncing, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.fw_request_retry))
        }
        TextButton(onClick = { forget = true }, enabled = !state.syncing) { Text(stringResource(R.string.fw_request_forget)) }
    }
    state.uploadReply?.let { reply -> SectionCard {
        Text(stringResource(when {
            reply.error != null -> R.string.fw_request_action_rejected
            reply.state == "denied" -> R.string.fw_request_denial_sent
            reply.granted != null -> R.string.fw_request_approval_sent
            reply.state == "approved" -> R.string.fw_app_allowed_sent
            else -> R.string.fw_request_sent
        }))
    } }
    if (family && state.binding.approves.isNotEmpty()) {
        Text(state.binding.approves.joinToString(" · ") { it.name },
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (requests.isNotEmpty() || newApps.isNotEmpty())
            Overline(stringResource(R.string.fw_request_waiting_count, requests.size + newApps.size))
        for (app in newApps) SectionCard {
            Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                AppTile(app.profile.id, app.profile.name.take(1).uppercase(), 28.dp, CircleShape)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.fw_request_subject, app.profile.name, stringResource(R.string.fw_app_new_title)),
                        style = MaterialTheme.typography.titleMedium)
                    Text(app.label.ifEmpty { app.pkg }, style = MaterialTheme.typography.titleMedium)
                }
            }
            if (app.label.isNotEmpty()) Text(app.pkg, style = MaterialTheme.typography.bodySmall)
            Text(stringResource(R.string.fw_app_new_body))
            Button(onClick = { actions.decideApp(app, true) }, enabled = ready, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_app_allow))
            }
            OutlinedButton(onClick = { actions.decideApp(app, false) }, enabled = ready, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_app_deny))
            }
        }
        if (requests.isEmpty() && newApps.isEmpty()) SectionCard {
            Text(stringResource(R.string.fw_request_empty), style = MaterialTheme.typography.titleMedium)
            Text(stringResource(R.string.fw_request_empty_body))
        }
        for (r in requests) SectionCard {
            val label = if (r.target == "total") stringResource(R.string.fw_request_screen) else r.label.ifEmpty { r.target }
            Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                AppTile(r.profile.id, r.profile.name.take(1).uppercase(), 28.dp, CircleShape)
                Spacer(Modifier.width(12.dp))
                Text(stringResource(R.string.fw_request_card_title, r.profile.name, durationText(r.minutes * 60_000L), label),
                    style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            }
            if (r.target != "total" && r.target != label) Text(r.target, style = MaterialTheme.typography.bodySmall)
            Text(stringResource(R.string.fw_request_received_closes, formatWhen(r.received * 1000, state.nowMillis),
                formatWhen(r.expires * 1000, state.nowMillis)), style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (r.note.isNotEmpty()) Text(stringResource(R.string.fw_request_quoted_note, r.note))
            val current = state.requestEpoch?.let { it < r.expires } ?: true
            Button(onClick = { decision = DecisionChoice(r, true) }, enabled = ready && current, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_request_approve))
            }
            OutlinedButton(onClick = { decision = DecisionChoice(r, false) }, enabled = ready && current, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_request_deny))
            }
        }
    } else if (state.parentMode) SectionCard {
        Text(stringResource(R.string.fw_request_no_scope), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fw_request_no_scope_body))
    }
    InfoNote(Icons.Outlined.Inbox, stringResource(R.string.fw_request_phone_check))
    decision?.let { selected ->
        val r = selected.request
        val exists = family && requests.any { it.id == r.id && it.hash == r.hash }
        if (exists) {
            val label = if (r.target == "total") stringResource(R.string.fw_request_screen) else r.label.ifEmpty { r.target }
            val subject = stringResource(R.string.fw_request_subject, r.profile.name, label)
            if (selected.approve) MinutesDialog(stringResource(R.string.fw_request_approve_title), subject, r.minutes,
                R.string.fw_request_approve, ready, onDismiss = { decision = null }, onConfirm = { minutes ->
                    decision = null; actions.answerTime(r, true, minutes)
                })
            else ConfirmAction(stringResource(R.string.fw_request_deny_title), subject, R.string.fw_request_deny,
                onDismiss = { decision = null }, onConfirm = { decision = null; actions.answerTime(r, false, 0) }, enabled = ready)
        }
    }
    if (forget) ConfirmAction(stringResource(R.string.fw_request_forget), stringResource(R.string.fw_request_forget_body),
        R.string.fw_request_forget, onDismiss = { forget = false }, onConfirm = { forget = false; actions.forgetUpload() })
}

@Composable
fun ParentSettings(state: FamilyWardenState, actions: FamilyWardenActions) {
    var disconnect by rememberSaveable { mutableStateOf(false) }
    HeroCard {
        HeroTitle(stringResource(R.string.fw_parent_settings))
        HeroBody(stringResource(R.string.fw_parent_settings_body))
    }
    SectionCard {
        Text(state.binding?.profileName ?: stringResource(R.string.fw_request_waiting_approval), style = MaterialTheme.typography.titleLarge)
        state.host?.let { Text(it) }
        state.phoneFingerprint?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
        Button(onClick = actions.sync, enabled = !state.syncing, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_sync)) }
        OutlinedButton(onClick = actions.manualPair, enabled = !state.syncing, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.action_pair_manually))
        }
        TextButton(onClick = { disconnect = true }, enabled = !state.syncing) { Text(stringResource(R.string.fw_parent_disconnect)) }
    }
    if (disconnect) ConfirmAction(stringResource(R.string.fw_parent_disconnect), stringResource(R.string.fw_parent_disconnect_body),
        R.string.fw_parent_disconnect, onDismiss = { disconnect = false }, onConfirm = { disconnect = false; actions.parentSignOut() })
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun MinutesDialog(title: String, subject: String, max: Int, confirm: Int, ready: Boolean,
    onDismiss: () -> Unit, onConfirm: (Int) -> Unit) {
    var value by rememberSaveable(title, subject, max) { mutableStateOf(max.toString()) }
    val minutes = value.toIntOrNull()?.takeIf { it in 1..max }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(Modifier.verticalScroll(rememberScrollState()).imePadding().padding(horizontal = 24.dp)
            .padding(bottom = 24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(title, style = MaterialTheme.typography.titleLarge)
            Text(subject)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                (listOf(15, 30, 60, 120) + max).filter { it <= max }.distinct().sorted().forEach { amount ->
                    FilterChip(selected = minutes == amount, onClick = { value = amount.toString() },
                        enabled = ready, modifier = Modifier.heightIn(min = 48.dp), label = {
                            Text(if (amount == max) stringResource(R.string.fw_request_asked_amount, durationText(amount * 60_000L))
                                else durationText(amount * 60_000L))
                        })
                }
            }
            OutlinedTextField(value, onValueChange = { value = it.filter { c -> c in '0'..'9' }.take(3) },
                label = { Text(stringResource(R.string.fw_request_minutes)) }, singleLine = true, enabled = ready,
                modifier = Modifier.fillMaxWidth(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                isError = minutes == null)
            Text(stringResource(R.string.fw_request_range, max), style = MaterialTheme.typography.bodySmall)
            InfoNote(Icons.Outlined.Inbox, stringResource(R.string.fw_request_approval_bounds))
            Button(onClick = { if (ready) minutes?.let(onConfirm) }, enabled = ready && minutes != null,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(confirm)) }
            TextButton(onClick = onDismiss, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    }
}

@Composable
private fun ConfirmAction(title: String, body: String, confirm: Int, onDismiss: () -> Unit, onConfirm: () -> Unit,
    enabled: Boolean = true) {
    AlertDialog(onDismissRequest = onDismiss, title = { Text(title) }, text = { Text(body) },
        confirmButton = { TextButton(onClick = onConfirm, enabled = enabled) { Text(stringResource(confirm)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } })
}
