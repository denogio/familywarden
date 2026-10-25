package io.github.denogio.familywarden

import io.github.denogio.familywarden.ui.*
import org.junit.Assert.*
import org.junit.Test

class FamilyWardenStateTest {
    private val policy = DnsPolicy("alex.dns.example.net", setOf(Protection.SAFE_SEARCH))
    private val binding = DeviceBinding("fw-example", "captain", "Alex", true, true, true)
    private fun healthy() = FamilyWardenState(
        nowMillis = 1800000000000, owner = true, paired = true, provisioned = true,
        binding = binding, desiredDns = policy, appliedDnsHost = policy.hostname,
        dnsLocked = true, networkId = 12,
        resolverCheck = ResolverCheck(policy, 12, 1800000000000, ProbeResult.ANSWERED),
    )

    @Test fun familyPresentationNeedsApprovalScopeNotAdministratorOrCaptain() {
        val state = healthy()
        assertFalse(state.canManageFamily)
        assertTrue(state.copy(parentMode = true, owner = false).canManageFamily)
        val parent = state.copy(binding = binding.copy(approves = listOf(ProfileRef("sam", "Sam"))))
        assertTrue(parent.canManageFamily)
        assertTrue(parent.copy(pact = PactStatus.Bound(true, null, Pact(null, null, null))).canManageFamily)
        assertFalse(parent.copy(binding = binding.copy(approves = emptyList())).canManageFamily)
    }

    @Test fun pendingBadgeCountsOnlyCurrentScopedRequestsAndNewApps() {
        val sam = ProfileRef("sam", "Sam")
        val alex = ProfileRef("alex", "Alex")
        val request = ParentRequest("request-001", "a".repeat(64), sam, "total", 30, 10, 100, "")
        val state = healthy().copy(binding = binding.copy(approves = listOf(sam)), requestEpoch = 99,
            parentRequests = listOf(request, request.copy(id = "request-002", profile = alex)),
            parentNewApps = listOf(NewAppRequest(sam, "com.example.draw", "Draw", 10)))
        assertEquals(2, state.familyPendingCount)
        assertEquals(1, state.copy(requestEpoch = 100).familyPendingCount)
        assertEquals(0, state.copy(binding = null).familyPendingCount)
    }

    @Test fun positiveHeadlineRequiresAllIndependentFacts() {
        val s = healthy()
        assertEquals(DnsHeadline.AppliedAndTested, s.headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(owner = false).headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(binding = null).headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(desiredDns = null).headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(appliedDnsHost = "other.example.net").headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(dnsLocked = false).headline)
        assertNotEquals(DnsHeadline.AppliedAndTested, s.copy(dnsError = "failed").headline)
    }

    @Test fun roamingInvalidatesHealthResult() {
        assertEquals(DnsHeadline.AppliedUntested, healthy().copy(networkId = 13).headline)
        assertNull(healthy().copy(networkId = null).currentCheck)
    }

    @Test fun networkGenerationInvalidatesLateResultEvenAfterReturningToSameNetwork() {
        assertEquals(DnsHeadline.AppliedUntested, healthy().copy(probeGeneration = 1).headline)
    }

    @Test fun policyChangesInvalidateProbeEvenWhenHostnameDoesNotChange() {
        val changed = policy.copy(protections = setOf(Protection.PARENTAL))
        assertEquals(DnsHeadline.AppliedUntested, healthy().copy(desiredDns = changed).headline)
    }

    @Test fun routerOfflineDoesNotChangeDnsOrPactFacts() {
        val s = healthy().copy(syncError = "router unreachable")
        assertEquals(DnsHeadline.AppliedAndTested, s.headline)
        assertEquals(policy, s.desiredDns)
    }

    @Test fun missingPolicyNeverClaimsProtection() {
        assertEquals(DnsHeadline.NotConfigured, FamilyWardenState(0).headline)
        assertEquals(DnsHeadline.PolicyMissing, FamilyWardenState(0, paired = true).headline)
        assertEquals(DnsHeadline.RouterUnsupported, FamilyWardenState(0, paired = true, policyUnsupported = true).headline)
    }

    @Test fun resolverFailureIsNotAClaimThatNetworkBlocksPort853() {
        val s = healthy()
        assertEquals(DnsHeadline.Unavailable, s.copy(resolverCheck = s.resolverCheck!!.copy(result = ProbeResult.UNAVAILABLE)).headline)
        assertEquals(DnsHeadline.Testing, s.copy(resolverCheck = s.resolverCheck!!.copy(result = ProbeResult.TESTING)).headline)
    }

    @Test fun captainOptInIsIndependentOfAdministratorRole() {
        val s = healthy().copy(binding = binding.copy(administrator = false, selfRemoval = false), ulyssesEnabled = true)
        assertTrue(s.showUlysses)
        assertFalse(s.canRequestRemoval)
        assertFalse(s.canRePair)
        assertFalse(healthy().copy(binding = binding.copy(captain = false), ulyssesEnabled = true).showUlysses)
    }

    @Test fun activeApplicablePactForcesVisibilityAndLocksActions() {
        val s = healthy().copy(ulyssesEnabled = false,
            pact = PactStatus.Bound(true, null, Pact(null, null, null)))
        assertTrue(s.showUlysses)
        assertFalse(s.canRequestRemoval)
        assertFalse(s.canRePair)
    }

    @Test fun refusalCopyDistinguishesAuthorityPactTransportAndVerification() {
        val state = MastState(1800000000, "captain", false, "", 0, 0,
            Rules(true, "allow", "none", emptyList()), binding = binding, wireVersion = 2)
        assertEquals(RemovalRefusal.ActivePact, removalRefusal(state.copy(pactActive = true, pactType = "ongoing"), null))
        assertEquals(RemovalRefusal.MissingAuthority, removalRefusal(state.copy(binding = binding.copy(administrator = false, selfRemoval = false)), null))
        assertEquals(RemovalRefusal.Unreachable, removalRefusal(state, "router unreachable"))
        assertEquals(RemovalRefusal.Unsupported, removalRefusal(state, "unsupported device policy"))
        assertEquals(RemovalRefusal.Unverified, removalRefusal(state, "bad signature"))
        assertEquals(RemovalRefusal.Unverified, removalRefusal(null, null))
        assertEquals(RemovalRefusal.NotCompleted, removalRefusal(state, null))
    }

    @Test fun managedExplicitApprovalAllowsRemovalButNotRePairing() {
        val s = healthy().copy(binding = binding.copy(administrator = false, selfRemoval = true))
        assertTrue(s.canRequestRemoval)
        assertFalse(s.canRePair)
    }

    @Test fun managedNoPactIsNotRemovalOrRePairPermission() {
        val s = healthy().copy(binding = binding.copy(administrator = false, selfRemoval = false), pact = PactStatus.FreeSailing)
        assertFalse(s.canRequestRemoval)
        assertFalse(s.canRePair)
        assertFalse(healthy().copy(binding = null).canRequestRemoval)
    }

    @Test fun managedOrPactBoundPhoneCanStillScanToFollowItsRouter() {
        val managed = healthy().copy(binding = binding.copy(administrator = false, selfRemoval = false))
        assertFalse(managed.canRePair)
        assertTrue(managed.canScan)
        assertTrue(healthy().copy(pact = PactStatus.Bound(true, null, Pact(null, null, null))).canScan)
        assertFalse(managed.copy(owner = false).canScan)
    }

    @Test fun addressRepairOnlyForTheVeryKeyThisPhonePinned() {
        val key = PhoneKey.generate().publicKey
        val info = Sync.PairingInfo("192.0.2.9", key, Usign.parsePublicKey(key).fingerprint)
        assertTrue(Sync.isAddressRepair(true, key, info))
        assertFalse(Sync.isAddressRepair(false, key, info))          // not enrolled: normal pairing
        assertFalse(Sync.isAddressRepair(true, null, info))
        assertFalse(Sync.isAddressRepair(true, PhoneKey.generate().publicKey, info)) // another router
    }
}
