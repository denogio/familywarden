package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*

private data class DecisionChoice(val request: ParentRequest, val approve: Boolean)

@Composable
fun RequestsContent(state: FamilyWardenState, actions: FamilyWardenActions) {
    var target by rememberSaveable { mutableStateOf<String?>(null) }
    var decision by remember { mutableStateOf<DecisionChoice?>(null) }
    var forget by rememberSaveable { mutableStateOf(false) }
    val ready = !state.syncing && !state.uploadPending
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
            else -> R.string.fw_request_sent
        }))
    } }
    if (state.binding.approves.isNotEmpty()) {
        HeroCard {
            HeroTitle(stringResource(R.string.fw_requests))
            HeroBody(state.binding.approves.joinToString(" · ") { it.name })
        }
        for (u in state.familyUsage) SectionCard {
            Overline(stringResource(R.string.fw_usage_today_title))
            Text(u.profile.name, style = MaterialTheme.typography.titleLarge)
            val total = u.totalSeconds / 60
            Text(pluralStringResource(R.plurals.fw_time_minutes, total, total), style = MaterialTheme.typography.headlineSmall)
            for (d in u.devices) {
                val m = d.totalSeconds / 60
                Text(stringResource(R.string.fw_request_subject, d.name, pluralStringResource(R.plurals.fw_time_minutes, m, m)),
                    style = MaterialTheme.typography.bodyMedium)
            }
            for (a in u.apps.filter { it.seconds >= 60 }) {
                val m = a.seconds / 60
                Text(stringResource(R.string.fw_request_subject, a.label.ifEmpty { a.pkg }, pluralStringResource(R.plurals.fw_time_minutes, m, m)),
                    style = MaterialTheme.typography.bodySmall)
            }
            if (u.devices.isEmpty()) Text(stringResource(R.string.fw_usage_none))
            if (u.devices.any { it.incomplete }) Text(stringResource(R.string.fw_usage_incomplete), style = MaterialTheme.typography.bodySmall)
        }
        if (state.parentRequests.isEmpty()) SectionCard {
            Text(stringResource(R.string.fw_request_empty), style = MaterialTheme.typography.titleMedium)
            Text(stringResource(R.string.fw_request_empty_body))
        }
        for (r in state.parentRequests) SectionCard {
            val label = if (r.target == "total") stringResource(R.string.fw_request_screen) else r.label.ifEmpty { r.target }
            Text(r.profile.name, style = MaterialTheme.typography.titleLarge)
            Text(label, style = MaterialTheme.typography.titleMedium)
            if (r.target != "total" && r.target != label) Text(r.target, style = MaterialTheme.typography.bodySmall)
            Text(pluralStringResource(R.plurals.fw_request_amount, r.minutes, r.minutes))
            if (r.note.isNotEmpty()) Text(r.note)
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
    if (state.appsActive) {
        HeroCard {
            HeroTitle(stringResource(R.string.fw_request_ask))
            HeroBody(stringResource(if (state.bound) R.string.fw_request_pact else R.string.fw_request_intro))
        }
        state.childRequest?.let { r -> SectionCard {
            val expired = state.requestEpoch?.let { now -> now >= (r.notAfter ?: r.expires) } == true
            Text(stringResource(when {
                expired || r.state == "expired" -> R.string.fw_request_expired
                r.state == "approved" -> R.string.fw_request_approved
                r.state == "denied" -> R.string.fw_request_denied
                else -> R.string.fw_request_pending
            }), style = MaterialTheme.typography.titleMedium)
            Text(if (r.target == "total") stringResource(R.string.fw_request_screen)
                else state.requestTargets.firstOrNull { it.pkg == r.target }?.label ?: r.target)
            val amount = r.granted ?: r.minutes
            Text(pluralStringResource(R.plurals.fw_request_amount, amount, amount))
            if (r.state == "approved" && !expired) {
                val remaining = state.screenTime?.grantRemaining?.get(r.target)
                if (remaining != null) {
                    val minutes = ((remaining + 59_999) / 60_000).toInt()
                    Text(pluralStringResource(R.plurals.fw_request_remaining, minutes, minutes))
                } else Text(stringResource(if (state.appsStatus == "grant_unavailable")
                    R.string.fw_request_time_unavailable else R.string.fw_request_apply_pending))
            }
        } }
        val open = state.childRequest?.let { it.state == "pending" && (state.requestEpoch == null || state.requestEpoch < it.expires) } == true
        if (state.canRequestTotal) Button(onClick = { target = "total" }, enabled = ready && !state.bound && !open, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.fw_request_screen))
        }
        for (app in state.requestTargets) OutlinedButton(onClick = { target = app.pkg },
            enabled = ready && !state.bound && !open, modifier = Modifier.fillMaxWidth()) { Text(app.label) }
    }
    InfoNote(Icons.Outlined.Inbox, stringResource(R.string.fw_request_phone_check))
    target?.let { chosen ->
        val label = if (chosen == "total") stringResource(R.string.fw_request_screen)
            else state.requestTargets.firstOrNull { it.pkg == chosen }?.label ?: chosen
        MinutesDialog(stringResource(R.string.fw_request_ask), label, 240, R.string.fw_request_send,
            onDismiss = { target = null }, onConfirm = { minutes -> target = null; actions.askTime(chosen, minutes) })
    }
    decision?.let { selected ->
        val r = selected.request
        val exists = state.parentRequests.any { it.id == r.id && it.hash == r.hash }
        if (exists) {
            val label = if (r.target == "total") stringResource(R.string.fw_request_screen) else r.label.ifEmpty { r.target }
            val subject = stringResource(R.string.fw_request_subject, r.profile.name, label)
            if (selected.approve) MinutesDialog(stringResource(R.string.fw_request_approve_title), subject, r.minutes,
                R.string.fw_request_approve, onDismiss = { decision = null }, onConfirm = { minutes ->
                    decision = null; actions.answerTime(r, true, minutes)
                })
            else ConfirmAction(stringResource(R.string.fw_request_deny_title), subject, R.string.fw_request_deny,
                onDismiss = { decision = null }, onConfirm = { decision = null; actions.answerTime(r, false, 0) })
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

@Composable
private fun MinutesDialog(title: String, subject: String, max: Int, confirm: Int, onDismiss: () -> Unit, onConfirm: (Int) -> Unit) {
    var value by rememberSaveable(title, subject, max) { mutableStateOf(minOf(30, max).toString()) }
    val minutes = value.toIntOrNull()?.takeIf { it in 1..max }
    AlertDialog(onDismissRequest = onDismiss, title = { Text(title) }, text = {
        Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(subject)
            OutlinedTextField(value, onValueChange = { value = it.filter { c -> c in '0'..'9' }.take(3) },
                label = { Text(stringResource(R.string.fw_request_minutes)) }, singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), isError = minutes == null)
            Text(stringResource(R.string.fw_request_range, max))
        }
    }, confirmButton = { TextButton(onClick = { minutes?.let(onConfirm) }, enabled = minutes != null) { Text(stringResource(confirm)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } })
}

@Composable
private fun ConfirmAction(title: String, body: String, confirm: Int, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    AlertDialog(onDismissRequest = onDismiss, title = { Text(title) }, text = { Text(body) },
        confirmButton = { TextButton(onClick = onConfirm) { Text(stringResource(confirm)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } })
}
