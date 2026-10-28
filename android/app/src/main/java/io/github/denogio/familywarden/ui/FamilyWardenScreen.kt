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
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.BuildConfig
import io.github.denogio.familywarden.ui.components.*
import io.github.denogio.familywarden.ui.theme.MastSpacing
import io.github.denogio.familywarden.ui.theme.MonoLarge
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import java.time.Instant
import java.time.ZoneId

private enum class Page(val title: Int, val icon: ImageVector) {
    Overview(R.string.fw_overview, Icons.Outlined.Explore),
    FamilyTime(R.string.fw_family_screen_time, Icons.Outlined.BarChart),
    Controls(R.string.fw_family_controls, Icons.Outlined.Tune),
    History(R.string.fw_history, Icons.Outlined.BarChart),
    Requests(R.string.fw_requests, Icons.Outlined.Inbox),
    Dns(R.string.fw_dns, Icons.Outlined.Shield),
    Ulysses(R.string.fw_ulysses, MastIcons.Mast),
    Settings(R.string.fw_settings, Icons.Outlined.Tune),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FamilyWardenScreen(state: FamilyWardenState, actions: FamilyWardenActions, initialRequests: Boolean = false) {
    var selected by rememberSaveable { mutableStateOf(if (initialRequests) Page.Requests.name else Page.Overview.name) }
    var familySelected by rememberSaveable { mutableStateOf(if (initialRequests) Page.Requests.name else Page.FamilyTime.name) }
    var familyMode by rememberSaveable { mutableStateOf(initialRequests) }
    val showFamily = state.canManageFamily && (state.parentMode || familyMode)
    var selectedChild by rememberSaveable(state.binding?.deviceId) { mutableStateOf<String?>(null) }
    val familyProfile = state.familyProfile(selectedChild)
    LaunchedEffect(familyProfile?.id) { selectedChild = familyProfile?.id }
    var featureSheet by rememberSaveable { mutableStateOf(false) }
    var asking by rememberSaveable { mutableStateOf(false) }
    var enableConfirmation by rememberSaveable { mutableStateOf(false) }
    val childNavigation = state.appsActive && state.binding?.captain == false
    val pages = if (showFamily) listOf(Page.FamilyTime, Page.Controls, Page.Requests) else Page.entries.filter {
        when (it) {
            Page.FamilyTime, Page.Controls -> false
            Page.Overview -> !state.parentMode
            Page.Requests -> false // own requests start from Today; the inbox is a family page
            Page.History -> childNavigation
            Page.Dns -> !childNavigation && !state.parentMode
            Page.Ulysses -> state.showUlysses
            else -> true
        }
    }
    val page = pages.firstOrNull { it.name == (if (showFamily) familySelected else selected) } ?: pages.first()
    val readyForNavigation = (state.owner || state.parentMode) && state.paired
    // The ask flow is a full screen above Today, with its own back arrow.
    val askFlow = asking && readyForNavigation && !showFamily && page == Page.Overview && state.screenTime != null
    androidx.activity.compose.BackHandler(enabled = askFlow) { asking = false }
    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            if (!askFlow) Surface(color = MaterialTheme.colorScheme.background) {
                Row(Modifier.fillMaxWidth().windowInsetsPadding(WindowInsets.statusBars)
                    .padding(horizontal = 16.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Row(Modifier.weight(1f), verticalAlignment = Alignment.CenterVertically) {
                        if (page == Page.Overview || !readyForNavigation) {
                            Icon(painterResource(R.drawable.ic_lighthouse), null, tint = Color.Unspecified, modifier = Modifier.size(28.dp))
                            Spacer(Modifier.width(10.dp))
                        }
                        Column {
                            Text(stringResource(if (!readyForNavigation) R.string.fw_name
                                else if (showFamily && page == Page.Requests) R.string.fw_family_requests
                                else if (page == Page.Overview && state.screenTime != null) R.string.fw_today else page.title),
                                style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
                            // Today names the router-local weekday and time, as on the design boards.
                            if (readyForNavigation && page == Page.History) Text(stringResource(R.string.fw_history_last_30),
                                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            state.screenTime?.takeIf { readyForNavigation && page == Page.Overview }?.let { time ->
                                val locale = androidx.compose.ui.platform.LocalConfiguration.current.locales[0]
                                val at = Instant.ofEpochMilli(state.nowMillis).atZone(time.resetsAt.zone)
                                Text(at.format(java.time.format.DateTimeFormatter.ofPattern("EEEE", locale)) + " · " +
                                    at.format(java.time.format.DateTimeFormatter.ofLocalizedTime(java.time.format.FormatStyle.SHORT).withLocale(locale)),
                                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                    if (readyForNavigation && state.canManageFamily && !state.parentMode) {
                        TextButton(onClick = { familyMode = !showFamily }) {
                            BadgedBox(badge = { if (!showFamily && state.familyPendingCount > 0) Badge { Text(state.familyPendingCount.toString()) } }) {
                                Text(stringResource(if (showFamily) R.string.fw_my_phone else R.string.fw_family))
                            }
                        }
                    }
                    if (state.paired) IconButton(onClick = actions.sync, enabled = !state.syncing) {
                        if (state.syncing) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                        else Icon(Icons.Outlined.Sync, stringResource(R.string.action_sync))
                    }
                }
            }
        },
        bottomBar = {
            if (readyForNavigation && !askFlow) NavigationBar {
                pages.forEach { p -> NavigationBarItem(
                    selected = p == page,
                    onClick = { if (showFamily) familySelected = p.name else selected = p.name },
                    // Keep localized screen-reader names without visible labels, at every font scale.
                    icon = {
                        BadgedBox(badge = { if (showFamily && p == Page.Requests && state.familyPendingCount > 0) Badge { Text(state.familyPendingCount.toString()) } }) {
                            Icon(if (p == Page.Overview && state.screenTime != null) Icons.Outlined.WbSunny else p.icon,
                                stringResource(if (showFamily && p == Page.Requests) R.string.fw_family_requests
                                else if (p == Page.Overview && state.screenTime != null) R.string.fw_today else p.title))
                        }
                    },
                ) }
            }
        },
    ) { padding ->
        val scroll = key(page, readyForNavigation,
            if (showFamily && page != Page.Requests) familyProfile?.id else null) { rememberScrollState() }
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(scroll)
                .padding(horizontal = MastSpacing.gutter, vertical = MastSpacing.s),
            verticalArrangement = Arrangement.spacedBy(MastSpacing.m),
        ) {
            if (readyForNavigation && showFamily && page != Page.Requests) {
                FamilyChildSelector(state, familyProfile?.id) { selectedChild = it }
            }
            when {
                askFlow -> AskFlow(state, actions) { asking = false }
                !state.owner && !state.parentMode -> OwnerSetup(actions, !state.provisioned)
                !state.paired -> PairSetup(actions, state.parentMode)
                page == Page.FamilyTime -> FamilyTimeContent(state, familyProfile?.id)
                page == Page.Controls -> FamilyControlsContent(state, actions, familyProfile?.id)
                page == Page.Requests -> RequestsContent(state, actions, family = showFamily)
                page == Page.Settings && state.parentMode -> ParentSettings(state, actions)
                page == Page.Overview && state.screenTime != null -> ScreenTimeContent(state, actions) { asking = true }
                page == Page.History -> ScreenTimeHistory(state.screenTime)
                page == Page.Overview -> {
                    DnsHero(state)
                    if (state.showUlysses) {
                        StatusHero(state.pact ?: PactStatus.FreeSailing, state.nowMillis)
                        TextButton(onClick = { selected = Page.Ulysses.name }, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.fw_open_ulysses))
                        }
                    }
                    Facts(state)
                    AppLimitsNote(state, actions)
                    WideAction(R.string.action_sync, Icons.Outlined.Sync, actions.sync, !state.syncing)
                }
                page == Page.Dns -> DnsContent(state, actions)
                page == Page.Ulysses -> UlyssesContent(state, actions)
                page == Page.Settings -> {
                    if (childNavigation) ChildSettingsContent(state, actions) { featureSheet = true }
                    else SettingsContent(state, actions) { featureSheet = true }
                }
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
        s.syncError == "unsupported capabilities" -> stringResource(R.string.fw_sync_upgrade_required)
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

/** App limits are enforced locally; only problems need the user's attention. */
@Composable
private fun AppLimitsNote(s: FamilyWardenState, actions: FamilyWardenActions) {
    if (!s.appsActive) return
    when (s.appsStatus) {
        "usage_access_missing" -> {
            InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_usage_missing))
            TextButton(onClick = actions.openUsageAccess, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.fw_apps_open_usage))
            }
        }
        "suspend_failed" -> InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_suspend_failed))
        "ok" -> InfoNote(Icons.Outlined.Lock, stringResource(R.string.fw_apps_active))
        else -> InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_time_unavailable))
    }
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
private fun ChildSettingsContent(s: FamilyWardenState, actions: FamilyWardenActions, features: () -> Unit) {
    var detail by rememberSaveable { mutableStateOf<String?>(null) }
    androidx.activity.compose.BackHandler(enabled = detail != null) { detail = null }
    if (detail != null) {
        TextButton(onClick = { detail = null }) {
            Icon(Icons.Outlined.ArrowBack, null)
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.fw_settings))
        }
        if (detail == "dns") DnsContent(s, actions)
        else {
            Facts(s)
            SettingsContent(s, actions, features)
        }
        return
    }
    SectionCard {
        TextButton(onClick = { detail = "dns" }, modifier = Modifier.fillMaxWidth()) {
            Icon(Icons.Outlined.Shield, null)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.fw_dns), style = MaterialTheme.typography.titleMedium)
                Text(stringResource(if (s.headline == DnsHeadline.AppliedAndTested) R.string.fw_dns_ok
                    else R.string.fw_review_dns), style = MaterialTheme.typography.bodySmall)
            }
            Icon(Icons.Outlined.ChevronRight, null)
        }
        HorizontalDivider()
        TextButton(onClick = { detail = "router" }, modifier = Modifier.fillMaxWidth()) {
            Icon(Icons.Outlined.Home, null)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.fw_home_router), style = MaterialTheme.typography.titleMedium)
                Text(if (s.syncError == "router unreachable") stringResource(R.string.fw_router_offline)
                    else if (s.syncError != null) stringResource(R.string.fw_sync_failed)
                    else if (s.lastSync > 0) stringResource(R.string.fw_rules_received, formatWhen(s.lastSync * 1000, s.nowMillis))
                    else stringResource(R.string.fw_policy_waiting), style = MaterialTheme.typography.bodySmall)
            }
            Icon(Icons.Outlined.ChevronRight, null)
        }
        HorizontalDivider()
        Row(Modifier.padding(horizontal = 12.dp, vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.People, null)
            Spacer(Modifier.width(12.dp))
            Text(stringResource(R.string.fw_managed_family), style = MaterialTheme.typography.titleMedium)
        }
    }
    Text(stringResource(R.string.fw_child_settings_note), style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant)
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
        if (b.approves.isNotEmpty()) Text(stringResource(R.string.fw_role_parent, b.approves.joinToString(", ") { it.name }))
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
private fun OwnerSetup(actions: FamilyWardenActions, allowParent: Boolean) {
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
    if (allowParent) FilledTonalButton(onClick = actions.chooseParentMode, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.fw_parent_setup))
    }
}

@Composable
private fun PairSetup(actions: FamilyWardenActions, parentMode: Boolean = false) {
    HeroCard {
        HeroBadge(Icons.Outlined.Router, MaterialTheme.colorScheme.surfaceContainerHigh, MaterialTheme.colorScheme.onSurface)
        HeroTitle(stringResource(R.string.pair_title))
        HeroBody(stringResource(R.string.pair_body))
    }
    WideAction(R.string.action_scan, Icons.Outlined.QrCodeScanner, actions.scan)
    TextButton(onClick = actions.manualPair, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_pair_manually)) }
    TextButton(onClick = if (parentMode) actions.parentSignOut else actions.remove, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(if (parentMode) R.string.fw_parent_disconnect else R.string.action_remove))
    }
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
