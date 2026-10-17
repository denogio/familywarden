package io.github.denogio.familywarden.review

import io.github.denogio.familywarden.ui.model.*
import org.junit.Assert.*
import org.junit.Test

/**
 * Executable review, not production behavior. Original draft is preserved unchanged.
 * Characterization assertions reproduce its blind spots; proposal assertions show
 * the minimum headline guards. Authorization/device-binding questions remain open.
 */
class DesignContractReviewTest {
    private val now = 1_800_000_000_000L
    private val host = "phone-id.dns.example.net"
    private val verified = PolicyState.Verified(now - 1000, host, null, emptyList())
    private fun healthy() = AppUiState(
        now, SetupProgress.Complete,
        capabilities = Capabilities(managedDevice = true),
        policy = verified,
        dnsApplication = DnsApplication.Applied(host, locked = true),
        resolver = ResolverHealth.Reachable(now),
        routerSync = RouterSync.Reachable(now),
    )

    @Test fun draftReportsOnWithoutAnyVerifiedPolicy() {
        val s = healthy().copy(policy = PolicyState.NotReceived)
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.PolicyMissing, reviewedHeadline(s))
    }

    @Test fun draftReportsOnEvenWhenAndroidSettingIsNotLocked() {
        val s = healthy().copy(dnsApplication = DnsApplication.Applied(host, locked = false))
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.AppliedUnlocked, reviewedHeadline(s))
    }

    @Test fun draftIgnoresAppliedHostnameMismatch() {
        val s = healthy().copy(dnsApplication = DnsApplication.Applied("different.dns.example.net", true))
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.PolicyMismatch, reviewedHeadline(s))
    }

    @Test fun noProbeIsNotEvidenceOfReachability() {
        val s = healthy().copy(resolver = ResolverHealth.Untested)
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.ResolverUntested, reviewedHeadline(s))
    }

    @Test fun runningProbeIsNotEvidenceOfSuccess() {
        val s = healthy().copy(resolver = ResolverHealth.Testing)
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.ResolverTesting, reviewedHeadline(s))
    }

    @Test fun unreachableDoesNotDiagnoseWhichComponentFailed() {
        val s = healthy().copy(resolver = ResolverHealth.Unreachable(now, now - 1000))
        assertEquals(OverviewHeadline.NetworkBlocksResolver, s.overviewHeadline())
        assertEquals(ReviewedHeadline.ResolverUnavailable, reviewedHeadline(s))
    }

    @Test fun rejectedReplyKeepsPreviousTrustedPolicy() {
        val s = healthy().copy(policy = PolicyState.Rejected("bad signature", verified))
        assertEquals(ReviewedHeadline.AppliedAndLastTestPassed, reviewedHeadline(s))
        assertTrue(s.policy is PolicyState.Rejected) // UI must still show the rejection warning.
    }

    @Test fun rejectedReplyWithoutPriorPolicyCannotBeCalledProtected() {
        val s = healthy().copy(policy = PolicyState.Rejected("bad signature", null))
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.PolicyMissing, reviewedHeadline(s))
    }

    @Test fun offHomeRouterSyncDoesNotInvalidateAppliedDns() {
        val s = healthy().copy(routerSync = RouterSync.Unreachable(now - 365L * 86400000))
        assertEquals(OverviewHeadline.ProtectionOn, s.overviewHeadline())
        assertEquals(ReviewedHeadline.AppliedAndLastTestPassed, reviewedHeadline(s))
    }

    @Test fun applicationFailuresStaySeparate() {
        assertEquals(ReviewedHeadline.Applying, reviewedHeadline(healthy().copy(dnsApplication = DnsApplication.Applying)))
        assertEquals(ReviewedHeadline.NotApplied, reviewedHeadline(healthy().copy(dnsApplication = DnsApplication.NotApplied)))
        assertEquals(ReviewedHeadline.ApplyFailed, reviewedHeadline(healthy().copy(dnsApplication = DnsApplication.Failed("Android rejected setting"))))
        assertEquals(ReviewedHeadline.CertificateInvalid, reviewedHeadline(healthy().copy(resolver = ResolverHealth.CertificateInvalid(now, null))))
    }

    @Test fun dnsHostnamesAreCaseInsensitive() {
        val s = healthy().copy(dnsApplication = DnsApplication.Applied(host.uppercase() + ".", true))
        assertEquals(ReviewedHeadline.AppliedAndLastTestPassed, reviewedHeadline(s))
    }

    @Test fun ordinaryDeviceHasThreeDestinations() {
        assertEquals(listOf(Destination.Overview, Destination.Dns, Destination.Settings), healthy().destinations())
    }

    @Test fun activePactOverridesLocalOffPreference() {
        val s = healthy().copy(ulysses = UlyssesState(pact = Pact(PactKind.Ongoing, null)))
        assertTrue(s.ulyssesVisible())
        assertFalse(s.pairingAllowed())
        assertEquals(listOf(Destination.Overview, Destination.Dns, Destination.Ulysses, Destination.Settings), s.destinations())
    }

    @Test fun finitePactExpiresLocallyAtTheExactBoundary() {
        val s = healthy().copy(ulysses = UlyssesState(pact = Pact(PactKind.Until(now), null)))
        assertFalse(s.ulyssesVisible())
        assertTrue(s.pairingAllowed()) // Not a removal authorization assertion.
    }

    @Test fun syncedReleaseExpiresLocallyWithoutRouter() {
        val s = healthy().copy(
            ulysses = UlyssesState(pact = Pact(PactKind.Ongoing, now)),
            routerSync = RouterSync.Unreachable(now - 100000),
        )
        assertFalse(s.ulysses.pactActive(now))
    }

    @Test fun futureReleaseDoesNotExtendAFinitePact() {
        val s = UlyssesState(pact = Pact(PactKind.Until(now), now + 100000))
        assertFalse(s.pactActive(now))
    }

    @Test fun draftVisibilityNeedsAnAuthoritativeMapper() {
        // The function trusts "On" even with a mismatching association. It cannot grant
        // captain privileges; backend mapping and action guards must reject this state.
        val s = healthy().copy(
            ulyssesFeature = UlyssesFeature.On,
            capabilities = Capabilities(association = Association.Mismatch(null), captain = false),
        )
        assertTrue(s.ulyssesVisible())
    }

    @Test fun onboardingHasNoBottomNavigation() {
        assertTrue(healthy().copy(setup = SetupProgress.ApplyingDns).destinations().isEmpty())
    }

    @Test fun roleAxesRemainIndependent() {
        val admin = healthy().copy(capabilities = Capabilities(administrator = true, captain = false))
        assertFalse(admin.ulyssesVisible())
        assertTrue(Destination.Family in admin.destinations())
        val captain = healthy().copy(capabilities = Capabilities(captain = true))
        assertFalse(Destination.Family in captain.destinations())
        assertFalse(captain.ulyssesVisible()) // Eligibility does not opt in automatically.
    }
}
