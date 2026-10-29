package io.github.denogio.familywarden.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*

/** Availability of this app version, not a claim about this phone's permissions. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun CapabilityEntry(s: FamilyWardenState, actions: FamilyWardenActions) {
    var open by rememberSaveable { mutableStateOf(false) }
    OutlinedButton(onClick = { open = true }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
        Icon(Icons.Outlined.Info, null)
        Spacer(Modifier.width(8.dp))
        Text(stringResource(R.string.fw_capabilities_title))
    }
    if (open) ModalBottomSheet(onDismissRequest = { open = false },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text(stringResource(R.string.fw_capabilities_title), style = MaterialTheme.typography.titleLarge)
            InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_capabilities_scope))
            SectionCard {
                Overline(stringResource(R.string.fw_capabilities_native))
                Text(stringResource(R.string.fw_capabilities_native_body))
            }
            SectionCard {
                Overline(stringResource(R.string.fw_capabilities_router))
                Text(stringResource(R.string.fw_capabilities_router_body))
                Button(onClick = actions.openRouter, enabled = s.host != null,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.fw_voyage_open_router))
                }
            }
            SectionCard {
                Overline(stringResource(R.string.fw_capabilities_missing))
                Text(stringResource(R.string.fw_capabilities_missing_body))
            }
            TextButton(onClick = { open = false }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text(stringResource(R.string.action_ok))
            }
        }
    }
}
