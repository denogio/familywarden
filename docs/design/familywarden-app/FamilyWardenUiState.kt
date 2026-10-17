package io.github.denogio.familywarden.ui.model

import androidx.compose.runtime.Immutable

/*
 * UI state contract for FamilyWarden. Pure data plus a few pure derivation functions.
 * No Android, no Compose runtime beyond @Immutable. The composables only render this and
 * call FamilyWardenActions.
 *
 * Every axis is independent. The UI never infers one axis from another; where a
 * combination matters (headline, navigation, Ulysses visibility) the rule is a pure
 * function in this file so it can be unit tested.
 *
 * All times are epoch milliseconds.
 */

@Immutable
data class AppUiState(
    val nowMillis: Long,
    val setup: SetupProgress,
    val capabilities: Capabilities = Capabilities(),
    val ulyssesFeature: UlyssesFeature = UlyssesFeature.Off,
    val policy: PolicyState = PolicyState.NotReceived,
    val dnsApplication: DnsApplication = DnsApplication.NotApplied,
    val resolver: ResolverHealth = ResolverHealth.Untested,
    val routerSync: RouterSync = RouterSync.Never,
    val ulysses: UlyssesState = UlyssesState(),
    val removal: RemovalState = RemovalState.Idle,
    val device: DeviceInfo = DeviceInfo(),
    val dialog: AppDialog? = null,
)

// ---------------------------------------------------------------- Setup

@Immutable
sealed interface SetupProgress {
    data class NotDeviceOwner(val readiness: OwnerReadiness) : SetupProgress
    data object NotPaired : SetupProgress
    data class ConfirmKey(val host: String, val fingerprint: String) : SetupProgress
    /** Step 3: each part is its own axis in AppUiState (policy, dnsApplication, resolver). */
    data object ApplyingDns : SetupProgress
    data object Complete : SetupProgress
}

@Immutable
data class OwnerReadiness(
    val accountCount: Int,
    val usbDebugging: Boolean?,   // null = can't tell from the app
    val isDeviceOwner: Boolean,
)

// ---------------------------------------------------------------- Roles

@Immutable
data class Capabilities(
    /** Managed device: true once paired. */
    val managedDevice: Boolean = false,
    val association: Association = Association.Unknown,
    /** Router-confirmed captain. Only meaningful when association is Confirmed. */
    val captain: Boolean = false,
    /** FUTURE. Always false in the initial scope. Never implies captain, and the reverse. */
    val administrator: Boolean = false,
)

/** NEEDS ROUTER SUPPORT: device/profile association is a planned backend extension. */
@Immutable
sealed interface Association {
    data object Unknown : Association
    data object RouterUnsupported : Association
    data class Pending(val askedAt: Long) : Association
    data class Confirmed(val profileName: String, val confirmedAt: Long) : Association
    data class Mismatch(val routerProfileName: String?) : Association
}

// ---------------------------------------------------------------- Ulysses feature (local opt-in)

@Immutable
sealed interface UlyssesFeature {
    data object Off : UlyssesFeature
    /** Opt-in started; asking the router. */
    data object Checking : UlyssesFeature
    data class Ineligible(val reason: IneligibleReason) : UlyssesFeature
    /** Router confirmed captain; waiting for the user's "Turn on". */
    data object Eligible : UlyssesFeature
    data object On : UlyssesFeature
    /** On, and cannot be turned off because a pact is active. */
    data object LockedOn : UlyssesFeature
}

enum class IneligibleReason { RouterUnreachable, RouterUnsupported, AssociationUnknown, AssociationMismatch, NotCaptain }

// ---------------------------------------------------------------- DNS: four separate facts

@Immutable
sealed interface PolicyState {
    data object NotReceived : PolicyState
    data class Verified(
        val receivedAt: Long,
        val resolverHostname: String,     // e.g. "phone-id.dns.example.net" (example)
        val policyName: String?,          // EXAMPLE DATA until the payload is decided
        val categories: List<String>,     // EXAMPLE DATA
    ) : PolicyState
    /** A new policy arrived but failed verification. The previous Verified one stays applied. */
    data class Rejected(val reason: String, val lastVerified: Verified?) : PolicyState
}

@Immutable
sealed interface DnsApplication {
    data object NotApplied : DnsApplication
    data object Applying : DnsApplication
    /** Strict Private DNS (DoT) set via device owner. locked = user restriction in place. */
    data class Applied(val hostname: String, val locked: Boolean) : DnsApplication
    data class Failed(val reason: String) : DnsApplication
}

@Immutable
sealed interface ResolverHealth {
    data object Untested : ResolverHealth
    data object Testing : ResolverHealth
    data class Reachable(val testedAt: Long) : ResolverHealth
    /** Possible causes are shown as possibilities, not diagnoses. */
    data class Unreachable(val testedAt: Long, val lastOkAt: Long?) : ResolverHealth
    data class CertificateInvalid(val checkedAt: Long, val presentedName: String?) : ResolverHealth
}

@Immutable
sealed interface RouterSync {
    data object Never : RouterSync
    data class Syncing(val lastVerifiedAt: Long?) : RouterSync
    data class Reachable(val lastSyncAt: Long) : RouterSync
    /** Normal away from home. Neutral. The cached policy stays in force; no time-based escalation. */
    data class Unreachable(val lastVerifiedAt: Long?) : RouterSync
    /** Not normal. Problem tone. Reply ignored. */
    data class BadSignature(val lastVerifiedAt: Long?) : RouterSync
    data class OtherError(val message: String, val lastVerifiedAt: Long?) : RouterSync
}

// ---------------------------------------------------------------- Ulysses (pact facts, cached)

@Immutable
data class UlyssesState(
    val pact: Pact? = null,
    val hotspot: HotspotState = HotspotState.NotRestricted,
    val schedule: Schedule? = null,
    val captainName: String? = null,
    val sailPage: SailPage = SailPage.Unavailable,
)

@Immutable
data class Pact(
    val kind: PactKind,
    /** Sailing home: released at this time, locally, even without a sync. */
    val releaseAt: Long?,
)

@Immutable
sealed interface PactKind {
    data object Ongoing : PactKind
    data class Until(val endsAt: Long) : PactKind
}

@Immutable
sealed interface HotspotState {
    /** Ulysses off or no pact. The UI says nothing about hotspot. */
    data object NotRestricted : HotspotState
    data class Allowed(val nextLockAt: Long?) : HotspotState
    data class Blocked(val nextOpenAt: Long?) : HotspotState
}

/** Allowed windows in minutes from local midnight, e.g. 360..1290 for 06:00 to 21:30. */
@Immutable
data class Schedule(val windows: List<IntRange>, val summary: String)

@Immutable
sealed interface SailPage {
    data object Unavailable : SailPage
    data class Reachable(val url: String) : SailPage
}

// ---------------------------------------------------------------- Removal

@Immutable
sealed interface RemovalState {
    data object Idle : RemovalState
    /** Asking the router now. A cached "no pact" is never enough. */
    data object Checking : RemovalState
    data class Allowed(val confirmedAt: Long) : RemovalState
    data object RefusedPact : RemovalState
    data class RefusedUnreachable(val triedAt: Long) : RemovalState
    /** NEEDS ROUTER SUPPORT if the endpoint is missing. */
    data object RefusedUnsupported : RemovalState
}

// ---------------------------------------------------------------- Misc

@Immutable
data class DeviceInfo(
    val routerHost: String? = null,
    val fingerprint: String? = null,   // 16 hex chars, ungrouped
    val appVersion: String = "",
    val isDeviceOwner: Boolean = false,
)

@Immutable
sealed interface AppDialog {
    data class ManualAddress(val initial: String = "", val error: String? = null) : AppDialog
    data class ConfirmPairing(val host: String, val fingerprint: String) : AppDialog
    data object UlyssesOptIn : AppDialog          // sheet; content follows ulyssesFeature
    data object ConfirmRemoval : AppDialog
    data object RemovalRefusedPact : AppDialog
}

// ---------------------------------------------------------------- Derived rules (pure)

enum class Destination { Overview, Dns, Ulysses, Family, Settings }

/** True when any cached pact is in force at [now], including finite pacts not yet expired. */
fun UlyssesState.pactActive(now: Long): Boolean {
    val p = pact ?: return false
    if (p.releaseAt != null && now >= p.releaseAt) return false
    return when (val k = p.kind) {
        PactKind.Ongoing -> true
        is PactKind.Until -> now < k.endsAt
    }
}

/** Ulysses is visible when switched on, or whenever a pact is active: a pact always wins. */
fun AppUiState.ulyssesVisible(): Boolean =
    ulyssesFeature == UlyssesFeature.On ||
        ulyssesFeature == UlyssesFeature.LockedOn ||
        ulysses.pactActive(nowMillis)

/** Bottom bar. Empty before setup is complete (onboarding owns the screen). Max 5. */
fun AppUiState.destinations(): List<Destination> {
    if (setup != SetupProgress.Complete) return emptyList()
    return buildList {
        add(Destination.Overview)
        add(Destination.Dns)
        if (ulyssesVisible()) add(Destination.Ulysses)
        if (capabilities.administrator) add(Destination.Family) // FUTURE
        add(Destination.Settings)
    }
}

enum class Tone { Ok, Neutral, Pending, Attention, Problem, Night }

enum class OverviewHeadline(val tone: Tone) {
    ProtectionOn(Tone.Ok),
    NetworkBlocksResolver(Tone.Problem),
    CertificateInvalid(Tone.Problem),
    Applying(Tone.Pending),
    SetupIncomplete(Tone.Attention),
    ApplyFailed(Tone.Problem),
}

/** Router sync deliberately does not take part: it is its own fact row. */
fun AppUiState.overviewHeadline(): OverviewHeadline = when (dnsApplication) {
    is DnsApplication.Applied -> when (resolver) {
        is ResolverHealth.Unreachable -> OverviewHeadline.NetworkBlocksResolver
        is ResolverHealth.CertificateInvalid -> OverviewHeadline.CertificateInvalid
        else -> OverviewHeadline.ProtectionOn
    }
    DnsApplication.Applying -> OverviewHeadline.Applying
    DnsApplication.NotApplied -> OverviewHeadline.SetupIncomplete
    is DnsApplication.Failed -> OverviewHeadline.ApplyFailed
}

/** Pairing and removal are refused during a pact by logic; the UI only reflects it. */
fun AppUiState.pairingAllowed(): Boolean = !ulysses.pactActive(nowMillis)

// ---------------------------------------------------------------- Actions

@Immutable
class FamilyWardenActions(
    val onNavigate: (Destination) -> Unit = {},
    val onSync: () -> Unit = {},
    val onTestResolver: () -> Unit = {},
    val onOpenSailPage: () -> Unit = {},
    val onStartUlyssesOptIn: () -> Unit = {},
    val onConfirmUlyssesOptIn: () -> Unit = {},
    val onTurnOffUlysses: () -> Unit = {},      // ignored by logic while a pact is active
    val onCheckDeviceOwner: () -> Unit = {},
    val onCopyAdbCommand: () -> Unit = {},
    val onScanPairingCode: () -> Unit = {},
    val onEnterAddress: () -> Unit = {},
    val onSubmitAddress: (String) -> Unit = {},
    val onConfirmPairing: () -> Unit = {},
    val onCopyDiagnostics: () -> Unit = {},
    val onRequestRemoval: () -> Unit = {},      // logic asks the router, then sets removal state
    val onConfirmRemoval: () -> Unit = {},
    val onDismissDialog: () -> Unit = {},
)
