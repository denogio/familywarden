package io.github.denogio.familywarden.review

import io.github.denogio.familywarden.ui.model.*

/** Review proposal only: outside Android runtime sources, not an enforcement decision. */
enum class ReviewedHeadline {
    PolicyMissing, Applying, NotApplied, ApplyFailed,
    AppliedUnlocked, PolicyMismatch,
    ResolverUntested, ResolverTesting, ResolverUnavailable, CertificateInvalid,
    AppliedAndLastTestPassed,
}

/**
 * Demonstrates missing guards in the supplied draft. Health still needs hostname/network
 * binding and freshness semantics before implementation; the original model lacks those.
 * The final "passed" label deliberately describes a past test, not guaranteed filtering.
 */
fun reviewedHeadline(state: AppUiState): ReviewedHeadline {
    val app = state.dnsApplication
    when (app) {
        DnsApplication.Applying -> return ReviewedHeadline.Applying
        DnsApplication.NotApplied -> return ReviewedHeadline.NotApplied
        is DnsApplication.Failed -> return ReviewedHeadline.ApplyFailed
        is DnsApplication.Applied -> Unit
    }
    app as DnsApplication.Applied
    val verified = when (val policy = state.policy) {
        is PolicyState.Verified -> policy
        is PolicyState.Rejected -> policy.lastVerified
        PolicyState.NotReceived -> null
    } ?: return ReviewedHeadline.PolicyMissing
    if (!app.locked) return ReviewedHeadline.AppliedUnlocked
    if (!app.hostname.removeSuffix(".").equals(
            verified.resolverHostname.removeSuffix("."), ignoreCase = true)) {
        return ReviewedHeadline.PolicyMismatch
    }
    return when (state.resolver) {
        ResolverHealth.Untested -> ReviewedHeadline.ResolverUntested
        ResolverHealth.Testing -> ReviewedHeadline.ResolverTesting
        is ResolverHealth.Unreachable -> ReviewedHeadline.ResolverUnavailable
        is ResolverHealth.CertificateInvalid -> ReviewedHeadline.CertificateInvalid
        is ResolverHealth.Reachable -> ReviewedHeadline.AppliedAndLastTestPassed
    }
}
