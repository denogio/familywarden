package io.github.denogio.familywarden.ui

import io.github.denogio.familywarden.Decision
import io.github.denogio.familywarden.MastState
import io.github.denogio.familywarden.Reason
import io.github.denogio.familywarden.Policy

/**
 * Pure mapping from the existing data to [MastUiState]. No Android, no Compose, easy to
 * unit test. Keeps all "what does this mean" decisions out of the composables.
 *
 * Epoch values from the router (pactUntil, releaseAt, serverTime), lastSync and
 * Policy.nextChange are assumed to be in SECONDS. If any of them is already in
 * milliseconds, drop the conversion for that field.
 */
fun mapMastUiState(
    nowMillis: Long,
    isDeviceOwner: Boolean,
    routerHost: String?,
    fingerprint: String?,
    mastState: MastState?,
    decision: Decision,
    nextChangeEpoch: Long?,
    lastSyncEpoch: Long?,
    lastError: String?,
    rulesSummary: String? = null,
    isSyncing: Boolean = false,
    dialog: MastDialog? = null,
    /** Display only: the hotspot flip after [nextChangeEpoch]. */
    followingChangeEpoch: Long? = null,
): MastUiState {
    val details = Details(
        captain = mastState?.captain?.takeIf { it.isNotBlank() },
        routerHost = routerHost,
        fingerprint = fingerprint,
        lastSyncMillis = lastSyncEpoch.toMillisOrNull(),
        isDeviceOwner = isDeviceOwner,
    )

    val setup = when {
        !isDeviceOwner -> SetupState.NotDeviceOwner
        routerHost == null || decision.reason == Reason.NOT_PAIRED -> SetupState.NotPaired
        else -> SetupState.Paired(pactStatus(mastState, decision, nextChangeEpoch, followingChangeEpoch, rulesSummary, nowMillis / 1000))
    }

    val sync = when (lastError?.trim()?.lowercase()) {
        null, "" -> SyncState.Ok
        "router unreachable" -> SyncState.Unreachable
        "bad signature" -> SyncState.BadSignature
        else -> SyncState.Failed(lastError.orEmpty())
    }

    return MastUiState(
        nowMillis = nowMillis,
        setup = setup,
        sync = if (setup is SetupState.Paired) sync else SyncState.Ok,
        isSyncing = isSyncing,
        details = details,
        dialog = dialog,
    )
}

private fun pactStatus(
    s: MastState?,
    decision: Decision,
    nextChangeEpoch: Long?,
    followingChangeEpoch: Long?,
    rulesSummary: String?,
    nowEpoch: Long,
): PactStatus = when {
    s != null && Policy.pactActive(s, nowEpoch) -> PactStatus.Bound(
        blocked = decision.block,
        // Allowed -> pact ends is not a transition to "locks".
        nextChangeMillis = nextChangeEpoch.toMillisOrNull(),
        pact = Pact(
            untilMillis = if (s.pactType == "until") s.pactUntil.toMillisOrNull() else null,
            releaseAtMillis = s.releaseAt.toMillisOrNull()?.takeIf {
                s.pactType != "until" || s.pactUntil <= 0 || s.releaseAt <= s.pactUntil
            },
            rulesSummary = rulesSummary,
        ),
        followingChangeMillis = followingChangeEpoch.toMillisOrNull()?.takeIf { nextChangeEpoch != null },
    )
    decision.reason == Reason.NO_CAPTAIN -> PactStatus.NoCaptain
    else -> PactStatus.FreeSailing
}

private fun Long?.toMillisOrNull(): Long? = this?.takeIf { it > 0 }?.times(1000)
