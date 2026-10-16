package me.hojgaard.ulysses.mast.ui

import androidx.compose.runtime.Immutable

/**
 * Everything the screen renders. Built by [mapMastUiState] from the existing
 * MastState / Decision / prefs. All times are epoch milliseconds.
 */
@Immutable
data class MastUiState(
    /** Snapshot of "now" when the state was built. Countdowns tick from here. */
    val nowMillis: Long,
    val setup: SetupState,
    val sync: SyncState = SyncState.Ok,
    val isSyncing: Boolean = false,
    val details: Details = Details(),
    val dialog: MastDialog? = null,
)

@Immutable
sealed interface SetupState {
    data object NotDeviceOwner : SetupState
    data object NotPaired : SetupState
    data class Paired(val status: PactStatus) : SetupState
}

@Immutable
sealed interface PactStatus {
    /** Paired, but the router has no captain set. Nothing is enforced. */
    data object NoCaptain : PactStatus

    /** No pact. Hotspot allowed. Pairing and removal available. */
    data object FreeSailing : PactStatus

    /** A pact is active. [blocked] is the current Decision.block. */
    data class Bound(
        val blocked: Boolean,
        /** Policy.nextChange: when blocked/allowed next flips. Null when unknown. */
        val nextChangeMillis: Long?,
        val pact: Pact,
    ) : PactStatus
}

@Immutable
data class Pact(
    /** Null for an ongoing pact. */
    val untilMillis: Long?,
    /** "Sailing home": free at this time. Null when no release is requested. */
    val releaseAtMillis: Long?,
    /** Human summary of the allow windows, e.g. "06:00–21:30, every day". Optional. */
    val rulesSummary: String?,
)

@Immutable
sealed interface SyncState {
    data object Ok : SyncState

    /** Normal away from home. Neutral presentation. */
    data object Unreachable : SyncState

    /** Not normal. Clear warning. */
    data object BadSignature : SyncState

    /** Any other lastError, shown neutrally with its message. */
    data class Failed(val message: String) : SyncState
}

@Immutable
data class Details(
    val captain: String? = null,
    val routerHost: String? = null,
    /** 16 hex chars, ungrouped. The UI groups it in fours. */
    val fingerprint: String? = null,
    val lastSyncMillis: Long? = null,
    val isDeviceOwner: Boolean = false,
)

@Immutable
sealed interface MastDialog {
    data class ManualAddress(val initial: String = "", val error: String? = null) : MastDialog
    data class ConfirmPairing(val host: String, val fingerprint: String) : MastDialog
    data object ConfirmRemove : MastDialog
    data object RemoveRefused : MastDialog
}

/**
 * Callbacks into MainActivity. The UI never decides anything itself: it only reports
 * what the user tapped.
 */
@Immutable
class MastActions(
    val onSync: () -> Unit = {},
    val onCheckDeviceOwner: () -> Unit = {},
    val onScanPairingCode: () -> Unit = {},
    /** Open the manual address dialog (Activity sets dialog = ManualAddress). */
    val onPairManually: () -> Unit = {},
    /** Address entered; Activity fetches the key and sets dialog = ConfirmPairing. */
    val onSubmitAddress: (String) -> Unit = {},
    val onConfirmPairing: () -> Unit = {},
    /** Activity syncs, then sets dialog = ConfirmRemove or RemoveRefused. */
    val onRemove: () -> Unit = {},
    val onConfirmRemove: () -> Unit = {},
    val onDismissDialog: () -> Unit = {},
)
