package io.github.denogio.familywarden.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.unit.dp
import java.time.Instant
import java.time.ZoneId
import io.github.denogio.familywarden.R

/** The single sync status on the overview, shown as a chip in the top bar. */
sealed interface SyncChip {
    data object Syncing : SyncChip
    data class Updated(val atMillis: Long) : SyncChip
    /** Router out of reach; the cached policy keeps applying. */
    data class Offline(val atMillis: Long?) : SyncChip
    data object Failed : SyncChip
    data object Never : SyncChip
}

fun syncChip(s: FamilyWardenState): SyncChip = when {
    s.syncing -> SyncChip.Syncing
    s.syncError == "router unreachable" -> SyncChip.Offline(s.lastSync.takeIf { it > 0 }?.times(1000))
    s.syncError != null -> SyncChip.Failed
    s.lastSync > 0 -> SyncChip.Updated(s.lastSync * 1000)
    else -> SyncChip.Never
}

@Composable
internal fun SyncStatusChip(s: FamilyWardenState, zone: ZoneId) {
    val chip = syncChip(s)
    val now = Instant.ofEpochMilli(s.nowMillis).atZone(zone)
    fun at(millis: Long) = Instant.ofEpochMilli(millis).atZone(zone)
    val text = when (chip) {
        SyncChip.Syncing -> stringResource(R.string.action_syncing)
        is SyncChip.Updated -> stringResource(R.string.fw_status_updated, whenText(at(chip.atMillis), now))
        is SyncChip.Offline -> chip.atMillis?.let { stringResource(R.string.fw_status_offline, whenText(at(it), now)) }
            ?: stringResource(R.string.fw_status_offline_never)
        SyncChip.Failed -> stringResource(R.string.fw_status_failed)
        SyncChip.Never -> stringResource(R.string.fw_status_never)
    }
    val colors = MaterialTheme.colorScheme
    val (container, dot) = when (chip) {
        is SyncChip.Updated, SyncChip.Syncing -> colors.tertiaryContainer to colors.tertiary
        is SyncChip.Offline, SyncChip.Never -> colors.surfaceContainerHigh to colors.onSurfaceVariant
        SyncChip.Failed -> colors.errorContainer to colors.error
    }
    Surface(shape = CircleShape, color = container) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(6.dp).background(dot, CircleShape).clearAndSetSemantics { })
            Spacer(Modifier.width(5.dp))
            Text(text, style = MaterialTheme.typography.labelSmall, color = colors.onSurface)
        }
    }
}
