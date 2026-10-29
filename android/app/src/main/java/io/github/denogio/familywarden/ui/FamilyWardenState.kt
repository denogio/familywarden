package io.github.denogio.familywarden.ui

import io.github.denogio.familywarden.*

/** Independent facts. A router outage never changes the applied-DNS fact. */
data class FamilyWardenState(
    val nowMillis: Long,
    val owner: Boolean = false,
    val parentMode: Boolean = false,
    val paired: Boolean = false,
    val provisioned: Boolean = false,
    val binding: DeviceBinding? = null,
    val policyUnsupported: Boolean = false,
    /** Signed reply: the router has not approved this phone (yet). */
    val awaitingApproval: Boolean = false,
    /** This phone's key fingerprint, to compare in LuCI when approving. */
    val phoneFingerprint: String? = null,
    val host: String? = null,
    val fingerprint: String? = null,
    val lastSync: Long = 0,
    val syncError: String? = null,
    val syncing: Boolean = false,
    val ulyssesEnabled: Boolean = false,
    val pact: PactStatus? = null,
    val rules: Rules? = null,
    val timeZone: String? = null,
    val desiredDns: DnsPolicy? = null,
    val appliedDnsHost: String? = null,
    val dnsLocked: Boolean = false,
    val dnsError: String? = null,
    val networkId: Long? = null,
    val resolverCheck: ResolverCheck? = null,
    val probeGeneration: Long = 0,
    val canTestResolver: Boolean = false,
    val dialog: MastDialog? = null,
    /** A trusted v3 policy with app rules is being applied on this phone. */
    val appsActive: Boolean = false,
    /** Last app-control state ("ok", "usage_access_missing", "suspend_failed", ...). */
    val appsStatus: String? = null,
    val screenTime: ScreenTimeState? = null,
    val childRequest: ChildRequest? = null,
    val parentRequests: List<ParentRequest> = emptyList(),
    val parentNewApps: List<NewAppRequest> = emptyList(),
    /** This child phone's apps paused until a parent allows them (local labels). */
    val waitingApps: List<String> = emptyList(),
    val familyUsage: List<FamilyUsage> = emptyList(),
    val requestTargets: List<RequestTarget> = emptyList(),
    val uploadPending: Boolean = false,
    val uploadReply: UploadReply? = null,
    val requestEpoch: Long? = null,
    val canRequestTotal: Boolean = true,
    val voyageDetails: VoyageDetails? = null,
) {
    /** Presentation only; the core still authorizes every family action. */
    val canManageFamily: Boolean get() = parentMode || binding?.approves?.isNotEmpty() == true
    /** Resolve saved presentation state against the current signed scope on every render. */
    fun familyProfile(preferred: String?): ProfileRef? = binding?.approves?.let { profiles ->
        profiles.firstOrNull { it.id == preferred } ?: profiles.firstOrNull()
    }
    /** Presentation filters only; every action still needs a fresh, verified router check. */
    val familyRequests: List<ParentRequest> get() {
        val scope = binding?.approves.orEmpty().map { it.id }.toSet()
        return parentRequests.filter { it.profile.id in scope && (requestEpoch == null || requestEpoch < it.expires) }
    }
    val familyNewApps: List<NewAppRequest> get() {
        val scope = binding?.approves.orEmpty().map { it.id }.toSet()
        return parentNewApps.filter { it.profile.id in scope }
    }
    val familyPendingCount: Int get() = familyRequests.size + familyNewApps.size
    val bound: Boolean get() = pact is PactStatus.Bound
    val showUlysses: Boolean get() = bound || (binding?.captain == true && ulyssesEnabled)
    val canRePair: Boolean get() = canRequestRemoval && (!provisioned || binding?.administrator == true)
    /** Scanning the router's code again only moves to a new address when the key matches. */
    val canScan: Boolean get() = canRePair || ((owner || parentMode) && provisioned)
    // This exposes an action only. Final removal ALWAYS needs a fresh request-bound response.
    val canRequestRemoval: Boolean get() = !bound && (!provisioned || binding?.selfRemoval == true)
    val currentCheck: ResolverCheck? get() = resolverCheck?.takeIf {
        it.policy == desiredDns && it.networkId == networkId && it.generation == probeGeneration
    }
    val headline: DnsHeadline get() = when {
        paired && binding == null && awaitingApproval -> DnsHeadline.AwaitingApproval
        paired && binding == null -> if (policyUnsupported) DnsHeadline.RouterUnsupported else DnsHeadline.PolicyMissing
        desiredDns == null -> DnsHeadline.NotConfigured
        !owner -> DnsHeadline.NotApplied
        dnsError != null -> DnsHeadline.ApplyFailed
        appliedDnsHost != desiredDns.hostname -> DnsHeadline.NotApplied
        !dnsLocked -> DnsHeadline.Unlocked
        currentCheck?.result == ProbeResult.TESTING -> DnsHeadline.Testing
        currentCheck?.result == ProbeResult.CERTIFICATE -> DnsHeadline.CertificateError
        currentCheck?.result == ProbeResult.UNAVAILABLE -> DnsHeadline.Unavailable
        currentCheck?.result == ProbeResult.DNS_ERROR -> DnsHeadline.DnsError
        currentCheck?.result == ProbeResult.ANSWERED -> DnsHeadline.AppliedAndTested
        else -> DnsHeadline.AppliedUntested
    }
}

enum class DnsHeadline { AwaitingApproval, RouterUnsupported, PolicyMissing, NotConfigured, NotApplied, Unlocked, ApplyFailed,
    Testing, CertificateError, Unavailable, DnsError, AppliedAndTested, AppliedUntested }
enum class ProbeResult { TESTING, ANSWERED, UNAVAILABLE, CERTIFICATE, DNS_ERROR }
enum class RemovalRefusal { ActivePact, MissingAuthority, Unreachable, Unsupported, Unverified, NotCompleted }

/** Diagnostic copy after a failed check, never an authorization decision. */
fun removalRefusal(state: MastState?, syncError: String?): RemovalRefusal = when {
    syncError == "router unreachable" -> RemovalRefusal.Unreachable
    syncError == "unsupported device policy" -> RemovalRefusal.Unsupported
    syncError != null || state == null || state.wireVersion < 2 -> RemovalRefusal.Unverified
    Policy.pactActive(state, state.serverTime) -> RemovalRefusal.ActivePact
    state.binding?.selfRemoval != true -> RemovalRefusal.MissingAuthority
    else -> RemovalRefusal.NotCompleted
}
data class ResolverCheck(val policy: DnsPolicy, val networkId: Long, val testedAt: Long,
    val result: ProbeResult, val generation: Long = 0)

data class RequestTarget(val pkg: String, val label: String)

class FamilyWardenActions(
    val sync: () -> Unit,
    val scan: () -> Unit,
    val manualPair: () -> Unit,
    val submitAddress: (String) -> Unit,
    val confirmPair: () -> Unit,
    val dismiss: () -> Unit,
    val remove: () -> Unit,
    val confirmRemove: () -> Unit,
    val toggleUlysses: (Boolean) -> Unit,
    val testResolver: () -> Unit,
    val openSail: () -> Unit,
    val copyDiagnostics: () -> Unit,
    val openUsageAccess: () -> Unit,
    val askTime: (String, Int, String) -> Unit = { _, _, _ -> },
    val answerTime: (ParentRequest, Boolean, Int) -> Unit = { _, _, _ -> },
    val decideApp: (NewAppRequest, Boolean) -> Unit = { _, _ -> },
    val retryUpload: () -> Unit = {},
    val forgetUpload: () -> Unit = {},
    val chooseParentMode: () -> Unit = {},
    val parentSignOut: () -> Unit = {},
    val openRouter: () -> Unit = {},
    /** Blocking read; callers must use a worker thread, never the Compose main thread. */
    val loadHistory: (String, java.time.LocalDate) -> FamilyHistoryDay = { _, _ -> throw HistoryRefused("unavailable") },
    /** Blocking read, like [loadHistory]. */
    val loadApps: (String) -> InstalledApps = { throw InstalledRefused("unavailable") },
)
