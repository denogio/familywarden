package io.github.denogio.familywarden.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Router
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.MastActions
import io.github.denogio.familywarden.ui.MastDialog
import io.github.denogio.familywarden.ui.RemovalRefusal
import io.github.denogio.familywarden.ui.groupFingerprint
import io.github.denogio.familywarden.ui.spokenFingerprint
import io.github.denogio.familywarden.ui.theme.MastSpacing
import io.github.denogio.familywarden.ui.theme.MonoLarge
import io.github.denogio.familywarden.ui.theme.MonoMedium

/** Shows whichever dialog the Activity asked for. */
@Composable
fun MastDialogHost(dialog: MastDialog?, actions: MastActions) {
    when (dialog) {
        null -> Unit
        is MastDialog.ManualAddress -> ManualAddressDialog(dialog, actions.onSubmitAddress, actions.onDismissDialog)
        is MastDialog.ConfirmPairing -> PairConfirmSheet(dialog, actions.onConfirmPairing, actions.onDismissDialog)
        MastDialog.ConfirmRemove -> ConfirmRemoveDialog(actions.onConfirmRemove, actions.onDismissDialog)
        is MastDialog.RemoveRefused -> RemoveRefusedDialog(dialog.reason, actions.onDismissDialog)
    }
}

@Composable
fun ManualAddressDialog(
    dialog: MastDialog.ManualAddress,
    onSubmit: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var address by rememberSaveable { mutableStateOf(dialog.initial) }
    val submit = { if (address.isNotBlank()) onSubmit(address.trim()) }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Outlined.Router, contentDescription = null) },
        title = { Text(stringResource(R.string.pair_manual_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(MastSpacing.m)) {
                Text(stringResource(R.string.pair_manual_body), style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = address,
                    onValueChange = { address = it },
                    label = { Text(stringResource(R.string.pair_manual_label)) },
                    placeholder = { Text(stringResource(R.string.pair_manual_placeholder), style = MonoMedium) },
                    textStyle = MonoMedium,
                    singleLine = true,
                    isError = dialog.error != null,
                    supportingText = if (dialog.error != null) {
                        { Text(dialog.error.orEmpty()) }
                    } else {
                        null
                    },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { submit() }),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = submit, enabled = address.isNotBlank()) {
                Text(stringResource(R.string.action_continue))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PairConfirmSheet(
    dialog: MastDialog.ConfirmPairing,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = MaterialTheme.colorScheme.surfaceContainer,
    ) {
        PairConfirmContent(dialog.host, dialog.fingerprint, onConfirm, onDismiss)
    }
}

/** Sheet body, separate so it can be previewed without a window. */
@Composable
fun PairConfirmContent(
    host: String,
    fingerprint: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val fpLabel = stringResource(R.string.pair_confirm_fingerprint)
    Column(
        modifier
            .fillMaxWidth()
            .padding(start = MastSpacing.xl, end = MastSpacing.xl, bottom = MastSpacing.xl),
        verticalArrangement = Arrangement.spacedBy(MastSpacing.l),
    ) {
        Text(stringResource(R.string.pair_confirm_title), style = MaterialTheme.typography.headlineSmall)
        Column(verticalArrangement = Arrangement.spacedBy(MastSpacing.xs)) {
            Overline(stringResource(R.string.pair_confirm_address), color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(host, style = MonoMedium.copy(fontSize = MaterialTheme.typography.bodyLarge.fontSize))
        }
        Surface(
            shape = MaterialTheme.shapes.medium,
            color = MaterialTheme.colorScheme.surfaceContainerHigh,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Column(Modifier.padding(MastSpacing.l)) {
                Overline(fpLabel, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(
                    groupFingerprint(fingerprint),
                    style = MonoLarge,
                    modifier = Modifier.semantics { contentDescription = "$fpLabel: ${spokenFingerprint(fingerprint)}" },
                )
            }
        }
        Row {
            Icon(
                Icons.Outlined.Info,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(20.dp),
            )
            Spacer(Modifier.width(10.dp))
            Text(
                stringResource(R.string.pair_confirm_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Column(verticalArrangement = Arrangement.spacedBy(MastSpacing.xs)) {
            Button(
                onClick = onConfirm,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = MastSpacing.buttonHeight),
            ) { Text(stringResource(R.string.action_pair_confirm)) }
            TextButton(
                onClick = onDismiss,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = MastSpacing.buttonHeight),
            ) { Text(stringResource(R.string.action_cancel)) }
        }
    }
}

@Composable
fun RemoveRefusedDialog(reason: RemovalRefusal, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Outlined.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.secondary) },
        title = { Text(stringResource(R.string.fw_remove_denied), style = MaterialTheme.typography.headlineSmall) },
        text = { Text(stringResource(when (reason) {
            RemovalRefusal.ActivePact -> R.string.fw_remove_active
            RemovalRefusal.MissingAuthority -> R.string.fw_remove_authority
            RemovalRefusal.Unreachable -> R.string.fw_remove_unreachable
            RemovalRefusal.Unsupported -> R.string.fw_remove_unsupported
            RemovalRefusal.Unverified -> R.string.fw_remove_unverified
            RemovalRefusal.NotCompleted -> R.string.fw_remove_incomplete
        })) },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_ok)) } },
        containerColor = MaterialTheme.colorScheme.surfaceContainer,
    )
}

@Composable
fun ConfirmRemoveDialog(onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Outlined.DeleteOutline, contentDescription = null, tint = MaterialTheme.colorScheme.error) },
        title = { Text(stringResource(R.string.remove_confirm_title), style = MaterialTheme.typography.headlineSmall) },
        text = { Text(stringResource(R.string.fw_remove_confirm_body)) },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(stringResource(R.string.action_remove_confirm), color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
        containerColor = MaterialTheme.colorScheme.surfaceContainer,
    )
}
