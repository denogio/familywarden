package io.github.denogio.familywarden.ui

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.ui.theme.MastTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.DayOfWeek
import java.time.LocalTime
import java.time.ZonedDateTime

/** UI6 role overviews with fictional fixtures (Alex): child and adult states, EN/DA, dark, 200 % text. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RoleOverviewsTest {
    @get:Rule val rule = createComposeRule()

    private val now = ZonedDateTime.parse("2026-10-23T19:00:00+02:00[Europe/Copenhagen]")
    private val apps = AppsPolicy("0123456789abcdef", false,
        mapOf("com.example.video" to 30, "com.example.game" to 60), emptySet(), emptySet(), emptyList(),
        mapOf(now.dayOfWeek to 120), setOf("com.example.school"))
    private val night = apps.copy(downtime = listOf(Window(DayOfWeek.entries.toSet(), LocalTime.of(21, 30), LocalTime.of(6, 30))))
    private val report = AppUsageReport(apps.rev, now.zone.id, now.toLocalDate(), 1000, true,
        mapOf(UsageCounter.TOTAL to 82 * 60_000L, "com.example.video" to 26 * 60_000L, "com.example.game" to 45 * 60_000L),
        mapOf("com.example.video" to "Video", "com.example.game" to "Game", "com.example.school" to "School"))
    private val actions = FamilyWardenActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
    private val child = FamilyWardenState(now.toInstant().toEpochMilli(), owner = true, paired = true, provisioned = true,
        binding = DeviceBinding("fw-alex", "alex", "Alex", false, false, false), lastSync = now.toEpochSecond(),
        timeZone = now.zone.id, appsActive = true, appsStatus = "ok", requestEpoch = now.toEpochSecond(),
        screenTime = screenTimeState(apps, report, now, 1000, true))
    private val lock = now.withHour(21).withMinute(30)
    private val morning = lock.plusDays(1).withHour(6).withMinute(30)
    private val pact = PactStatus.Bound(false, lock.toInstant().toEpochMilli(),
        Pact(now.plusDays(2).toInstant().toEpochMilli(), null, null), morning.toInstant().toEpochMilli())
    private val adult = child.copy(binding = DeviceBinding("fw-alex", "alex", "Alex", true, true, true), pact = pact,
        screenTime = screenTimeState(night, report, now, 1000, true))

    private fun used(total: Long, policy: AppsPolicy = apps) =
        screenTimeState(policy, report.copy(used = mapOf(UsageCounter.TOTAL to total)), now, 1000, true)

    private fun show(state: FamilyWardenState, dark: Boolean = false, fontScale: Float = 1f) = rule.setContent {
        val density = LocalDensity.current
        CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
            MastTheme(darkTheme = dark) { FamilyWardenScreen(state, actions) }
        }
    }
    private fun has(text: String, substring: Boolean = false) =
        rule.onAllNodesWithText(text, substring = substring).onFirst().assertExists()
    private fun absent(text: String, substring: Boolean = false) =
        rule.onAllNodesWithText(text, substring = substring, ignoreCase = substring).assertCountEquals(0)
    private fun navigation(vararg labels: String) = labels.forEach { label ->
        rule.onNode(hasText(label) and isSelectable()).assertExists()
        // The visible label is the only accessible name: TalkBack must not read it twice.
        rule.onAllNodesWithContentDescription(label).assertCountEquals(0)
    }

    @Test fun childNormalAnswersTimeLeftThenAskThenRunningLow() {
        show(child)
        has("TIME LEFT TODAY"); has("38 min"); has("Ask for more time")
        has("RUNNING LOW"); has("Video"); has("4 min left")
        absent("Game") // 15 min left is not running low
        has("ALWAYS OK"); has("School"); has("Phone, emergency calls and alarm clock")
        has("Updated", substring = true)
        navigation("Today", "History", "Settings")
        absent("hotspot", substring = true)
    }

    @Test fun childNearLimitTimesUpAndDowntime() {
        show(child.copy(screenTime = used(115 * 60_000L)))
        has("ALMOST TIME"); has("5 min")
    }

    @Test fun childTimesUpKeepsWhatStillWorks() {
        show(child.copy(screenTime = used(120 * 60_000L)), dark = true)
        has("Time’s up for today"); has("Phone, emergency calls and alarm clock"); has("Ask for more time")
        absent("ALWAYS OK")
    }

    @Test fun childDowntime() {
        val evening = apps.copy(downtime = listOf(Window(setOf(now.dayOfWeek), LocalTime.of(18, 0), LocalTime.of(20, 0))))
        show(child.copy(screenTime = screenTimeState(evening, report, now, 1000, true)))
        has("Downtime until", substring = true); has("Calls and the apps below still work.")
    }

    @Test fun childUnknownIsHonestAndNamesTheReason() {
        show(child.copy(appsStatus = "usage_access_missing", screenTime = screenTimeState(apps, report, now, 1000, false)))
        has("TIME UNKNOWN"); has("Screen time can’t be measured right now")
        has("Downtime and app rules still apply. Usage access is off.", substring = true)
        has("Allow usage access")
        absent("38 min"); absent("RUNNING LOW"); absent("Not counted", substring = true)
    }

    @Test fun unknownSaysInterruptedOnlyForAUsageGap() {
        show(child.copy(appsStatus = "usage_gap", screenTime = screenTimeState(apps, report.copy(reliable = false), now, 1000, true)))
        has("Measurement was interrupted today. Counting resumes tomorrow.", substring = true)
        absent("No fresh usage data from this phone.", substring = true)
    }

    @Test fun unknownWithFailedPausingDoesNotClaimRulesStillApply() {
        show(child.copy(appsStatus = "suspend_failed", screenTime = screenTimeState(apps, report.copy(reliable = false), now, 1000, true)))
        has("TIME UNKNOWN")
        absent("still apply", substring = true); absent("No fresh usage data", substring = true)
        has("Some apps could not be paused", substring = true)
    }

    @Test fun unknownWithoutAKnownCauseSaysNoFreshData() {
        show(child.copy(screenTime = screenTimeState(apps, report.copy(reliable = false), now, 1000, true)))
        has("No fresh usage data from this phone.", substring = true)
        absent("Measurement was interrupted", substring = true)
    }

    @Test fun childBoundByAPactCannotAskAndSeesNoPactCard() {
        show(child.copy(pact = PactStatus.Bound(true, null, Pact(null, null, null))))
        absent("Ask for more time"); absent("Lashed to the mast", substring = true)
    }

    @Test fun expiredRequestLeavesTheOverviewButALiveOneStays() {
        val request = ChildRequest("request-alex-001", "com.example.video", 30, "pending", now.toEpochSecond() + 600)
        show(child.copy(childRequest = request.copy(state = "expired")))
        absent("Your request", substring = true)
    }

    @Test fun pendingRequestStaysOnTheOverview() {
        val request = ChildRequest("request-alex-001", "com.example.video", 30, "pending", now.toEpochSecond() + 600)
        show(child.copy(childRequest = request))
        rule.onAllNodesWithContentDescription("Your request").onFirst().assertExists()
    }

    @Test fun adultOpenUntilCombinesAppsAndHotspotOnlyAtTheSameInstant() {
        show(adult)
        has("OPEN UNTIL", substring = true); has("2 h 30 min")
        has("Then apps and hotspot rest until", substring = true)
        has("38 min of screen time left today")
        has("Lashed to the mast till", substring = true); has("Hotspot is allowed")
        has("RUNNING LOW"); has("4 min left")
        navigation("Today", "DNS", "Ulysses", "Settings")
        absent("ALWAYS OK")
    }

    @Test fun adultHotspotLockFirstStillNamesTheAppsPause() {
        show(adult.copy(pact = pact.copy(nextChangeMillis = lock.minusHours(1).toInstant().toEpochMilli())))
        has("OPEN UNTIL", substring = true); has("1 h 30 min")
        has("Then hotspot rests until", substring = true)
        has("Apps rest from", substring = true)
        absent("Then apps and hotspot rest", substring = true)
    }

    @Test fun adultWithoutAPactNeverMentionsHotspot() {
        show(adult.copy(pact = PactStatus.FreeSailing))
        has("Then apps rest until", substring = true)
        absent("hotspot", substring = true)
    }

    @Test fun adultNearLimitAndTimesUpUseTheBudgetHero() {
        show(adult.copy(screenTime = used(115 * 60_000L, night)))
        has("ALMOST TIME"); has("5 min")
    }

    @Test fun adultTimesUp() {
        show(adult.copy(screenTime = used(120 * 60_000L, night)), dark = true)
        has("Time’s up for today")
    }

    @Test fun adultRestingNamesWhatRestsUntilWhen() {
        val late = now.withHour(22)
        show(adult.copy(nowMillis = late.toInstant().toEpochMilli(),
            pact = pact.copy(blocked = true, nextChangeMillis = morning.toInstant().toEpochMilli(), followingChangeMillis = null),
            screenTime = screenTimeState(night, report, late, 1000, true)))
        has("RESTING"); has("Apps and hotspot rest until", substring = true); has("Hotspot is locked")
        absent("OPEN UNTIL", substring = true)
    }

    @Test fun adultUnknown() {
        show(adult.copy(appsStatus = "usage_gap", screenTime = screenTimeState(night, report.copy(reliable = false), now, 1000, true)))
        has("TIME UNKNOWN"); has("Measurement was interrupted today.", substring = true)
        absent("OPEN UNTIL", substring = true); absent("RUNNING LOW")
    }

    @Config(qualifiers = "da")
    @Test fun adultPactOnlyInDanishDarkAtDoubleText() {
        show(adult.copy(screenTime = null, appsActive = false), dark = true, fontScale = 2f)
        has("ÅBENT TIL", substring = true); has("Derefter holder hotspot pause til", substring = true)
        has("Bundet til masten til", substring = true); has("Hotspot er tilladt")
        navigation("Oversigt", "DNS", "Ulysses", "Indstillinger")
        wholeLabels("Oversigt", "DNS", "Ulysses", "Indstillinger")
    }

    @Config(qualifiers = "da-w360dp-h800dp")
    @Test fun childNavigationLabelsStayWholeInDanishAtDoubleText() {
        show(child, fontScale = 2f)
        wholeLabels("I dag", "Historik", "Indstillinger")
    }

    /** Bottom-navigation labels shrink rather than truncate or clip (headings are not checked here). */
    private fun wholeLabels(vararg labels: String) = labels.forEach { label ->
        val height = rule.onRoot().fetchSemanticsNode().size.height
        val nodes = rule.onAllNodesWithText(label, useUnmergedTree = true).fetchSemanticsNodes()
            .filter { androidx.compose.ui.semantics.SemanticsActions.GetTextLayoutResult in it.config && it.boundsInRoot.top > height / 2 }
        org.junit.Assert.assertTrue("no text node: $label", nodes.isNotEmpty())
        nodes.forEach { node ->
            val layouts = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
            node.config[androidx.compose.ui.semantics.SemanticsActions.GetTextLayoutResult].action!!.invoke(layouts)
            org.junit.Assert.assertFalse("truncated: $label", layouts.single().hasVisualOverflow)
        }
    }

    @Test fun statusChipSaysOfflineOrNotUpdatedOnce() {
        show(child.copy(syncError = "router unreachable"))
        has("Offline · updated", substring = true)
        absent("The router could not be reached", substring = true)
    }

    @Test fun statusChipFailureExplainsTheReason() {
        show(adult.copy(syncError = "bad signature"))
        has("Not updated"); has("Reply could not be verified; last trusted policy retained")
    }

    @Test fun adultWithoutAppRulesExplainsSyncFailure() {
        show(adult.copy(screenTime = null, appsActive = false, syncError = "device not approved"))
        has("The router no longer approves this phone; last policy kept")
    }
}
