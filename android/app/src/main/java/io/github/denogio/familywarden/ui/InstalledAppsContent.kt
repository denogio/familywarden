package io.github.denogio.familywarden.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.components.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

private sealed interface AppsView {
    data object Loading : AppsView
    data class Loaded(val apps: InstalledApps) : AppsView
    data class Failed(val code: String?) : AppsView
}

private enum class AppFilter(val label: Int) { All(R.string.fw_apps_filter_all), Limited(R.string.fw_apps_filter_limited),
    Blocked(R.string.fw_apps_filter_blocked), Always(R.string.fw_apps_filter_always), New(R.string.fw_apps_filter_new) }

/** A report older than a day is said, never hidden. */
private const val STALE_SECONDS = 24 * 3600L

internal fun InstalledApp.essentialAnywhere() = essential.isNotEmpty()
/** New-app approval pauses user apps only; system apps and essential roles are never waiting. */
internal fun InstalledApp.waiting(suspendNew: Boolean) =
    suspendNew && rule == AppRule.None && on.isNotEmpty() && essential.isEmpty() && system.size < on.size

/** Browse reported apps and edit profile-wide standing rules through signed CAS writes. */
@Composable
internal fun InstalledAppsContent(s: FamilyWardenState, actions: FamilyWardenActions, preferred: String?,
    selected: String?, onSelect: (String?) -> Unit, back: () -> Unit) {
    if (selected == null) PendingRuleStatus(s, actions)
    val profile = s.familyProfile(preferred)
    if (profile == null) {
        TextButton(onClick = back) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, null); Spacer(Modifier.width(8.dp)); Text(stringResource(R.string.fw_family_controls)) }
        Text(stringResource(if (s.binding == null) R.string.fw_request_waiting_approval else R.string.fw_request_no_scope))
        LaunchedEffect(Unit) { if (selected != null) onSelect(null) }
        return
    }
    val key = listOf(profile.id, s.binding?.deviceId, s.fingerprint, s.phoneFingerprint, s.binding?.approves?.map { it.id }?.sorted().toString())
    var retry by remember { mutableIntStateOf(0) }
    var view by remember(key) { mutableStateOf<AppsView>(AppsView.Loading) }
    val loader by rememberUpdatedState(actions.loadApps)
    LaunchedEffect(key, retry, s.lastSync) {
        if (view !is AppsView.Loaded) view = AppsView.Loading
        view = try {
            withContext(Dispatchers.IO) { loader(profile.id) }.also { require(it.profile.id == profile.id) }.let { AppsView.Loaded(it) }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (e: Exception) { AppsView.Failed((e as? InstalledRefused)?.code) }
    }
    val current = view
    val app = (current as? AppsView.Loaded)?.apps?.apps?.firstOrNull { it.pkg == selected }
    // A different child or a vanished app never keeps an open editor.
    var opened by remember { mutableStateOf(profile.id) }
    var gone by remember(key) { mutableStateOf<String?>(null) }
    LaunchedEffect(profile.id, current, selected) {
        if (opened != profile.id) { opened = profile.id; if (selected != null) onSelect(null) }
        else if (selected != null && current is AppsView.Loaded && app == null) { gone = selected; onSelect(null) }
    }
    if (selected != null && app != null) {
        AppDetail(s, actions, profile, current.apps, app, { retry++ }) { onSelect(null) }
        return
    }
    TextButton(onClick = back) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, null); Spacer(Modifier.width(8.dp)); Text(stringResource(R.string.fw_family_controls)) }
    Text(stringResource(R.string.fw_apps_installed_on, profile.name), style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant)
    when (current) {
        AppsView.Loading -> SectionCard { CircularProgressIndicator(Modifier.size(24.dp)); Text(stringResource(R.string.fw_apps_loading)) }
        is AppsView.Failed -> SectionCard {
            Text(stringResource(when (current.code) {
                "not_parent" -> R.string.fw_history_scope_changed
                "app_control_off" -> R.string.fw_apps_control_off
                else -> R.string.fw_apps_load_failed
            }))
            TextButton(onClick = { retry++ }) { Text(stringResource(R.string.action_try_again)) }
        }
        is AppsView.Loaded -> {
            gone?.let { InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_gone, it)) }
            AppList(s, profile, current.apps) { gone = null; onSelect(it) }
        }
    }
    InfoNote(Icons.Outlined.Groups, stringResource(R.string.fw_apps_profile_wide, profile.name))
}

@Composable
private fun AppList(s: FamilyWardenState, profile: ProfileRef, data: InstalledApps, onSelect: (String?) -> Unit) {
    var query by rememberSaveable(profile.id) { mutableStateOf("") }
    var filter by rememberSaveable(profile.id) { mutableStateOf(AppFilter.All.name) }
    SourceNotes(s, profile, data)
    if (data.sources.none { it.reported }) {
        SectionCard {
            Text(stringResource(R.string.fw_apps_none_title), style = MaterialTheme.typography.titleMedium)
            Text(stringResource(R.string.fw_apps_none_body, profile.name))
        }
        return
    }
    OutlinedTextField(query, { query = it.take(64) }, Modifier.fillMaxWidth(), singleLine = true,
        leadingIcon = { Icon(Icons.Outlined.Search, null) },
        label = { Text(stringResource(R.string.fw_apps_search, profile.name)) })
    @OptIn(ExperimentalLayoutApi::class)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        AppFilter.entries.forEach { f ->
            FilterChip(selected = filter == f.name, onClick = { filter = f.name }, label = { Text(stringResource(f.label)) },
                modifier = Modifier.heightIn(min = 48.dp))
        }
    }
    val words = query.trim().lowercase()
    val ordinary = data.apps.filter { !it.essentialAnywhere() }.filter { a ->
        (words.isEmpty() || a.label.lowercase().contains(words) || (a.label.isEmpty() && a.pkg.contains(words))) && when (AppFilter.valueOf(filter)) {
            AppFilter.All -> true
            AppFilter.Limited -> a.rule is AppRule.Limit
            AppFilter.Blocked -> a.rule == AppRule.Blocked
            AppFilter.Always -> a.rule == AppRule.Always
            AppFilter.New -> a.waiting(data.suspendNew)
        }
    }.sortedWith(compareBy({ it.label.ifEmpty { it.pkg }.lowercase() }, { it.pkg }))
    val duplicates = data.apps.groupBy { it.label.lowercase() }.filter { it.key.isNotEmpty() && it.value.size > 1 }.keys
    val initials = tileInitials(data.apps.map { it.label.ifEmpty { it.pkg } })
    if (ordinary.isEmpty()) InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_no_match))
    else SectionCard { ordinary.forEach { AppRow(s, data, it, it.label.lowercase() in duplicates, initials, onSelect) } }
    val essential = data.apps.filter { it.essentialAnywhere() }
    if (essential.isNotEmpty() && AppFilter.valueOf(filter) == AppFilter.All && words.isEmpty()) SectionCard {
        Overline(stringResource(R.string.fw_apps_essential_group))
        essential.forEach { AppRow(s, data, it, it.label.lowercase() in duplicates, initials, onSelect) }
        Text(stringResource(R.string.fw_apps_essential_note, profile.name), style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
private fun SourceNotes(s: FamilyWardenState, profile: ProfileRef, data: InstalledApps) {
    if (data.sources.isEmpty()) InfoNote(Icons.Outlined.PhonelinkOff, stringResource(R.string.fw_apps_no_phones, profile.name))
    data.sources.forEach { src ->
        when {
            !src.reported -> InfoNote(Icons.Outlined.CloudOff, stringResource(R.string.fw_apps_source_missing, src.name))
            data.serverTime - (src.received ?: 0) > STALE_SECONDS ->
                InfoNote(Icons.Outlined.History, stringResource(R.string.fw_apps_source_old, src.name, appsTime(s, src.received!!)))
            src.partial -> InfoNote(Icons.Outlined.Info, stringResource(R.string.fw_apps_source_partial, src.name, src.sent ?: 0, src.installed ?: 0))
        }
    }
}

@Composable
private fun AppRow(s: FamilyWardenState, data: InstalledApps, app: InstalledApp, duplicate: Boolean,
    initials: Map<String, String>, onSelect: (String?) -> Unit) {
    val name = app.label.ifEmpty { app.pkg }
    Row(Modifier.fillMaxWidth().heightIn(min = 56.dp).clickable(role = Role.Button) { onSelect(app.pkg) }.padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        AppTile(app.pkg, initials.getValue(name), 40.dp)
        Column(Modifier.weight(1f)) {
            Text(name, style = MaterialTheme.typography.titleMedium)
            if (duplicate && app.label.isNotEmpty()) Text(app.pkg, style = MaterialTheme.typography.bodySmall)
            Text(ruleText(app, data.suspendNew) + " · " + whereText(data, app), style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun AppDetail(s: FamilyWardenState, actions: FamilyWardenActions, profile: ProfileRef, data: InstalledApps,
    app: InstalledApp, refresh: () -> Unit, back: () -> Unit) {
    androidx.activity.compose.BackHandler(onBack = back)
    TextButton(onClick = back) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, null); Spacer(Modifier.width(8.dp)); Text(stringResource(R.string.fw_apps_title)) }
    val name = app.label.ifEmpty { app.pkg }
    SectionCard {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            AppTile(app.pkg, tileInitials(listOf(name)).getValue(name), 48.dp)
            Column {
                Text(name, style = MaterialTheme.typography.titleLarge)
                Text(app.pkg, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
    SectionCard {
        Overline(stringResource(R.string.fw_apps_rule_for, profile.name))
        Text(ruleText(app, data.suspendNew), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(when {
            app.essentialAnywhere() -> R.string.fw_apps_rule_essential
            app.rule == AppRule.Blocked -> R.string.fw_apps_rule_blocked_body
            app.rule == AppRule.Always -> R.string.fw_apps_rule_always_body
            app.rule is AppRule.Limit -> R.string.fw_apps_rule_limit_body
            app.waiting(data.suspendNew) -> R.string.fw_apps_rule_waiting_body
            else -> R.string.fw_apps_rule_allowed_body
        }, profile.name), style = MaterialTheme.typography.bodyMedium)
    }
    SectionCard {
        Overline(stringResource(R.string.fw_apps_installed_on_title))
        if (app.on.isEmpty()) Text(stringResource(R.string.fw_apps_not_reported))
        app.on.sorted().forEach { i ->
            val src = data.sources[i]
            Text(src.name, style = MaterialTheme.typography.titleMedium)
            Text(stringResource(R.string.fw_apps_reported_at, appsTime(s, src.received!!)) +
                (if (data.serverTime - src.received > STALE_SECONDS) " · " + stringResource(R.string.fw_apps_may_be_old) else "") +
                (if (i in app.essential) " · " + stringResource(R.string.fw_apps_essential_here) else ""),
                style = MaterialTheme.typography.bodySmall)
        }
    }
    InfoNote(Icons.Outlined.Groups, stringResource(R.string.fw_apps_profile_wide, profile.name))
    AppRuleEditor(s, actions, data, app, refresh)
}

@Composable
private fun ruleText(app: InstalledApp, suspendNew: Boolean): String = when {
    app.essentialAnywhere() -> stringResource(R.string.fw_apps_essential)
    app.rule == AppRule.Blocked -> stringResource(R.string.fw_apps_filter_blocked)
    app.rule == AppRule.Always -> stringResource(R.string.fw_apps_filter_always)
    app.rule is AppRule.Limit -> stringResource(R.string.fw_apps_limit, durationText(app.rule.minutes * 60_000L))
    app.waiting(suspendNew) -> stringResource(R.string.fw_apps_waiting)
    else -> stringResource(R.string.fw_apps_allowed)
}

@Composable
private fun whereText(data: InstalledApps, app: InstalledApp): String = when (app.on.size) {
    0 -> stringResource(R.string.fw_apps_not_reported_short)
    1 -> data.sources[app.on.single()].name
    else -> pluralStringResource(R.plurals.fw_apps_phones, app.on.size, app.on.size)
}

@Composable
private fun appsTime(s: FamilyWardenState, epoch: Long): String = Instant.ofEpochSecond(epoch)
    .atZone(ZoneId.of(s.timeZone ?: ZoneId.systemDefault().id))
    .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT).withLocale(LocalConfiguration.current.locales[0]))
