package io.github.denogio.familywarden.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.text.style.TextOverflow
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.BuildConfig
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastSpacing
import io.github.denogio.familywarden.ui.theme.MonoLarge
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import java.time.Instant
import java.time.ZoneId

private enum class Page(val title: Int, val icon: ImageVector) {
    Overview(R.string.fw_overview, Icons.Outlined.Explore),
    Dns(R.string.fw_dns, Icons.Outlined.Shield),
    Ulysses(R.string.fw_ulysses, MastIcons.Mast),
    Settings(R.string.fw_settings, Icons.Outlined.Tune),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FamilyWardenScreen(state: FamilyWardenState, actions: FamilyWardenActions) {
    var selected by rememberSaveable { mutableStateOf(Page.Overview.name) }
    var featureSheet by rememberSaveable { mutableStateOf(false) }
    var enableConfirmation by rememberSaveable { mutableStateOf(false) }
    val pages = Page.entries.filter { it != Page.Ulysses || state.showUlysses }
    val page = pages.firstOrNull { it.name == selected } ?: Page.Overview
    val readyForNavigation = state.owner && state.paired
    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        if (page == Page.Overview || !readyForNavigation) {
                            Icon(painterResource(R.drawable.ic_lighthouse), null, tint = Color.Unspecified, modifier = Modifier.size(28.dp))
                            Spacer(Modifier.width(10.dp))
                        }
                        Text(stringResource(if (page == Page.Overview || !readyForNavigation) R.string.fw_name else page.title),
                            style = MaterialTheme.typography.titleLarge)
                    }
                },
                actions = {
                    if (state.paired) IconButton(onClick = actions.sync, enabled = !state.syncing) {
                        if (state.syncing) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                        else Icon(Icons.Outlined.Sync, stringResource(R.string.action_sync))
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
        bottomBar = {
            if (readyForNavigation) NavigationBar {
                pages.forEach { p -> NavigationBarItem(
                    selected = p == page,
                    onClick = { selected = p.name },
                    icon = { Icon(p.icon, null) },
                    label = { Text(stringResource(p.title), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                ) }
            }
        },
    ) { padding ->
        val scroll = key(page, readyForNavigation) { rememberScrollState() }
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(scroll)
                .padding(horizontal = MastSpacing.gutter, vertical = MastSpacing.s),
            verticalArrangement = Arrangement.spacedBy(MastSpacing.m),
        ) {
            when {
                !state.owner -> OwnerSetup(actions)
                !state.paired -> PairSetup(actions)
                page == Page.Overview -> {
                    DnsHero(state)
                    if (state.showUlysses) {
                        StatusHero(state.pact ?: PactStatus.FreeSailing, state.nowMillis)
                        TextButton(onClick = { selected = Page.Ulysses.name }, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.fw_open_ulysses))
                        }
                    }
                    Facts(state)
                    WideAction(R.string.action_sync, Icons.Outlined.Sync, actions.sync, !state.syncing)
                }
                page == Page.Dns -> DnsContent(state, actions)
                page == Page.Ulysses -> UlyssesContent(state, actions)
                page == Page.Settings -> SettingsContent(state, actions) { featureSheet = true }
            }
            Spacer(Modifier.height(16.dp))
        }
    }
    MastDialogHost(state.dialog, MastActions(
        onSubmitAddress = actions.submitAddress,
        onConfirmPairing = actions.confirmPair,
        onDismissDialog = actions.dismiss,
        onConfirmRemove = actions.confirmRemove,
    ))
    if (featureSheet) ModalBottomSheet(onDismissRequest = { featureSheet = false }) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text(stringResource(R.string.fw_ulysses), style = MaterialTheme.typography.headlineSmall)
            Text(stringResource(R.string.fw_ulysses_description))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.fw_ulysses), modifier = Modifier.weight(1f))
                Switch(
                    checked = state.showUlysses,
                    enabled = !state.bound && state.binding?.captain == true,
                    onCheckedChange = { enabled ->
                        if (enabled) enableConfirmation = true else actions.toggleUlysses(false)
                    },
                )
            }
            if (state.bound) InfoNote(Icons.Outlined.Lock, stringResource(R.string.pact_locked_note))
            else if (state.binding?.captain != true) InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_ulysses_eligibility))
            Spacer(Modifier.height(16.dp))
        }
    }
    if (enableConfirmation) AlertDialog(
        onDismissRequest = { enableConfirmation = false },
        title = { Text(stringResource(R.string.fw_ulysses_enable_title)) },
        text = { Text(stringResource(R.string.fw_ulysses_enable_body)) },
        confirmButton = { TextButton(onClick = { enableConfirmation = false; actions.toggleUlysses(true) }) { Text(stringResource(R.string.action_continue)) } },
        dismissButton = { TextButton(onClick = { enableConfirmation = false }) { Text(stringResource(R.string.action_cancel)) } },
    )
}

@Composable
private fun DnsHero(s: FamilyWardenState) {
    if (s.headline == DnsHeadline.AwaitingApproval) {
        HeroCard {
            HeroBadge(Icons.Outlined.HourglassEmpty, MaterialTheme.colorScheme.surfaceContainerHigh, MaterialTheme.colorScheme.onSurface)
            HeroTitle(stringResource(R.string.fw_awaiting_title))
            HeroBody(stringResource(R.string.fw_awaiting_body))
            Text(groupFingerprint(s.phoneFingerprint.orEmpty()), style = MonoLarge,
                modifier = Modifier.padding(top = 12.dp).semantics {
                    contentDescription = spokenFingerprint(s.phoneFingerprint.orEmpty())
                })
        }
        return
    }
    val (title, body) = when (s.headline) {
        DnsHeadline.AwaitingApproval -> error("handled above")
        DnsHeadline.RouterUnsupported -> R.string.fw_router_unsupported_title to R.string.fw_router_unsupported_body
        DnsHeadline.PolicyMissing -> R.string.fw_policy_unavailable_title to R.string.fw_policy_unavailable_body
        DnsHeadline.NotConfigured -> R.string.fw_dns_missing_title to R.string.fw_dns_missing_body
        DnsHeadline.NotApplied -> R.string.fw_dns_not_applied to R.string.fw_dns_not_applied_body
        DnsHeadline.Unlocked -> R.string.fw_dns_unlocked to R.string.fw_dns_unlocked_body
        DnsHeadline.ApplyFailed -> R.string.fw_dns_failed to R.string.fw_dns_failed_body
        DnsHeadline.Testing -> R.string.fw_dns_testing to R.string.fw_dns_testing_body
        DnsHeadline.CertificateError -> R.string.fw_dns_certificate to R.string.fw_dns_certificate_body
        DnsHeadline.Unavailable -> R.string.fw_dns_unavailable to R.string.fw_dns_unavailable_body
        DnsHeadline.DnsError -> R.string.fw_dns_answer_error to R.string.fw_dns_answer_error_body
        DnsHeadline.AppliedAndTested -> R.string.fw_dns_ok to R.string.fw_dns_ok_body
        DnsHeadline.AppliedUntested -> R.string.fw_dns_untested to R.string.fw_dns_untested_body
    }
    val problem = s.headline in setOf(DnsHeadline.ApplyFailed, DnsHeadline.Unlocked,
        DnsHeadline.CertificateError, DnsHeadline.Unavailable, DnsHeadline.DnsError)
    val colors = MaterialTheme.colorScheme
    HeroCard(color = when {
        problem -> colors.errorContainer
        s.headline == DnsHeadline.AppliedAndTested -> colors.tertiaryContainer
        else -> colors.surfaceContainer
    }) {
        HeroBadge(if (problem) Icons.Outlined.WarningAmber else Icons.Outlined.Shield,
            if (problem) colors.errorContainer else colors.surfaceContainerHigh,
            if (problem) colors.error else colors.tertiary)
        HeroTitle(stringResource(title), colors.onSurface)
        HeroBody(stringResource(body))
    }
}

@Composable
private fun Facts(s: FamilyWardenState) {
    val policy = if (s.binding != null) formatWhenCapitalized(s.lastSync * 1000, s.nowMillis)
        else stringResource(R.string.fw_policy_waiting)
    val applied = when {
        s.desiredDns == null -> stringResource(R.string.fw_dns_not_configured)
        s.appliedDnsHost != s.desiredDns.hostname -> stringResource(R.string.fw_dns_not_applied)
        !s.dnsLocked -> stringResource(R.string.fw_dns_unlocked)
        else -> stringResource(R.string.fw_dns_mode)
    }
    val check = s.currentCheck
    val health = when (check?.result) {
        ProbeResult.ANSWERED -> stringResource(R.string.fw_last_test, formatWhen(check.testedAt, s.nowMillis))
        ProbeResult.TESTING -> stringResource(R.string.fw_dns_testing)
        ProbeResult.CERTIFICATE -> stringResource(R.string.fw_dns_certificate)
        ProbeResult.UNAVAILABLE -> stringResource(R.string.fw_dns_unavailable)
        ProbeResult.DNS_ERROR -> stringResource(R.string.fw_dns_answer_error)
        null -> stringResource(R.string.fw_not_tested)
    }
    val sync = when {
        s.syncing -> stringResource(R.string.action_syncing)
        s.syncError == "router unreachable" -> stringResource(R.string.fw_router_offline)
        s.syncError == "bad signature" -> stringResource(R.string.fw_sync_bad_signature)
        s.syncError == "device not approved" -> stringResource(R.string.fw_sync_not_approved)
        s.syncError != null -> stringResource(R.string.fw_sync_failed)
        s.lastSync > 0 -> formatWhenCapitalized(s.lastSync * 1000, s.nowMillis)
        else -> stringResource(R.string.details_never)
    }
    DetailsCard(listOf(
        DetailItem(stringResource(R.string.fw_policy_verified), policy),
        DetailItem(stringResource(R.string.fw_applied), applied),
        DetailItem(stringResource(R.string.fw_resolver_reachable), health),
        DetailItem(stringResource(R.string.fw_router_reachable), sync),
    ))
}

@Composable
private fun DnsContent(s: FamilyWardenState, actions: FamilyWardenActions) {
    DnsHero(s)
    DetailsCard(listOf(
        DetailItem(stringResource(R.string.pair_manual_label), s.desiredDns?.hostname ?: stringResource(R.string.details_never), mono = true),
        DetailItem(stringResource(R.string.fw_dns), stringResource(
            if (s.desiredDns != null) R.string.fw_dns_mode else R.string.fw_dns_not_configured)),
    ))
    WideAction(R.string.fw_test_connection, Icons.Outlined.NetworkCheck, actions.testResolver,
        s.canTestResolver && s.desiredDns != null && s.currentCheck?.result != ProbeResult.TESTING)
    if (s.desiredDns != null && !s.canTestResolver) InfoNote(Icons.Outlined.Info,
        stringResource(R.string.fw_probe_unavailable))
    if (s.desiredDns != null) SectionCard {
        Overline(stringResource(R.string.fw_policy_contents))
        s.desiredDns.protections.sortedBy { it.ordinal }.forEach { protection ->
            Text("• " + stringResource(when (protection) {
                Protection.FILTER_LISTS -> R.string.fw_filter_lists
                Protection.SAFE_BROWSING -> R.string.fw_safe_browsing
                Protection.PARENTAL -> R.string.fw_parental
                Protection.SAFE_SEARCH -> R.string.fw_safe_search
            }), modifier = Modifier.padding(vertical = 3.dp))
        }
    }
    InfoNote(Icons.Outlined.Lock, stringResource(R.string.fw_always_on))
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_dns_limit))
}

@Composable
private fun UlyssesContent(s: FamilyWardenState, actions: FamilyWardenActions) {
    StatusHero(s.pact ?: PactStatus.FreeSailing, s.nowMillis)
    if (s.pact is PactStatus.Bound) PactCard(s.pact.pact, s.binding?.profileName)
    s.rules?.let { rules ->
        val zone = ZoneId.of(s.timeZone ?: ZoneId.systemDefault().id)
        val date = Instant.ofEpochMilli(s.nowMillis).atZone(zone).toLocalDate()
        val intervals = remember(rules, date, zone) { daySchedule(rules, date, zone) }
        SectionCard {
            Overline(stringResource(R.string.fw_schedule_today))
            val color = MaterialTheme.colorScheme.tertiary
            Canvas(Modifier.fillMaxWidth().height(20.dp).clearAndSetSemantics { }) {
                intervals.forEach { window ->
                    drawRect(color, topLeft = androidx.compose.ui.geometry.Offset(size.width * window.startMinute / 1440f, 0f),
                        size = androidx.compose.ui.geometry.Size(size.width * (window.endExclusive - window.startMinute) / 1440f, size.height))
                }
            }
            Spacer(Modifier.height(8.dp))
            Text(if (intervals.isEmpty()) stringResource(R.string.fw_schedule_none) else intervals.joinToString(" · ") {
                fun clock(n: Int) = "%02d:%02d".format(n / 60, n % 60)
                clock(it.startMinute) + "–" + clock(it.endExclusive)
            }, style = MaterialTheme.typography.bodyMedium)
            Text(stringResource(R.string.fw_timezone, zone.id), style = MaterialTheme.typography.bodySmall)
        }
    }
    WideAction(R.string.fw_open_sail, Icons.Outlined.Sailing, actions.openSail, s.host != null)
    InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_sail_note))
}

@Composable
private fun SettingsContent(s: FamilyWardenState, actions: FamilyWardenActions, features: () -> Unit) {
    SectionCard {
        Overline(stringResource(R.string.fw_features))
        TextButton(onClick = features, modifier = Modifier.fillMaxWidth()) {
            Icon(MastIcons.Mast, null, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(10.dp))
            Text(stringResource(R.string.fw_ulysses))
            Spacer(Modifier.weight(1f))
            if (s.bound) Icon(Icons.Outlined.Lock, stringResource(R.string.pact_locked_note))
            else Icon(Icons.Outlined.ChevronRight, null)
        }
    }
    DetailsCard(listOf(
        DetailItem(stringResource(R.string.fw_router_address), s.host.orEmpty(), mono = true),
        DetailItem(stringResource(R.string.details_key), groupFingerprint(s.fingerprint.orEmpty()), mono = true,
            spoken = spokenFingerprint(s.fingerprint.orEmpty())),
        DetailItem(stringResource(R.string.fw_profile), s.binding?.profileName ?: stringResource(R.string.fw_profile_unknown)),
        DetailItem(stringResource(R.string.fw_phone_key), groupFingerprint(s.phoneFingerprint.orEmpty()), mono = true,
            spoken = spokenFingerprint(s.phoneFingerprint.orEmpty())),
        DetailItem(stringResource(R.string.details_device_owner), stringResource(if (s.owner) R.string.details_yes else R.string.details_no)),
    ))
    s.binding?.let { b -> SectionCard {
        Text(stringResource(R.string.fw_role_managed))
        if (b.captain) Text(stringResource(R.string.fw_role_captain))
        if (b.administrator) Text(stringResource(R.string.fw_role_admin))
    } }
    WideAction(R.string.action_scan, Icons.Outlined.QrCodeScanner, actions.scan, s.canScan)
    TextButton(onClick = actions.manualPair, enabled = s.canScan, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.action_pair_manually))
    }
    WideAction(R.string.fw_diagnostics, Icons.Outlined.ContentCopy, actions.copyDiagnostics)
    Text(stringResource(R.string.fw_version, BuildConfig.VERSION_NAME), style = MaterialTheme.typography.bodySmall)
    if (!s.provisioned) InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_setup_incomplete))
    else if (!s.canRequestRemoval) InfoNote(Icons.Outlined.Lock,
        stringResource(if (s.bound) R.string.pact_locked_note else R.string.fw_no_removal_authority))
    OutlinedButton(onClick = actions.remove, enabled = s.canRequestRemoval,
        modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error)) {
        Text(stringResource(R.string.action_remove))
    }
}

@Composable
private fun OwnerSetup(actions: FamilyWardenActions) {
    HeroCard {
        HeroTitle(stringResource(R.string.setup_title))
        HeroBody(stringResource(R.string.setup_body))
    }
    InfoNote(Icons.Outlined.WarningAmber, stringResource(R.string.fw_migration_note))
    SectionCard {
        SetupStep(1, stringResource(R.string.setup_step_1))
        SetupStep(2, stringResource(R.string.setup_step_2))
        SetupStep(3, stringResource(R.string.setup_step_3))
        Spacer(Modifier.height(16.dp))
        CommandBlock(stringResource(R.string.setup_command))
    }
    WideAction(R.string.action_check_again, Icons.Outlined.Refresh, actions.sync)
}

@Composable
private fun PairSetup(actions: FamilyWardenActions) {
    HeroCard {
        HeroBadge(Icons.Outlined.Router, MaterialTheme.colorScheme.surfaceContainerHigh, MaterialTheme.colorScheme.onSurface)
        HeroTitle(stringResource(R.string.pair_title))
        HeroBody(stringResource(R.string.pair_body))
    }
    WideAction(R.string.action_scan, Icons.Outlined.QrCodeScanner, actions.scan)
    TextButton(onClick = actions.manualPair, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_pair_manually)) }
    TextButton(onClick = actions.remove, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_remove)) }
}

@Composable
private fun WideAction(text: Int, icon: ImageVector, onClick: () -> Unit, enabled: Boolean = true) {
    FilledTonalButton(onClick = onClick, enabled = enabled,
        modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp)) {
        Icon(icon, null, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(8.dp))
        Text(stringResource(text))
    }
}
