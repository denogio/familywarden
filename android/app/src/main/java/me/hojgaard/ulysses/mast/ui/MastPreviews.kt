package me.hojgaard.ulysses.mast.ui

import android.content.res.Configuration
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.tooling.preview.Preview
import me.hojgaard.ulysses.mast.ui.components.PairConfirmContent
import me.hojgaard.ulysses.mast.ui.theme.MastTheme

// Every state in light and dark. Times are relative to a fixed NOW so previews are stable.

@Preview(name = "Light", showBackground = true, widthDp = 390, heightDp = 844)
@Preview(
    name = "Dark",
    showBackground = true,
    widthDp = 390,
    heightDp = 844,
    uiMode = Configuration.UI_MODE_NIGHT_YES or Configuration.UI_MODE_TYPE_NORMAL,
)
annotation class MastPreviews

private const val MIN = 60_000L
private const val HOUR = 60 * MIN
private const val DAY = 24 * HOUR

/** 2026-10-16 18:18 UTC. */
private const val NOW = 1_792_174_680_000L

private val pairedDetails = Details(
    captain = "dennis",
    routerHost = "192.168.32.2:7700",
    fingerprint = "e7e33ab8f182801f",
    lastSyncMillis = NOW - 2 * MIN,
    isDeviceOwner = true,
)

private val ongoingPact = Pact(untilMillis = null, releaseAtMillis = null, rulesSummary = "06:00–21:30, every day")
private val datedPact = ongoingPact.copy(untilMillis = NOW + 18 * DAY)

private fun paired(status: PactStatus, sync: SyncState = SyncState.Ok, dialog: MastDialog? = null) = MastUiState(
    nowMillis = NOW,
    setup = SetupState.Paired(status),
    sync = sync,
    details = pairedDetails,
    dialog = dialog,
)

private val blocked = PactStatus.Bound(blocked = true, nextChangeMillis = NOW + 11 * HOUR + 42 * MIN, pact = ongoingPact)

internal object PreviewStates {
    val notDeviceOwner = MastUiState(NOW, SetupState.NotDeviceOwner, details = Details(isDeviceOwner = false))
    val notPaired = MastUiState(NOW, SetupState.NotPaired, details = Details(isDeviceOwner = true))
    val freeSailing = paired(PactStatus.FreeSailing)
    val noCaptain = paired(PactStatus.NoCaptain).copy(details = pairedDetails.copy(captain = null))
    val boundAllowed = paired(PactStatus.Bound(blocked = false, nextChangeMillis = NOW + 3 * HOUR + 12 * MIN, pact = datedPact))
    val boundBlocked = paired(blocked)
    val boundBlockedDated = paired(blocked.copy(pact = datedPact))
    val sailingHome = paired(blocked.copy(pact = ongoingPact.copy(releaseAtMillis = NOW + 4 * HOUR + 12 * MIN + 8_000)))
    val unreachable = paired(blocked, sync = SyncState.Unreachable).copy(details = pairedDetails.copy(lastSyncMillis = NOW - 12 * HOUR))
    val badSignature = paired(blocked, sync = SyncState.BadSignature)
    val staleReply = paired(PactStatus.FreeSailing, sync = SyncState.Failed("stale reply"))
    val removeRefused = paired(PactStatus.FreeSailing, dialog = MastDialog.RemoveRefused)
    val removeConfirm = paired(PactStatus.FreeSailing, dialog = MastDialog.ConfirmRemove)
    val manualAddress = MastUiState(NOW, SetupState.NotPaired, details = Details(isDeviceOwner = true), dialog = MastDialog.ManualAddress())
}

@Composable
private fun Screen(state: MastUiState) {
    MastTheme { MastScreen(state, MastActions()) }
}

@MastPreviews @Composable private fun P01NotDeviceOwner() = Screen(PreviewStates.notDeviceOwner)
@MastPreviews @Composable private fun P02NotPaired() = Screen(PreviewStates.notPaired)
@MastPreviews @Composable private fun P02bManualAddress() = Screen(PreviewStates.manualAddress)

@MastPreviews
@Composable
private fun P03PairConfirm() {
    MastTheme {
        Surface(color = MaterialTheme.colorScheme.surfaceContainer) {
            PairConfirmContent(host = "192.168.32.2:7700", fingerprint = "e7e33ab8f182801f", onConfirm = {}, onDismiss = {})
        }
    }
}

@MastPreviews @Composable private fun P04FreeSailing() = Screen(PreviewStates.freeSailing)
@MastPreviews @Composable private fun P04bNoCaptain() = Screen(PreviewStates.noCaptain)
@MastPreviews @Composable private fun P05BoundAllowed() = Screen(PreviewStates.boundAllowed)
@MastPreviews @Composable private fun P06BoundBlocked() = Screen(PreviewStates.boundBlocked)
@MastPreviews @Composable private fun P06bBoundBlockedDated() = Screen(PreviewStates.boundBlockedDated)

@Preview(name = "Blocked, 200 % font", showBackground = true, widthDp = 390, heightDp = 1600, fontScale = 2f)
@Composable
private fun P06cBoundBlockedLargeFont() = Screen(PreviewStates.boundBlockedDated)

@Preview(
    name = "Blocked, 200 % font, dark",
    showBackground = true,
    widthDp = 390,
    heightDp = 1600,
    fontScale = 2f,
    uiMode = Configuration.UI_MODE_NIGHT_YES or Configuration.UI_MODE_TYPE_NORMAL,
)
@Composable
private fun P06dBoundBlockedLargeFontDark() = Screen(PreviewStates.boundBlockedDated)

@MastPreviews @Composable private fun P07SailingHome() = Screen(PreviewStates.sailingHome)
@MastPreviews @Composable private fun P08aUnreachable() = Screen(PreviewStates.unreachable)
@MastPreviews @Composable private fun P08bBadSignature() = Screen(PreviewStates.badSignature)
@MastPreviews @Composable private fun P08cOtherError() = Screen(PreviewStates.staleReply)
@MastPreviews @Composable private fun P09aRemoveRefused() = Screen(PreviewStates.removeRefused)
@MastPreviews @Composable private fun P09bRemoveConfirm() = Screen(PreviewStates.removeConfirm)
