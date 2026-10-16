package me.hojgaard.ulysses.mast.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Anchor
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Sync
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.WifiTethering
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.TopAppBarScrollBehavior
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import me.hojgaard.ulysses.mast.R
import me.hojgaard.ulysses.mast.ui.components.CommandBlock
import me.hojgaard.ulysses.mast.ui.components.DetailItem
import me.hojgaard.ulysses.mast.ui.components.DetailsCard
import me.hojgaard.ulysses.mast.ui.components.HeroBadge
import me.hojgaard.ulysses.mast.ui.components.HeroBody
import me.hojgaard.ulysses.mast.ui.components.HeroCard
import me.hojgaard.ulysses.mast.ui.components.HeroTitle
import me.hojgaard.ulysses.mast.ui.components.InfoNote
import me.hojgaard.ulysses.mast.ui.components.MastDialogHost
import me.hojgaard.ulysses.mast.ui.components.Overline
import me.hojgaard.ulysses.mast.ui.components.PactCard
import me.hojgaard.ulysses.mast.ui.components.SectionCard
import me.hojgaard.ulysses.mast.ui.components.SetupStep
import me.hojgaard.ulysses.mast.ui.components.StatusHero
import me.hojgaard.ulysses.mast.ui.components.SyncBanner
import me.hojgaard.ulysses.mast.ui.theme.MastSpacing

/**
 * The one screen. Stateless: renders [state] and reports taps through [actions].
 * Handles edge-to-edge via Scaffold insets (top bar + navigation bar).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MastScreen(
    state: MastUiState,
    actions: MastActions,
    modifier: Modifier = Modifier,
) {
    val scrollBehavior = TopAppBarDefaults.pinnedScrollBehavior()
    Scaffold(
        modifier = modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            MastTopBar(
                showSync = state.setup is SetupState.Paired,
                isSyncing = state.isSyncing,
                onSync = actions.onSync,
                scrollBehavior = scrollBehavior,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(innerPadding)
                .padding(horizontal = MastSpacing.gutter, vertical = MastSpacing.s),
            verticalArrangement = Arrangement.spacedBy(MastSpacing.m),
        ) {
            when (val setup = state.setup) {
                SetupState.NotDeviceOwner -> NotDeviceOwnerContent(state, actions)
                SetupState.NotPaired -> NotPairedContent(state, actions)
                is SetupState.Paired -> PairedContent(setup.status, state, actions)
            }
            Spacer(Modifier.height(MastSpacing.l))
        }
    }
    MastDialogHost(state.dialog, state.details.captain, actions)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MastTopBar(
    showSync: Boolean,
    isSyncing: Boolean,
    onSync: () -> Unit,
    scrollBehavior: TopAppBarScrollBehavior,
) {
    val syncingLabel = stringResource(R.string.action_syncing)
    TopAppBar(
        title = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    MastIcons.Mast,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.secondary,
                    modifier = Modifier.size(26.dp),
                )
                Spacer(Modifier.width(10.dp))
                Text(
                    stringResource(R.string.mast_title),
                    style = MaterialTheme.typography.titleLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        },
        actions = {
            if (showSync) {
                if (isSyncing) {
                    Box(Modifier.size(48.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(
                            strokeWidth = 2.dp,
                            modifier = Modifier
                                .size(22.dp)
                                .semantics { contentDescription = syncingLabel },
                        )
                    }
                } else {
                    IconButton(onClick = onSync) {
                        Icon(Icons.Outlined.Sync, contentDescription = stringResource(R.string.action_sync))
                    }
                }
            }
        },
        colors = TopAppBarDefaults.topAppBarColors(
            containerColor = MaterialTheme.colorScheme.background,
            scrolledContainerColor = MaterialTheme.colorScheme.surfaceContainer,
        ),
        scrollBehavior = scrollBehavior,
    )
}

// ------------------------------------------------------------------ 1. Not device owner

@Composable
private fun NotDeviceOwnerContent(state: MastUiState, actions: MastActions) {
    val colors = MaterialTheme.colorScheme
    HeroCard {
        HeroBadge(Icons.Outlined.Terminal, container = colors.surfaceContainerHigh, content = colors.onSurface)
        HeroTitle(stringResource(R.string.setup_title), color = colors.onSurface)
        HeroBody(stringResource(R.string.setup_body))
    }
    SectionCard {
        Overline(stringResource(R.string.setup_overline))
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            SetupStep(1, stringResource(R.string.setup_step_1))
            SetupStep(2, stringResource(R.string.setup_step_2))
            SetupStep(3, stringResource(R.string.setup_step_3))
        }
        Spacer(Modifier.height(MastSpacing.m))
        CommandBlock(stringResource(R.string.setup_command))
    }
    WideButton(stringResource(R.string.action_check_again), onClick = actions.onCheckDeviceOwner)
    DetailsCard(detailItems(state))
}

// ------------------------------------------------------------------ 2. Not paired

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun NotPairedContent(state: MastUiState, actions: MastActions) {
    val colors = MaterialTheme.colorScheme
    val pathDescription = stringResource(R.string.pair_path_description)
    HeroCard {
        HeroBadge(Icons.Outlined.Anchor, container = colors.surfaceContainerHigh, content = colors.onSurface)
        HeroTitle(stringResource(R.string.pair_title), color = colors.onSurface)
        HeroBody(stringResource(R.string.pair_body))
        Spacer(Modifier.height(14.dp))
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
            modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = pathDescription },
        ) {
            Crumb(stringResource(R.string.pair_path_familywarden))
            CrumbSeparator()
            Crumb(stringResource(R.string.pair_path_settings))
            CrumbSeparator()
            Crumb(stringResource(R.string.pair_path_ulysses))
        }
    }
    WideButton(
        text = stringResource(R.string.action_scan),
        icon = Icons.Outlined.QrCodeScanner,
        onClick = actions.onScanPairingCode,
    )
    TextButton(
        onClick = actions.onPairManually,
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = MastSpacing.buttonHeight),
    ) { Text(stringResource(R.string.action_pair_manually)) }
    Surface(shape = MaterialTheme.shapes.medium, color = colors.surfaceContainer, modifier = Modifier.fillMaxWidth()) {
        Row(Modifier.padding(horizontal = MastSpacing.l, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.WifiTethering, contentDescription = null, tint = colors.tertiary, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(MastSpacing.m))
            Text(stringResource(R.string.pair_unpaired_status), style = MaterialTheme.typography.bodySmall)
        }
    }
    DetailsCard(detailItems(state))
    TextButton(
        onClick = actions.onRemove,
        modifier = Modifier.fillMaxWidth().heightIn(min = MastSpacing.buttonHeight),
    ) { Text(stringResource(R.string.action_remove), color = colors.error) }
}

@Composable
private fun Crumb(text: String) {
    Surface(shape = MaterialTheme.shapes.extraSmall, color = MaterialTheme.colorScheme.surfaceContainerHigh) {
        Text(
            text,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

@Composable
private fun CrumbSeparator() {
    Text("›", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

// ------------------------------------------------------------------ 4-8. Paired

@Composable
private fun PairedContent(status: PactStatus, state: MastUiState, actions: MastActions) {
    val bound = status is PactStatus.Bound
    SyncBanner(state.sync, state.details, state.nowMillis)
    StatusHero(status, state.nowMillis)
    if (status is PactStatus.Bound) {
        PactCard(status.pact, state.details.captain)
    }
    FilledTonalButton(
        onClick = actions.onSync,
        enabled = !state.isSyncing,
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = MastSpacing.buttonHeight),
    ) {
        Icon(Icons.Outlined.Sync, contentDescription = null, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(MastSpacing.s))
        Text(
            stringResource(
                when {
                    state.isSyncing -> R.string.action_syncing
                    state.sync is SyncState.Unreachable -> R.string.action_try_again
                    else -> R.string.action_sync
                },
            ),
        )
    }
    if (bound) {
        // Pairing and removal are not offered while bound: one quiet line instead of
        // a row of greyed-out buttons.
        InfoNote(Icons.Outlined.Lock, stringResource(R.string.pact_locked_note))
        DetailsCard(detailItems(state))
    } else {
        DetailsCard(detailItems(state))
        Spacer(Modifier.height(MastSpacing.xs))
        OutlinedButton(
            onClick = actions.onScanPairingCode,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = MastSpacing.buttonHeight),
        ) {
            Icon(Icons.Outlined.QrCodeScanner, contentDescription = null, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(MastSpacing.s))
            Text(stringResource(R.string.action_pair_other))
        }
        TextButton(
            onClick = actions.onPairManually,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = MastSpacing.buttonHeight),
        ) { Text(stringResource(R.string.action_pair_manually)) }
        OutlinedButton(
            onClick = actions.onRemove,
            colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = MastSpacing.buttonHeight),
        ) {
            Icon(Icons.Outlined.DeleteOutline, contentDescription = null, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(MastSpacing.s))
            Text(stringResource(R.string.action_remove))
        }
    }
}

// ------------------------------------------------------------------ Shared bits

@Composable
private fun WideButton(text: String, onClick: () -> Unit, icon: ImageVector? = null) {
    Button(
        onClick = onClick,
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = MastSpacing.buttonHeight),
    ) {
        if (icon != null) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(MastSpacing.s))
        }
        Text(text)
    }
}

/** Secondary details: captain, router, key, last sync, device owner. */
@Composable
private fun detailItems(state: MastUiState): List<DetailItem> {
    val d = state.details
    val trusted = state.sync is SyncState.BadSignature
    return buildList {
        d.captain?.let { add(DetailItem(stringResource(R.string.details_captain), it)) }
        if (d.isDeviceOwner) {
            add(
                DetailItem(
                    stringResource(R.string.details_router),
                    d.routerHost ?: stringResource(R.string.details_not_paired),
                    mono = d.routerHost != null,
                ),
            )
        }
        d.fingerprint?.let {
            add(DetailItem(stringResource(R.string.details_key), groupFingerprint(it), mono = true, spoken = spokenFingerprint(it)))
        }
        if (d.routerHost != null) {
            add(
                DetailItem(
                    stringResource(if (trusted) R.string.details_last_trusted_sync else R.string.details_last_sync),
                    d.lastSyncMillis?.let { formatWhenCapitalized(it, state.nowMillis) }
                        ?: stringResource(R.string.details_never),
                ),
            )
        }
        add(
            DetailItem(
                stringResource(R.string.details_device_owner),
                stringResource(if (d.isDeviceOwner) R.string.details_yes else R.string.details_no),
            ),
        )
    }
}
