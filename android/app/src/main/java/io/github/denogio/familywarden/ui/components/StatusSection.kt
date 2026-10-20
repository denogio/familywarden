package io.github.denogio.familywarden.ui.components

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Anchor
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Sailing
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material.icons.outlined.WifiTethering
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import io.github.denogio.familywarden.R
import io.github.denogio.familywarden.ui.Details
import io.github.denogio.familywarden.ui.Pact
import io.github.denogio.familywarden.ui.PactStatus
import io.github.denogio.familywarden.ui.SyncState
import io.github.denogio.familywarden.ui.formatCountdown
import io.github.denogio.familywarden.ui.formatDay
import io.github.denogio.familywarden.ui.formatDuration
import io.github.denogio.familywarden.ui.formatWhen
import io.github.denogio.familywarden.ui.groupFingerprint
import io.github.denogio.familywarden.ui.rememberNow
import io.github.denogio.familywarden.ui.theme.CountdownStyle
import io.github.denogio.familywarden.ui.theme.MastThemeExt

/** The hero card for a paired phone. Always shows icon + words, never colour alone. */
@Composable
fun StatusHero(status: PactStatus, nowMillis: Long, modifier: Modifier = Modifier) {
    when (status) {
        PactStatus.NoCaptain -> CalmHero(
            icon = Icons.Outlined.Anchor,
            title = stringResource(R.string.status_no_captain_title),
            body = stringResource(R.string.status_no_captain_body),
            modifier = modifier,
        )
        PactStatus.FreeSailing -> CalmHero(
            icon = Icons.Outlined.Sailing,
            title = stringResource(R.string.status_free_title),
            body = stringResource(R.string.status_free_body),
            modifier = modifier,
        )
        is PactStatus.Bound -> when {
            status.pact.releaseAtMillis != null ->
                SailingHomeHero(status.pact.releaseAtMillis, status.blocked, nowMillis, modifier)
            status.blocked -> LockedHero(status.nextChangeMillis, nowMillis, modifier)
            else -> AllowedHero(status.nextChangeMillis, nowMillis, modifier)
        }
    }
}

@Composable
private fun CalmHero(icon: ImageVector, title: String, body: String, modifier: Modifier) {
    val colors = MaterialTheme.colorScheme
    HeroCard(
        modifier = modifier,
        color = colors.tertiaryContainer,
        decoration = { Wave(colors.tertiary.copy(alpha = 0.55f)) },
    ) {
        HeroBadge(icon, container = colors.surfaceContainer, content = colors.tertiary)
        HeroTitle(title, color = colors.onSurface)
        HeroBody(body)
    }
}

@Composable
private fun LockedHero(nextChangeMillis: Long?, initialNow: Long, modifier: Modifier) {
    val now by rememberNow(initialNow, periodMillis = 30_000)
    val night = MastThemeExt.colors
    NightCard(modifier) {
        HeroBadge(Icons.Outlined.Lock, container = night.nightAccent.copy(alpha = 0.18f), content = night.nightAccent)
        HeroTitle(stringResource(R.string.status_locked_title), color = night.onNight)
        HeroBody(
            text = if (nextChangeMillis != null) {
                stringResource(R.string.status_locked_until, formatWhen(nextChangeMillis, now))
            } else {
                stringResource(R.string.status_locked_no_end)
            },
            color = night.onNightMuted,
        )
        if (nextChangeMillis != null && nextChangeMillis > now) {
            HeroFooter(
                icon = Icons.Outlined.Schedule,
                text = stringResource(R.string.status_opens_in, formatDuration(nextChangeMillis - now)),
                textColor = night.onNight,
                iconColor = night.onNight,
                dividerColor = night.nightDivider,
            )
        }
    }
}

@Composable
private fun AllowedHero(nextChangeMillis: Long?, initialNow: Long, modifier: Modifier) {
    val now by rememberNow(initialNow, periodMillis = 30_000)
    val colors = MaterialTheme.colorScheme
    HeroCard(modifier = modifier) {
        HeroBadge(Icons.Outlined.WifiTethering, container = colors.tertiaryContainer, content = colors.tertiary)
        HeroTitle(stringResource(R.string.status_allowed_title), color = colors.onSurface)
        HeroBody(stringResource(R.string.status_allowed_body))
        if (nextChangeMillis != null && nextChangeMillis > now) {
            HeroFooter(
                icon = Icons.Outlined.Lock,
                text = stringResource(
                    R.string.status_locks_at,
                    formatWhen(nextChangeMillis, now),
                    formatDuration(nextChangeMillis - now),
                ),
                textColor = colors.onSurface,
                iconColor = colors.secondary,
                dividerColor = colors.outlineVariant,
            )
        }
    }
}

@Composable
private fun SailingHomeHero(releaseAtMillis: Long, blocked: Boolean, initialNow: Long, modifier: Modifier) {
    val now by rememberNow(initialNow, periodMillis = 1_000)
    val night = MastThemeExt.colors
    val left = (releaseAtMillis - now).coerceAtLeast(0)
    val spokenLeft = stringResource(R.string.status_countdown_description, formatDuration(left))
    NightCard(modifier) {
        HeroBadge(Icons.Outlined.Sailing, container = night.nightAccent.copy(alpha = 0.18f), content = night.nightAccent)
        Overline(stringResource(R.string.status_sailing_home_overline), color = night.nightAccent)
        HeroTitle(
            stringResource(R.string.status_sailing_home_title, formatWhen(releaseAtMillis, now)),
            color = night.onNight,
        )
        HeroBody(
            stringResource(if (blocked) R.string.status_sailing_home_locked else R.string.status_sailing_home_allowed),
            color = night.onNightMuted,
        )
        Spacer(Modifier.height(18.dp))
        HorizontalDivider(color = night.nightDivider)
        Spacer(Modifier.height(14.dp))
        // One semantic node with minute precision; the ticking seconds are visual only.
        Row(
            verticalAlignment = Alignment.Bottom,
            modifier = Modifier.clearAndSetSemantics { contentDescription = spokenLeft },
        ) {
            Text(formatCountdown(left), style = CountdownStyle, color = night.onNight)
            Spacer(Modifier.width(10.dp))
            Text(
                stringResource(R.string.status_countdown_left),
                style = MaterialTheme.typography.bodySmall,
                color = night.onNightMuted,
                modifier = Modifier.padding(bottom = 7.dp),
            )
        }
    }
}

/** "The pact" card under the hero while bound. */
@Composable
fun PactCard(pact: Pact, captain: String?, modifier: Modifier = Modifier) {
    SectionCard(modifier) {
        Overline(stringResource(R.string.pact_overline))
        Text(
            text = if (pact.untilMillis == null) {
                stringResource(R.string.pact_ongoing)
            } else {
                stringResource(R.string.pact_until, formatDay(pact.untilMillis))
            },
            style = MaterialTheme.typography.titleMedium,
        )
        val body = when {
            captain != null && pact.rulesSummary != null ->
                stringResource(R.string.pact_chosen_by_with_rules, captain, pact.rulesSummary)
            captain != null -> stringResource(R.string.pact_chosen_by, captain)
            pact.rulesSummary != null -> stringResource(R.string.pact_rules, pact.rulesSummary)
            else -> null
        }
        if (body != null) {
            Spacer(Modifier.height(6.dp))
            Text(body, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (pact.releaseAtMillis != null) {
            Spacer(Modifier.height(6.dp))
            Text(
                stringResource(R.string.pact_release_requested),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Shown above the hero when the last sync failed. Unreachable is calm; bad signature is not. */
@Composable
fun SyncBanner(sync: SyncState, details: Details, nowMillis: Long, modifier: Modifier = Modifier) {
    when (sync) {
        SyncState.Ok -> Unit
        SyncState.Unreachable -> Banner(
            icon = Icons.Outlined.CloudOff,
            title = stringResource(R.string.sync_unreachable_title),
            body = details.lastSyncMillis?.let {
                stringResource(R.string.sync_unreachable_body, formatWhen(it, nowMillis))
            } ?: stringResource(R.string.sync_unreachable_body_never),
            modifier = modifier,
        )
        SyncState.BadSignature -> Banner(
            icon = Icons.Outlined.WarningAmber,
            title = stringResource(R.string.sync_bad_signature_title),
            body = stringResource(
                R.string.sync_bad_signature_body,
                details.fingerprint?.let(::groupFingerprint) ?: "",
            ),
            extra = stringResource(R.string.sync_bad_signature_advice),
            warning = true,
            modifier = modifier,
        )
        is SyncState.Failed -> Banner(
            icon = Icons.Outlined.Info,
            title = stringResource(R.string.sync_failed_title),
            body = stringResource(R.string.sync_failed_body, sync.message),
            modifier = modifier,
        )
    }
}
