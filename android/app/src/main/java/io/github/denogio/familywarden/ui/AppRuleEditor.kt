package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private fun AppRule.draft() = when (this) {
    AppRule.Blocked -> RuleDraft("blocked")
    AppRule.Always -> RuleDraft("always")
    is AppRule.Limit -> RuleDraft("limit", minutes.toString())
    else -> RuleDraft("allowed")
}

/** Drafts and durable signed operations have independent lifetimes. Navigation
 * never clears an unresolved operation, including one for a different child. */
@Composable
internal fun AppRuleEditor(s: FamilyWardenState, actions: FamilyWardenActions, data: InstalledApps,
    app: InstalledApp, refresh: () -> Unit) {
    val identity = listOf(s.fingerprint, s.phoneFingerprint, s.binding?.deviceId, data.profile.id, app.pkg)
    // An untouched draft follows the latest read rule; edited drafts are stored and survive.
    var draft by remember(identity, app.rule) { mutableStateOf(actions.ruleDraft(data.profile.id, app.pkg) ?: app.rule.draft()) }
    var operation by remember(identity) { mutableStateOf(actions.ruleOperation()) }
    var saving by remember(identity) { mutableStateOf(false) }
    var notSent by remember(identity) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val currentActions by rememberUpdatedState(actions)
    fun update(next: RuleDraft) {
        actions.saveRuleDraft(data.profile.id, app.pkg, next)
        draft = next
    }
    fun run(check: Boolean, retry: Boolean = false) {
        saving = true
        notSent = false
        val previous = currentActions.ruleOperation()
        scope.launch {
            try {
                operation = withContext(Dispatchers.IO) {
                    if (check) currentActions.checkAppRule(retry) else currentActions.saveAppRule(data, app, draft)
                }
                if (operation?.state in setOf("saved", "matched", "conflict", "expired")) refresh()
            } catch (_: Exception) {
                operation = currentActions.ruleOperation()
                notSent = !check && ruleSaveNotSent(previous, operation)
            } finally { saving = false; operation = currentActions.ruleOperation() }
        }
    }
    val pending = operation?.unresolved == true
    val authorized = s.binding?.approves?.any { it.id == data.profile.id } == true
    SectionCard {
        Overline(stringResource(R.string.fw_apps_edit_rule))
        Text(stringResource(R.string.fw_apps_draft))
        listOf("allowed" to R.string.fw_apps_allowed, "limit" to R.string.fw_apps_daily_limit,
            "always" to R.string.fw_apps_filter_always, "blocked" to R.string.fw_apps_filter_blocked).forEach { (mode, label) ->
            Row {
                RadioButton(selected = draft.mode == mode, enabled = !saving, onClick = { update(draft.copy(mode = mode)) })
                TextButton(onClick = { update(draft.copy(mode = mode)) }, enabled = !saving,
                    modifier = Modifier.heightIn(min = 48.dp)) { Text(stringResource(label)) }
            }
        }
        if (draft.mode == "limit") OutlinedTextField(value = draft.minutes, onValueChange = { update(draft.copy(minutes = it)) },
            label = { Text(stringResource(R.string.fw_apps_minutes)) }, singleLine = true, enabled = !saving,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            isError = runCatching { draft.quota() }.isFailure, modifier = Modifier.fillMaxWidth())
        Text(stringResource(ruleDraftBody(draft.mode), data.profile.name), style = MaterialTheme.typography.bodySmall)
        if (!authorized) Text(stringResource(R.string.fw_history_scope_changed))
        Button(onClick = { run(false) }, enabled = authorized && !pending && !saving && runCatching { draft.quota() }.isSuccess,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(if (saving) R.string.fw_apps_saving else R.string.fw_apps_save)) }
        TextButton(onClick = {
            actions.saveRuleDraft(data.profile.id, app.pkg, null)
            draft = app.rule.draft()
        }, enabled = !saving) { Text(stringResource(R.string.fw_apps_discard_draft)) }
        Text(stringResource(R.string.fw_apps_draft_only), style = MaterialTheme.typography.bodySmall)
    }
    if (notSent || operation != null) SectionCard {
        val op = if (notSent) null else operation
        Text(stringResource(when {
            notSent -> R.string.fw_apps_not_sent
            op?.error == "configuration_required" -> R.string.fw_apps_configuration_required
            op?.state == "saved" -> R.string.fw_apps_saved
            op?.state == "matched" -> R.string.fw_apps_matched
            op?.state == "refused" -> R.string.fw_apps_refused
            op?.state == "conflict" -> R.string.fw_apps_conflict
            op?.state == "expired" -> R.string.fw_apps_expired
            op?.state == "retryable" -> R.string.fw_apps_retryable
            else -> R.string.fw_apps_unknown
        }))
        if (op != null) {
            // Keep the original target visible, even after child/app navigation.
            Text(stringResource(R.string.fw_apps_operation_target, op.profile, op.pkg), style = MaterialTheme.typography.bodySmall)
            if (op.unresolved) {
                TextButton(onClick = { run(true) }, enabled = !saving) { Text(stringResource(R.string.fw_apps_check_status)) }
                if (op.state == "retryable") TextButton(onClick = { run(true, true) }, enabled = !saving) {
                    Text(stringResource(R.string.fw_apps_retry_original))
                }
            }
        }
        InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_saved_not_applied))
    }
}

/** Available even when the current child has no inventory or scope was revoked. */
@Composable
internal fun PendingRuleStatus(s: FamilyWardenState, actions: FamilyWardenActions) {
    val identity = listOf(s.fingerprint, s.phoneFingerprint, s.binding?.deviceId)
    var op by remember(identity) { mutableStateOf(actions.ruleOperation()) }
    var busy by remember(identity) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    if (op?.unresolved != true) return
    fun check(retry: Boolean) {
        busy = true
        scope.launch {
            try { op = withContext(Dispatchers.IO) { actions.checkAppRule(retry) } }
            catch (_: Exception) { op = actions.ruleOperation() ?: op }
            finally { busy = false }
        }
    }
    SectionCard {
        Text(stringResource(if (op?.state == "retryable") R.string.fw_apps_retryable else R.string.fw_apps_unknown))
        Text(stringResource(R.string.fw_apps_operation_target, op!!.profile, op!!.pkg))
        TextButton(onClick = { check(false) }, enabled = !busy) { Text(stringResource(R.string.fw_apps_check_status)) }
        if (op?.state == "retryable") TextButton(onClick = { check(true) }, enabled = !busy) {
            Text(stringResource(R.string.fw_apps_retry_original))
        }
    }
}

internal fun ruleDraftBody(mode: String): Int = when (mode) {
    "blocked" -> R.string.fw_apps_rule_blocked_body
    "always" -> R.string.fw_apps_rule_always_body
    "limit" -> R.string.fw_apps_rule_limit_body
    else -> R.string.fw_apps_rule_allowed_body
}
