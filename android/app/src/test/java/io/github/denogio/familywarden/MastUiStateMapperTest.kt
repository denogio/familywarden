package io.github.denogio.familywarden

import io.github.denogio.familywarden.ui.*
import org.junit.Assert.*
import org.junit.Test
import java.time.ZonedDateTime

class MastUiStateMapperTest {
    private val now = ZonedDateTime.parse("2026-10-12T22:00:00+02:00[Europe/Copenhagen]")
    private val state = MastState(1, "dennis", true, "ongoing", 0, 0,
        Rules(true, "block", "none", emptyList()))

    private fun map(s: MastState? = state, owner: Boolean = true, host: String? = "router.lan", error: String? = null) =
        mapMastUiState(now.toInstant().toEpochMilli(), owner, host, "0123456789abcdef", s,
            Policy.decide(s, now), Policy.nextBlockChange(s, now)?.toEpochSecond(), 123, error)

    @Test fun setupStates() {
        assertEquals(SetupState.NotDeviceOwner, map(owner = false).setup)
        assertEquals(SetupState.NotPaired, map(s = null, host = null).setup)
        assertEquals(PactStatus.FreeSailing, (map(state.copy(pactActive = false)).setup as SetupState.Paired).status)
    }

    @Test fun activePactMapsEpochSecondsAndBlock() {
        val ui = map(state.copy(pactType = "until", pactUntil = now.toEpochSecond() + 3600))
        val bound = (ui.setup as SetupState.Paired).status as PactStatus.Bound
        assertTrue(bound.blocked)
        assertEquals((now.toEpochSecond() + 3600) * 1000, bound.nextChangeMillis)
        assertEquals(bound.nextChangeMillis, bound.pact.untilMillis)
        assertEquals(123000L, ui.details.lastSyncMillis)
    }

    @Test fun localReleaseAndExpiryBecomeFreeWithoutSync() {
        for (s in listOf(state.copy(releaseAt = now.toEpochSecond()),
            state.copy(pactType = "until", pactUntil = now.toEpochSecond()))) {
            assertEquals(PactStatus.FreeSailing, (map(s).setup as SetupState.Paired).status)
        }
    }

    @Test fun noCaptainDoesNotOfferPairingDuringAnActivePact() {
        val status = (map(state.copy(captain = "")).setup as SetupState.Paired).status
        assertTrue(status is PactStatus.Bound)
        assertFalse((status as PactStatus.Bound).blocked)
    }

    @Test fun syncErrorsKeepCachedPactAndClassifyWarnings() {
        assertEquals(SyncState.Unreachable, map(error = "router unreachable").sync)
        val invalid = map(error = "bad signature")
        assertEquals(SyncState.BadSignature, invalid.sync)
        assertTrue((invalid.setup as SetupState.Paired).status is PactStatus.Bound)
        assertEquals(SyncState.Failed("stale reply"), map(error = "stale reply").sync)
    }

    @Test fun sailingHomeDoesNotOutliveFinitePact() {
        val bound = (map(state.copy(pactType = "until", pactUntil = now.toEpochSecond() + 60,
            releaseAt = now.toEpochSecond() + 3600)).setup as SetupState.Paired).status as PactStatus.Bound
        assertNull(bound.pact.releaseAtMillis)
    }

    @Test fun followingChangeIsDisplayOnlyAndNeedsANextChange() {
        val s = state.copy(rules = Rules(true, "scheduled", "none",
            listOf(Window(java.time.DayOfWeek.entries.toSet(), java.time.LocalTime.of(6, 30), java.time.LocalTime.of(21, 30)))))
        val at = now.withHour(20)
        val next = Policy.nextBlockChange(s, at)!!
        val following = Policy.nextBlockChange(s, next)!!
        val ui = mapMastUiState(at.toInstant().toEpochMilli(), true, "router.lan", null, s, Policy.decide(s, at),
            next.toEpochSecond(), 123, null, followingChangeEpoch = following.toEpochSecond())
        val bound = (ui.setup as SetupState.Paired).status as PactStatus.Bound
        assertFalse(bound.blocked)
        assertEquals(at.withHour(21).withMinute(30).toInstant().toEpochMilli(), bound.nextChangeMillis)
        assertEquals(at.plusDays(1).withHour(6).withMinute(30).toInstant().toEpochMilli(), bound.followingChangeMillis)
        val none = mapMastUiState(at.toInstant().toEpochMilli(), true, "router.lan", null, s, Policy.decide(s, at),
            null, 123, null, followingChangeEpoch = following.toEpochSecond())
        assertNull(((none.setup as SetupState.Paired).status as PactStatus.Bound).followingChangeMillis)
    }
}
